# Session Management Analysis Report

**Date:** 2026-03-06
**Repositories Analyzed:** nanoclaw, picoclaw, zeroclaw

---

## Executive Summary

| Repository | Language | Session Model | Storage | Scope |
|------------|----------|---------------|---------|-------|
| **nanoclaw** | TypeScript | Group-based session IDs | SQLite (`sessions` table) | Per-group-folder isolation |
| **picoclaw** | Go | Session key + history | JSON files in `sessions/` dir | Agent-scoped, multi-channel routing |
| **zeroclaw** | Rust | Session-scoped memory | SQLite (`memories.session_id`) | Memory isolation via session filter |

---

## 1. nanoclaw (TypeScript)

### Core Files
- `src/db.ts` - Database schema and session accessors
- `src/index.ts` - Main loop, session state management
- `src/container-runner.ts` - Container I/O with session ID passing

### Session Model
```
sessions table:
  - group_folder (PRIMARY KEY)
  - session_id (TEXT)
```

### Session Lifecycle

#### Creation
```typescript
// src/db.ts
export function setSession(groupFolder: string, sessionId: string): void {
  db.prepare(
    'INSERT OR REPLACE INTO sessions (group_folder, session_id) VALUES (?, ?)'
  ).run(groupFolder, sessionId);
}
```

Sessions are created on-demand when the agent returns a `newSessionId` in container output.

#### Storage
- **Database:** SQLite (`~/.nanoclaw/data/messages.db`)
- **Table:** `sessions` with `group_folder` as primary key
- **Migration:** Automatic JSON migration from `sessions.json` on first run

#### Retrieval
```typescript
// src/index.ts
let sessions: Record<string, string> = {};

function loadState(): void {
  sessions = getAllSessions();  // from db.ts
  // ...
}

// Usage in runAgent()
const sessionId = sessions[group.folder];
```

#### Update Flow
1. Agent runs in container with `sessionId` passed via stdin
2. Agent returns `newSessionId` in output markers
3. Session ID updated in memory and persisted to SQLite:
```typescript
if (output.newSessionId) {
  sessions[group.folder] = output.newSessionId;
  setSession(group.folder, output.newSessionId);
}
```

#### Destruction
- No explicit session destruction
- Sessions persist across restarts (SQLite-backed)
- Group deregistration removes session implicitly (no cascade delete in schema)

### Key Characteristics
- **Scope:** One session per group folder (e.g., "main", "friends", "work")
- **Isolation:** Container isolation per group with separate `.claude/` directories
- **Persistence:** SQLite survives restarts
- **No cleanup:** Sessions never expire or get garbage collected

---

## 2. picoclaw (Go)

### Core Files
- `pkg/session/manager.go` - Session manager implementation
- `pkg/routing/session_key.go` - Session key construction
- `pkg/agent/instance.go` - Agent instance with session manager
- `pkg/agent/context.go` - Context building with session history

### Session Model
```go
type Session struct {
    Key      string              `json:"key"`
    Messages []providers.Message `json:"messages"`
    Summary  string              `json:"summary,omitempty"`
    Created  time.Time           `json:"created"`
    Updated  time.Time           `json:"updated"`
}
```

### Session Key Structure
```
agent:<agentId>:<channel>:<peerKind>:<peerId>

Examples:
- agent:main:main                          (default main session)
- agent:main:discord:direct:123456         (DM scope per-peer)
- agent:main:telegram:group:987654321      (group chat)
- agent:main:discord:987654321:direct:123  (per-account-channel-peer)
```

### Session Lifecycle

#### Creation
```go
func (sm *SessionManager) GetOrCreate(key string) *Session {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    if session, ok := sm.sessions[key]; ok {
        return session
    }
    
    session = &Session{
        Key:      key,
        Messages: []providers.Message{},
        Created:  time.Now(),
        Updated:  time.Now(),
    }
    sm.sessions[key] = session
    return session
}
```

#### Storage
- **Format:** JSON files in `{workspace}/sessions/` directory
- **Filename:** Sanitized session key (colons → underscores)
- **Example:** `agent_main_discord_direct_123456.json`

#### Persistence
```go
func (sm *SessionManager) Save(key string) error {
    // Atomic write via temp file + rename
    tmpFile, _ := os.CreateTemp(sm.storage, "session-*.tmp")
    tmpFile.Write(data)
    os.Rename(tmpPath, sessionPath)  // atomic
}

func (sm *SessionManager) loadSessions() error {
    files, _ := os.ReadDir(sm.storage)
    for _, file := range files {
        // Parse JSON, populate sm.sessions map
    }
}
```

