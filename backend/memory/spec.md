// memory SPEC
//
// =============================================================================
// IMPLEMENTATION STATUS
// =============================================================================
//
// ✅ Implemented:
//   - Three-layer Memory (Longterm/Hot/Cold)
//   - MemoryCache with TTL-based caching
//   - ColdDB (SQLite + FTS5)
//   - HotMemory (JSON) - ActiveTopics, RecentKeywords
//   - LongtermMemory (Markdown)
//   - ContextBuilder with token budgeting
//   - Chinese tokenization with jieba
//
// 🚧 Partially Implemented:
//   - HotMemory.TopicSummaries: Type defined, generation not implemented
//   - Task Layer: Types defined, logic not implemented
//
// ⚠️ Known Limitations:
//   - Non-jieba fallback uses unicode61, not trigram
//   - GetHot/GetRecent return internal pointers (internal use only)
//
// =============================================================================
// Directory Structure
// =============================================================================
//
// backend/
// ├── memory/
// │   ├── spec.md            # This file
// │   ├── types.go           # Message, HotMemoryData, TaskInfo, etc.
// │   ├── cache.go           # MemoryCache (unified cache)
// │   ├── cold.go            // ColdDB (SQLite + FTS5)
// │   ├── hot.go             // HotMemory (JSON)
// │   ├── longterm.go        // LongtermMemory (Markdown)
// │   └── context.go         // ContextBuilder
// ├── providers/
// ├── main.go
// └── protocol/
//
// =============================================================================
// ARCHITECTURE DESIGN
// =============================================================================
//
// Layer 1: Memory System (MVP)
// ─────────────────────────────────────────────────────────────
// - Unified Message Model (Human + Time + Message)
// - Three-layer Memory: Longterm + Hot + Cold
// - MemoryCache: unified in-memory cache
// - ColdDB: SQLite + FTS5 for history + search
// - HotMemory: JSON for active topics/summaries
// - LongtermMemory: Markdown for identity/preferences
//
// Key Design Decisions:
// ─────────────────────────────────────────────────────────────
// 1. NO Session Entity
//    - Message belongs to Human, not Session
//    - Channel is just routing metadata
//    - Time is the core dimension
//
// 2. Unified Cache (MemoryCache)
//    - Caches Longterm, Hot, Recent messages
//    - File modification time for invalidation
//    - Fixed 20 messages for Sliding Window
//
// 3. Cold Memory = SQLite + FTS5
//    - Single source of truth for history
//    - FTS5 with Chinese tokenization (jieba/trigram)
//    - No separate JSONL files
//
// 4. Hot Memory = JSON
//    - Active topics + summaries + keywords
//    - 7-day TTL
//    - Always loaded (but may be empty)
//
// 5. Location as Regular Message
//    - Not metadata (avoids staleness)
//    - User shares when needed
//    - Tool calling for location request
//
// =============================================================================
// MODULE SPEC: memory
// =============================================================================
//
// RELY:
//   - Go standard library: database/sql, encoding/json, log, os, sync, time, testing
//   - SQLite driver: github.com/mattn/go-sqlite3 (requires CGO + gcc)
//   - Chinese tokenization: github.com/yanyiwu/gojieba (requires CGO + g++)
//   - Internal logger: gobot/log (for structured logging)
//
// BUILD REQUIREMENTS:
//   - CGO_ENABLED=1
//   - gcc/g++ for native compilation
//   - Docker: needs build-base or gcc-multilib
//
// TEST CONSTRAINTS:
//   - Use only standard library testing package
//   - NO third-party test frameworks (testify, ginkgo, etc.)
//   - Each test MUST use t.TempDir() for isolation
//   - Each test MUST defer cache.Close() to prevent leaks
//
// GUARANTEE:
//   - Unified cache for Longterm + Hot + Recent
//   - FTS5 full-text search for history
//   - Concurrent-safe operations
//   - Atomic file writes (tmp + rename)
//   - Single goroutine for HotMemory updates (no race conditions)
//   - Chinese tokenization support for FTS5
//   - Token budget management for context building
//
// IMPLEMENTATION NOTES (Critical):
// ─────────────────────────────────────────────────────────────
// 1. LIFECYCLE MANAGEMENT
//    - Must implement Close() for graceful shutdown
//    - Close channels, wait for WaitGroups, close DB connections
//    - Prevent goroutine leaks and SQLite file locks
//
// 2. CONCURRENCY SAFETY
//    - Use pointer-swapping with sync.RWMutex for hotData
//    - NEVER modify shared data in-place (Copy-on-Write)
//    - Worker creates new HotMemoryData, swaps pointer under lock
//    - Readers use RLock() to get consistent snapshot
//
// 3. DATA SERIALIZATION
//    - JSON marshal HumanIDs before SQLite INSERT
//    - JSON unmarshal after SELECT
//    - Atomic file writes: write to tmp, then rename
//
// 4. TESTING REQUIREMENTS
//    - Use t.TempDir() for isolation (never hardcode paths)
//    - Table-driven tests for multiple scenarios
//    - Mock time for TTL tests (override package-level variable)
//    - Always defer cache.Close() in tests
//    - Use go test -race to verify concurrency safety
//
// 5. EDGE CASES
//    - Auto-create directories if not exist (os.MkdirAll)
//    - Return empty/zero values for missing files
//    - Validate inputs early (fail fast)