#### Retrieval
```go
func (sm *SessionManager) GetHistory(key string) []providers.Message {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    session, ok := sm.sessions[key]
    if !ok {
        return []providers.Message{}
    }
    
    history := make([]providers.Message, len(session.Messages))
    copy(history, session.Messages)
    return history
}
```

#### Update Operations
- `AddMessage(sessionKey, role, content)` - Append simple message
- `AddFullMessage(sessionKey, msg)` - Append with tool calls
- `SetHistory(key, history)` - Replace entire history
- `SetSummary(key, summary)` - Store conversation summary
- `TruncateHistory(key, keepLast)` - Trim to keep last N messages

#### Destruction
- No explicit destruction API
- Files persist on disk
- Memory map cleared on process exit

### Session Key Routing
```go
func BuildAgentPeerSessionKey(params SessionKeyParams) string {
    // DM scope options:
    // - DMScopeMain: shared session for all DMs
    // - DMScopePerPeer: one session per peer across channels
    // - DMScopePerChannelPeer: one session per channel+peer
    // - DMScopePerAccountChannelPeer: full isolation
    
    // Groups always get per-peer sessions
}
```

### Key Characteristics
- **Scope:** Agent-scoped with flexible DM isolation levels
- **Storage:** JSON files (one per session)
- **Atomic saves:** Temp file + rename for crash safety
- **Thread-safe:** RWMutex for concurrent access
- **Cross-platform:** Filename sanitization for Windows compatibility
- **History management:** Truncation, summary support

---

## 3. zeroclaw (Rust)

### Core Files
- `src/memory/sqlite.rs` - SQLite memory with session_id column
- `src/agent/agent.rs` - Agent with conversation history
- `src/config/schema.rs` - Configuration (max_history_messages)

### Session Model
```rust
// Memory table with session isolation
memories table:
  - id          TEXT PRIMARY KEY
  - key         TEXT NOT NULL UNIQUE
  - content     TEXT NOT NULL
  - category    TEXT NOT NULL DEFAULT 'core'
  - embedding   BLOB
  - session_id  TEXT  // Added via migration
  - created_at  TEXT NOT NULL
  - updated_at  TEXT NOT NULL

// In-memory conversation history
Agent.history: Vec<ConversationMessage>
```

### Session Lifecycle

#### Creation (Memory)
```rust
async fn store(
    &self,
    key: &str,
    content: &str,
    category: MemoryCategory,
    session_id: Option<&str>,  // Optional session filter
) -> anyhow::Result<()> {
    // Insert with session_id column
    conn.execute(
        "INSERT INTO memories (..., session_id) VALUES (?, ..., ?)",
        params![..., sid],
    )?;
}
```

#### Session-Scoped Retrieval
```rust
async fn recall(
    &self,
    query: &str,
    limit: usize,
    session_id: Option<&str>,  // Filter by session
) -> anyhow::Result<Vec<MemoryEntry>> {
    // Hybrid search: vector + FTS5 BM25
    // Results filtered by session_id if provided
    
    if let Some(filter_sid) = session_ref {
        if entry.session_id.as_deref() != Some(filter_sid) {
            continue;  // Skip non-matching sessions
        }
    }
}
```

#### Conversation History (In-Memory)
```rust
pub struct Agent {
    history: Vec<ConversationMessage>,
    // ...
}

impl Agent {
    pub fn clear_history(&mut self) {
        self.history.clear();
    }
    
    fn trim_history(&mut self) {
        let max = self.config.max_history_messages;
        if self.history.len() > max {
            // Keep system messages, trim oldest others
        }
    }
}
```

#### Persistence
- **Memory:** SQLite with WAL mode (crash-safe)
- **Conversation history:** In-memory only (lost on restart)
- **Embeddings:** Cached in `embedding_cache` table

#### Schema Migration
```rust
// Automatic migration for session_id column
let has_session_id: bool = conn
    .prepare("SELECT sql FROM sqlite_master WHERE type='table' AND name='memories'")?
    .query_row([], |row| row.get::<_, String>(0))?
    .contains("session_id");

if !has_session_id {
    conn.execute_batch(
        "ALTER TABLE memories ADD COLUMN session_id TEXT;
         CREATE INDEX IF NOT EXISTS idx_memories_session ON memories(session_id);"
    )?;
}
```

### Key Characteristics
- **Two-level sessions:**
  1. **Conversation history** - In-memory, per agent instance
  2. **Memory storage** - SQLite with optional session_id filter
- **No explicit session creation** - session_id passed at store/recall time
- **Flexible isolation** - Same DB, filtered queries
- **Vector search** - Session-aware similarity search
- **Auto-migration** - Schema evolves gracefully

---

## Comparison Matrix