// =============================================================================
// File: types.go
// =============================================================================

// --- Message Model ---

type Message struct {
    ID          string    `json:"id"`
    Content     string    `json:"content"`
    Timestamp   time.Time `json:"timestamp"`
    HumanIDs    []string  `json:"humanIDs"`
    Channel     string    `json:"channel"`     // "discord", "telegram", "web"
    ChatID      string    `json:"chatID"`      // Group/Private chat ID
    IsFromHuman bool      `json:"isFromHuman"`
    Type        string    `json:"type,omitempty"` // "text", "location"
}

// TIMESTAMP RULE: All timestamps are stored in UTC
// - SQLite stores as "2006-01-02 15:04:05" (no timezone, interpreted as UTC)
// - time.Parse() returns UTC for timezone-free strings
// - Display layer should convert to user's local timezone

// STORAGE RULE: HumanIDs in SQLite
// - Before INSERT: json.Marshal(HumanIDs) → string
// - After SELECT: json.Unmarshal(string) → []string
// - Example: ["yuan-001", "zhang-002"] → `["yuan-001","zhang-002"]`

// --- Hot Memory Types ---

type HotMemoryData struct {
    ActiveTopics    []Topic        `json:"activeTopics"`
    TopicSummaries  []TopicSummary `json:"topicSummaries"`
    RecentKeywords  []Keyword      `json:"recentKeywords"`
    LastUpdated     time.Time      `json:"lastUpdated"`
}

type Topic struct {
    Name       string    `json:"name"`
    LastActive time.Time `json:"lastActive"`
    Count      int       `json:"count"`
}

type TopicSummary struct {
    Topic      string   `json:"topic"`
    Summary    string   `json:"summary"`
    KeyPoints  []string `json:"keyPoints"`
    LastActive time.Time `json:"lastActive"`
    // NOTE: TopicSummary generation not yet implemented (Future)
    // Current implementation only populates ActiveTopics and RecentKeywords
}

type Keyword struct {
    Word       string    `json:"word"`
    LastActive time.Time `json:"lastActive"`
    Count      int       `json:"count"`
}

// --- Task Types (Layer 4, Future) ---

type TaskInfo struct {
    Name     string
    Type     string    // "project", "novel", "research", "plan"
    Status   string    // "active", "paused", "completed"
    Keywords []string
    Summary  string
}

type TaskSummary struct {
    Name      string
    Type      string
    Completed time.Time
    Summary   string
}

type TaskIndex struct {
    Active    map[string]TaskInfo
    Paused    map[string]TaskInfo
    Completed []TaskSummary
}

// --- Context Types ---

type Context struct {
    Longterm  string
    Hot       *HotMemoryData
    Recent    []Message
    TaskIndex *TaskIndex    // Future (Layer 4)
    Tasks     []string      // Future (Layer 4)
}

// =============================================================================
// File: cache.go
// =============================================================================

// --- MemoryCache ---

type MemoryCache struct {
    dataDir string
    
    // Longterm cache (immutable after load)
    longterm    string
    longtermMod time.Time
    
    // Hot cache (Copy-on-Write for thread safety)
    hotData    *HotMemoryData
    hotDataMu  sync.RWMutex  // Protects hotData pointer swap
    hotMod     time.Time
    
    // Sliding Window cache (fixed 20)
    recentMessages []Message
    recentMu       sync.RWMutex
    
    // Database
    coldDB *sql.DB
    
    // Hot update worker (single goroutine)
    hotUpdateChan chan Message
    hotWorkerWg   sync.WaitGroup
    stopChan      chan struct{}  // Graceful shutdown
    
    // Time function (for testing)
    nowFunc func() time.Time
}

// CONCURRENCY RULE: Hot Memory Updates
// - Worker creates NEW HotMemoryData object (Copy-on-Write)
// - Worker swaps pointer with hotDataMu.Lock()
// - Readers use hotDataMu.RLock() to get consistent snapshot
// - NEVER modify hotData in-place (causes data races)

// --- Constructor ---

// FUNC SPEC: NewMemoryCache
// File: cache.go
//
// PRE:
//   - dataDir is a valid directory path
//
// POST:
//   - Creates dataDir/memory/ if not exists (os.MkdirAll)
//   - Opens cold.db (creates if not exists)
//   - Enables WAL mode for cold.db
//   - Creates tables (messages, messages_fts)
//   - Loads initial cache:
//     - If longterm.md not exists: initializes empty string
//     - If hot.json not exists: initializes empty HotMemoryData
//     - Loads recent 20 from cold.db (or empty if no messages)
//   - Starts hot update worker goroutine
//   - Returns MemoryCache instance
//
// INTENT:
//   - Initialize unified memory cache with graceful handling of missing files
func NewMemoryCache(dataDir string) (*MemoryCache, error)

// FUNC SPEC: Close
// File: cache.go
//
// PRE:
//   - cache is initialized
//
// POST:
//   - Closes stopChan (signals worker to stop)
//   - Waits for hotWorkerWg to finish pending writes (up to 5s timeout)
//   - Closes coldDB connection
//   - Releases all resources
//   - Returns error if cleanup fails
//
// INTENT:
//   - Graceful shutdown (prevent goroutine leaks and DB locks)
//
// POST-CLOSE BEHAVIOR:
//   - After Close(), calling Append/Search/GetLongterm will return errors
//   - Error message comes from underlying DB ("sql: database is closed")
//   - This is acceptable: calling methods after Close is a programming error
//   - No explicit closed-state check needed (DB handles it)
func (c *MemoryCache) Close() error

// --- Getters ---

// FUNC SPEC: GetLongterm
// File: cache.go
//
// PRE:
//   - cache is initialized
//
// POST:
//   - Returns cached content if within TTL (30s)
//   - If TTL expired: checks file, reloads if changed
//   - TTL hit: ~40ns, TTL miss: ~5ms
//
// INTENT:
//   - Get longterm memory with TTL-based cache
func (c *MemoryCache) GetLongterm() (string, error)

// FUNC SPEC: GetHot
// File: cache.go
//
// PRE:
//   - cache is initialized
//   - Caller MUST NOT modify the returned data (internal use only)
//
// POST:
//   - Returns internal pointer (zero-copy)
//   - Performance: ~15ns
//
// INTENT:
//   - Get hot memory (always from cache)
func (c *MemoryCache) GetHot() (*HotMemoryData, error)

// FUNC SPEC: GetRecent
// File: cache.go
//
// PRE:
//   - cache is initialized
//   - Caller MUST NOT modify the returned slice (internal use only)
//
// POST:
//   - Returns internal slice (max 20 messages)
//   - Performance: ~20ns (zero-copy)
//
// INTENT:
//   - Get recent messages (Sliding Window)
//   - For internal use only (ContextBuilder)
func (c *MemoryCache) GetRecent() []Message

// --- Writers ---

// FUNC SPEC: AddMessage
// File: cache.go
//
// PRE:
//   - msg is valid (non-empty ID, Content, Timestamp)
//
// POST:
//   - Validates input (returns error if invalid)
//   - Updates recentMessages immediately (synchronous, ~100ns)
//   - Queues msg for async Cold + Hot update
//   - Returns immediately (~1µs) regardless of system load
//   - If queue full: drops message and logs warning
//
// INTENT:
//   - Add message to memory system (async, non-blocking)
func (c *MemoryCache) AddMessage(msg Message) error

// FUNC SPEC: Append
// Alias for AddMessage (backward compatibility)
func (c *MemoryCache) Append(msg Message) error