| Feature | nanoclaw | picoclaw | zeroclaw |
|---------|----------|----------|----------|
| **Language** | TypeScript | Go | Rust |
| **Storage Backend** | SQLite | JSON files | SQLite |
| **Session Granularity** | Per-group-folder | Agent+channel+peer | Per memory entry |
| **History Storage** | External (Claude Code) | In JSON file | In-memory + SQLite |
| **Thread Safety** | Single-threaded | RWMutex | Mutex + async |
| **Persistence** | Full (SQLite) | Full (JSON) | Memory only, SQLite for long-term |
| **Atomic Writes** | SQLite transactions | Temp file + rename | WAL mode |
| **Session Key Format** | `group_folder` | `agent:...:peer` | Optional string |
| **Cleanup/GC** | None | None | Retention config |
| **Summary Support** | No | Yes | No |
| **Cross-session Isolation** | Container-level | File-level | Query-level |

---

## Architecture Patterns

### nanoclaw: Container-Isolated Sessions
```
┌─────────────────────────────────────────────────────┐
│                    Host Machine                      │
├─────────────────────────────────────────────────────┤
│  SQLite (messages.db)                               │
│  └── sessions table: {group_folder → session_id}    │
├─────────────────────────────────────────────────────┤
│  Container per group                                │
│  ├── Group A: /data/sessions/A/.claude/            │
│  └── Group B: /data/sessions/B/.claude/            │
└─────────────────────────────────────────────────────┘
```
- Session ID is opaque (Claude Code manages actual history)
- Each group runs in isolated container with separate `.claude/` state
- Host tracks session IDs for continuity across container restarts

### picoclaw: Agent-Scoped File Sessions
```
┌─────────────────────────────────────────────────────┐
│                    Workspace                         │
├─────────────────────────────────────────────────────┤
│  sessions/                                           │
│  ├── agent_main_main.json                          │
│  ├── agent_main_discord_direct_123.json            │
│  └── agent_main_telegram_group_456.json            │
├─────────────────────────────────────────────────────┤
│  SessionManager (in-memory map)                     │
│  └── sync with JSON files on Save()                 │
└─────────────────────────────────────────────────────┘
```
- Flexible routing based on DM scope configuration
- History stored as `[]providers.Message` in JSON
- Summary field for context compression

### zeroclaw: Session-Tagged Memory
```
┌─────────────────────────────────────────────────────┐
│                    SQLite (brain.db)                 │
├─────────────────────────────────────────────────────┤
│  memories table                                      │
│  ├── id, key, content, category, session_id        │
│  └── embedding (BLOB), FTS5 index                   │
├─────────────────────────────────────────────────────┤
│  Agent instance (in-memory)                         │
│  └── history: Vec<ConversationMessage>              │
└─────────────────────────────────────────────────────┘
```
- Session is an optional filter, not a primary entity
- Same memory store shared across all sessions
- Isolation via WHERE clause in queries

---

## Recommendations

### For Session Management Unification

1. **Adopt picoclaw's session key format** as the standard
   - Hierarchical: `agent:channel:peerKind:peerId`
   - Supports multiple isolation levels
   - Clear semantics

2. **Consider zeroclaw's approach for memory isolation**
   - Single DB with session_id column is efficient
   - No file proliferation
   - Cross-session search still possible when needed

3. **Add session lifecycle management**
   - All three lack explicit cleanup
   - Consider TTL-based expiration
   - Implement session archival for long-running deployments

4. **Standardize persistence strategy**
   - picoclaw's atomic JSON writes are robust
   - SQLite WAL mode (zeroclaw) is excellent for concurrency
   - nanoclaw's SQLite + container isolation is secure but complex

---

## Appendix: Code References

### nanoclaw Key Functions
| Function | File | Purpose |
|----------|------|---------|
| `getSession()` | `src/db.ts` | Retrieve session ID for group |
| `setSession()` | `src/db.ts` | Store session ID for group |
| `getAllSessions()` | `src/db.ts` | Load all sessions as map |
| `runContainerAgent()` | `src/container-runner.ts` | Pass sessionId to container |

### picoclaw Key Functions
| Function | File | Purpose |
|----------|------|---------|
| `NewSessionManager()` | `pkg/session/manager.go` | Create manager with storage dir |
| `GetOrCreate()` | `pkg/session/manager.go` | Get or create session |
| `Save()` | `pkg/session/manager.go` | Persist to JSON file |
| `BuildAgentPeerSessionKey()` | `pkg/routing/session_key.go` | Construct session key |

### zeroclaw Key Functions
| Function | File | Purpose |
|----------|------|---------|
| `store()` | `src/memory/sqlite.rs` | Store memory with session_id |
| `recall()` | `src/memory/sqlite.rs` | Search with session filter |
| `clear_history()` | `src/agent/agent.rs` | Clear conversation history |
| `trim_history()` | `src/agent/agent.rs` | Enforce max_history_messages |