// --- Async Worker ---

// FUNC SPEC: startWorker
// File: cache.go
//
// PRE:
//   - cache is initialized
//   - updateChan is created
//
// POST:
//   - Runs in separate goroutine
//   - Batches messages (every 100ms or on stop)
//   - Batch writes to cold.db
//   - Updates hot memory (keywords, topics)
//   - Writes hot.json atomically
//   - On Close(): drains remaining messages before exit
//
// INTENT:
//   - Unified async processing for Cold + Hot updates
func (c *MemoryCache) startWorker()

// =============================================================================
// File: cold.go
// =============================================================================

// --- ColdDB Schema ---

// TABLE: messages
//   - id TEXT PRIMARY KEY
//   - content TEXT NOT NULL
//   - timestamp TEXT NOT NULL
//   - human_ids TEXT (JSON array)
//   - channel TEXT
//   - chat_id TEXT
//   - embedding BLOB (optional, future: float32 array for semantic search)
//
// INDEX: idx_messages_timestamp ON messages(timestamp DESC)
//
// FUTURE EXTENSION:
//   - When embedding service available, add sqlite-vec extension
//   - Hybrid search: FTS5 (keyword) + Vector (semantic)
//   - See: https://github.com/asg017/sqlite-vec
//
// VIRTUAL TABLE: messages_fts USING fts5(
//   content,
//   content='messages',
//   content_rowid='rowid',
//   tokenize='trigram' OR use jieba preprocessing
// )
//
// TRIGGERS: messages_ai, messages_ad, messages_au (auto-sync FTS)

// --- Search ---

// FUNC SPEC: Search
// File: cold.go
//
// PRE:
//   - query is non-empty
//   - cold.db is initialized
//
// POST:
//   - If Chinese: tokenizes query with jieba (if available) or uses trigram
//   - Searches messages_fts using FTS5 MATCH
//   - Ranks by BM25
//   - Returns up to 20 results
//   - Latency: ~50-100ms
//
// INTENT:
//   - Search conversation history with FTS5
func (c *MemoryCache) Search(query string) ([]Message, error)

// --- Tokenization ---

// FUNC SPEC: tokenizeChinese
// File: cold.go
//
// PRE:
//   - content is Chinese text
//
// POST:
//   - If jieba available: uses jieba.CutForSearch(), joins with spaces
//   - If jieba unavailable: returns original content (no tokenization)
//   - Example: "今天天气怎么样" → "今天 天气 怎么样"
//
// NOTE:
//   - Current fallback is raw content + unicode61 tokenizer
//   - NOT trigram as originally specified (trigram not always available in SQLite builds)
//   - For best Chinese search, build with `-tags jieba`
//
// INTENT:
//   - Tokenize Chinese text for FTS5 indexing
func tokenizeChinese(content string) string

// =============================================================================
// File: hot.go
// =============================================================================

// --- Hot Memory Operations ---

// FUNC SPEC: loadHot
// File: hot.go
//
// PRE:
//   - filePath is valid path to hot.json
//
// POST:
//   - If file not exists: returns empty HotMemoryData
//   - If file exists: reads and unmarshals JSON
//   - Returns error if JSON invalid
//
// INTENT:
//   - Load hot memory from disk
func loadHot(filePath string) (*HotMemoryData, error)

// FUNC SPEC: saveHot
// File: hot.go
//
// PRE:
//   - data is valid HotMemoryData
//   - filePath is valid path
//
// POST:
//   - Marshals data to JSON (with indentation)
//   - Writes to tmp file
//   - Renames tmp to final (atomic)
//   - Returns error if write fails
//
// INTENT:
//   - Save hot memory to disk atomically
func saveHot(filePath string, data *HotMemoryData) error

// --- Hot Memory Logic ---

// FUNC SPEC: updateKeywords
// File: hot.go
//
// PRE:
//   - hotData is initialized
//   - words is list of extracted keywords
//
// POST:
//   - For each word:
//     - If exists: increments count, updates LastActive
//     - If not exists: appends new Keyword
//   - No return (modifies hotData in-place)
//
// INTENT:
//   - Update keyword frequencies
func updateKeywords(hotData *HotMemoryData, words []string)

// FUNC SPEC: updateTopics
// File: hot.go
//
// PRE:
//   - hotData is initialized
//   - Keywords with count >= 3 are considered topics
//
// POST:
//   - For each high-frequency keyword:
//     - If topic exists: increments count, updates LastActive
//     - If not exists: appends new Topic
//   - No return (modifies hotData in-place)
//
// INTENT:
//   - Auto-cluster topics from keywords
func updateTopics(hotData *HotMemoryData)

// FUNC SPEC: cleanupExpired
// File: hot.go
//
// PRE:
//   - hotData is initialized
//
// POST:
//   - Removes Keywords older than 7 days
//   - Removes Topics older than 7 days
//   - Removes TopicSummaries older than 7 days
//   - No return (modifies hotData in-place)
//
// INTENT:
//   - Enforce 7-day TTL for hot memory
func cleanupExpired(hotData *HotMemoryData)

// FUNC SPEC: extractKeywords
// File: hot.go
//
// PRE:
//   - content is text
//
// POST:
//   - Removes punctuation: `strings.MapFunc(r, strings.TrimFunc)` with custom punct set
//   - Converts to lowercase: `strings.ToLower()`
//   - Splits by whitespace
//   - Filters stop words (common Chinese/English)
//   - Ignores single-character words
//   - Returns list of keywords (max 10)
//
// EXAMPLE:
//   "Hello, World! 今天天气怎么样？" → ["hello", "world", "今天", "天气", "怎么样"]
//
// INTENT:
//   - Extract keywords from text (simple implementation)
func extractKeywords(content string) []string

// =============================================================================
// File: longterm.go
// =============================================================================

// --- Longterm Memory Operations ---

// FUNC SPEC: loadLongterm
// File: longterm.go
//
// PRE:
//   - filePath is valid path to longterm.md
//
// POST:
//   - If file not exists: returns empty string
//   - If file exists: reads entire file
//   - Returns error if read fails
//
// INTENT:
//   - Load longterm memory from disk
func loadLongterm(filePath string) (string, error)

// FUNC SPEC: saveLongterm
// File: longterm.go
//
// PRE:
//   - content is Markdown text
//   - filePath is valid path
//
// POST:
//   - Writes content to tmp file
//   - Renames tmp to final (atomic)
//   - Returns error if write fails
//
// INTENT:
//   - Save longterm memory to disk atomically
func saveLongterm(filePath, content string) error

// =============================================================================
// File: context.go
// =============================================================================

// --- Context Builder ---

type ContextBuilder struct {
    cache     *MemoryCache
    maxTokens int
}

// FUNC SPEC: NewContextBuilder
// File: context.go
//
// PRE:
//   - cache is initialized
//   - maxTokens > 0 (e.g., 4000)
//
// POST:
//   - Returns ContextBuilder instance
//
// INTENT:
//   - Create context builder with token budget
func NewContextBuilder(cache *MemoryCache, maxTokens int) *ContextBuilder

// FUNC SPEC: Build
// File: context.go
//
// PRE:
//   - msg is current message
//   - builder is initialized
//
// POST:
//   - Loads Longterm (P0, never truncate)
//   - Loads Hot (P4)
//   - Loads Recent 20 (P2-P3)
//   - Loads matching Tasks (P1) (Future)
//   - Counts tokens for each part
//   - If total tokens > maxTokens:
//     - Step 1: Completely drop P4 (Hot) if saves >= 20% tokens
//     - Step 2: If still exceeds, drop oldest from Recent (P3 → P2) one by one
//     - Step 3: Never drop P0 (Longterm) or P1 (Active Task)
//   - Returns Context
//   - Latency: ~0.5-5ms (depending on cache hits)
//
// TRUNCATION ALGORITHM:
//   1. Calculate totalTokens
//   2. If totalTokens <= maxTokens: return all parts
//   3. If totalTokens > maxTokens:
//      a. Remove P4 (Hot), recalculate
//      b. If still > maxTokens:
//         - Remove oldest message from Recent (P3)
//         - Repeat until fits or only P0+P1 remains
//
// INTENT:
//   - Build context with token budget management
func (b *ContextBuilder) Build(msg Message) (*Context, error)

// --- Token Counting ---

// FUNC SPEC: countTokens
// File: context.go
//
// PRE:
//   - text is string
//
// POST:
//   - Returns approximate token count
//   - Simple implementation: len(text) / 4 (Chinese) or / 3 (English)
//   - Future: use tiktoken-go for accurate count
//
// INTENT:
//   - Estimate token count for context management
func countTokens(text string) int

// =============================================================================
// Design Notes
// =============================================================================
//
// 1. Why single goroutine for Hot updates?
//    - Avoids race conditions in memory state
//    - Enables throttling/debouncing
//    - Simple and reliable
//
// 2. Why FTS5 with jieba instead of external search service?
//    - SQLite is embedded, no extra dependency
//    - FTS5 is mature and fast
//    - jieba provides good Chinese tokenization
//
// 3. Why fixed 20 for Sliding Window?
//    - Simple, no LRU complexity
//    - 20 messages is usually enough for context
//    - Small memory footprint (~10KB)
//
// 4. Why file modification time for cache invalidation?
//    - Simple and reliable
//    - No need for explicit invalidation
//    - Works with external edits (human can edit longterm.md)
//
// 5. Why no Search cache?
//    - Search is low frequency (~5-10% of requests)
//    - Queries vary (hard to hit cache)
//    - 50-100ms latency is acceptable
//
// 6. Extending to multi-tenant:
//    - Add humanID parameter to NewMemoryCache
//    - Use dataDir/{humanID}/ for isolation
//    - cold.db can be shared (filter by human_ids)

// =============================================================================
// TEST SPEC (For AI Generation)
// =============================================================================
//
// 1. ISOLATION:
// - Every test interacting with disk MUST use `t.TempDir()` as `dataDir`.
// - Do not hardcode paths like "./data".
// - Example: `dataDir := t.TempDir()`
//
// 2. LIFECYCLE:
// - Every test instantiating `MemoryCache` MUST `defer cache.Close()`.
// - Prevents SQLite DB locks and goroutine leaks across tests.
// - Example:
//   ```go
//   cache, err := NewMemoryCache(t.TempDir())
//   require.NoError(t, err)
//   defer cache.Close()
//   ```
//
// 3. CONCURRENCY & RACE CONDITIONS:
// - Write a `TestMemoryCache_ConcurrentAppend` that spawns 100 goroutines,
//   each calling `Append()` simultaneously.
// - Run tests with `go test -race` to ensure no data races occur.
// - Verify:
//   - `recentMessages` slice is thread-safe
//   - `hotData` pointer swap is atomic (no stale reads)
//
// 4. TABLE-DRIVEN TESTS:
// - Use Table-Driven patterns for `ContextBuilder.Build()` to test:
//   - Exactly at budget (should pass)
//   - Slightly over budget (should drop P4)
//   - Extreme over budget (should drop P4 + oldest Recent)
//   - Impossible to fit (should error or return minimal context)
// - Example:
//   ```go
//   tests := []struct {
//       name      string
//       maxTokens int
//       messages  int
//       wantDrops int  // How many parts should be dropped
//   }{
//       {"exact_fit", 1000, 10, 0},
//       {"drop_hot", 500, 10, 1},
//       {"drop_recent", 200, 10, 5},
//   }
//   ```
//
// 5. MOCKING TIME:
// - Test `cleanupExpired` (7-day TTL) by mocking time.
// - Option A: Override package-level variable:
//   ```go
//   oldNow := nowFunc
//   nowFunc = func() time.Time { return fixedTime }
//   defer func() { nowFunc = oldNow }()
//   ```
// - Option B: Use interface-based time provider.
//
// 6. INITIALIZATION EDGE CASES:
// - Test `NewMemoryCache` when:
//   - dataDir does not exist (should create)
//   - longterm.md does not exist (should return empty string)
//   - hot.json does not exist (should return empty data)
//   - cold.db does not exist (should create tables)
//
// 7. JSON SERIALIZATION:
// - Test that `HumanIDs` is correctly marshaled/unmarshaled in SQLite:
//   - Insert message with `["a", "b"]`
//   - Query message
//   - Assert `HumanIDs` equals `["a", "b"]`
//
// 8. CHINESE TOKENIZATION:
// - Test FTS5 search with Chinese queries:
//   - Insert: "今天天气怎么样"
//   - Search: "天气"
//   - Assert: finds the message
//
// 9. PERFORMANCE BENCHMARKS:
// - Benchmark `ContextBuilder.Build()` with varying message counts.
// - Ensure P99 latency < 10ms.
//
