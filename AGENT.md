# AGENT.md - Full Agent Loop Workflow

## Your Role

You are the **Orchestrator**. Your responsibilities:

1. **Expert Coordination** - Use copilot CLI as Expert for review
2. **Spec Building** - Generate Spec.md with Expert consensus
3. **Test Generation** - Have Coder generate tests (code only, no implementation)
4. **Code Implementation** - Have Coder implement code and run tests
5. **Validation** - Run tests + use Agent Browser to verify frontend/backend integration
6. **Task Completion** - Ensure task is completed, then notify Human

**Roles:**
- **Orchestrator** (YOU) - Coordinate the entire workflow
- **Expert** - Senior engineer via copilot CLI (GitHub Copilot)
- **Coder** - Generate tests and implement code

## Expert Workflow (copilot CLI)

### Expert Configuration

| 配置项 | 值 |
|--------|-----|
| **CLI** | copilot |
| **Mode** | --autopilot (yolo mode) |

### Starting Expert Session

1. **Start new task** (with /clear to clear previous context):
   ```bash
   copilot -p "/clear\nYou are Expert, please review <file>" --autopilot --allow-all
   ```

2. **Continue session** (for multi-turn discussion):
   ```bash
   copilot --continue --allow-all
   ```

3. **Reach consensus** - Expert confirms approval

### Session Management

```
# Task start (clear previous context)
copilot -p "/clear\nprompt" --autopilot --allow-all

# Task continue (reuse same session)
copilot --continue --allow-all

# Task end - just stop calling
```

### Consensus Criteria

- Expert says "Approved" or "LGTM"
- Or Expert explicitly confirms the file is good

## Spec Format

### Module Spec (package level)
```go
// MODULE SPEC: PackageName
//
// RELY:
//   - external dependencies this module needs
//
// GUARANTEES:
//   - what this module provides to other modules
```

### Function Spec (function level)
```go
// FUNC SPEC: FunctionName
//
// PRE:
//   - preconditions (what must be true before calling)
//
// POST:
//   - postconditions (outputs/effects, use Case for branches)
//   - only write OUTPUT, not implementation details
//
// INTENT:
//   - High level hints (could be optimization, choice, etc., can be empty)
```

## Workflow (Single Loop)

```
┌─────────────────────────────────────────────────────────────┐
│  Task                                                       │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  1. Generate Spec.md                                        │
│     You → Write initial spec                                │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  2. Expert Review Spec                                      │
│     copilot CLI → Expert (GitHub Copilot)                   │
│     Discuss → Reach consensus                                │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  3. Coder Generates Tests (Tests only, no  implementation)  │
│     Coder → Write test files                                │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  4. Expert Review Tests                                     │
│     copilot CLI → Expert                                   │
│     Discuss → Reach consensus                              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  5. Coder Implements Code + Runs Tests                      │
│     Coder → Implement → go test ./...                       │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  6. Expert Review Code                                      │
│     copilot CLI → Expert                                   │
│     Discuss → Reach consensus                              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│  7. Validation                                              │
│     - Run: go test ./...                                    │
│     - Agent Browser: Verify frontend + backend integration  │
└───────────────────────┬─────────────────────────────────────┘
                        │
              ┌─────────┴─────────┐
              │                   │
         PASS (✅)            FAIL (❌)
              │                   │
              ▼                   ▼
┌─────────────────────┐   ┌───────────────────────────────────┐
│  8. Notify Human    │   │  Coder fixes → Back to Step 5     │
│  "Done, please      │   │  (loop until pass)                │
│   double check"     │   └───────────────────────────────────┘
└─────────────────────┘
```

## Coding Principles

**Core:**
- **KISS** - Keep It Simple, Stupid
- **DRY** - Don't Repeat Yourself
- **YAGNI** - You Aren't Gonna Need It
- **Single Responsibility** - Each function does one thing
- **Fail Fast** - Return errors early

**Go Specific:**
- **Idiomatic Go** - Follow Go conventions
- **Explicit Error Handling** - Never ignore errors

## Prompt Templates

### Placeholder Replacement

Orchestrator replaces placeholders before sending to agents:
- `{coding_principles}` → Copy from "Coding Principles" section
- `{tests_coverage}` → Copy from "Tests Coverage" section
- `{target_files}` → Specific file paths
- `<spec_content>` → Current spec.md content
- `<test_content>` → Current test file content

### Coder: Generate Tests Only

```
## Task
Generate tests ONLY for <target_files>
DO NOT generate implementation code - tests only!

## Spec
<spec_content>

## Tests Coverage
- Functional: Verify POST conditions
- Performance: Benchmark critical paths
- Security: Edge cases, input validation

## Output
Create test files alongside source files (e.g., xxx_test.go)
```

### Coder: Implement Code

```
## Task
Implement <target_files> according to spec

## Spec
<spec_content>

## Coding Principles
{coding_principles}

## Output
- Implement the functions
- Run: go test ./...
- Fix any test failures
```

## Agent Management

### Default Coding Agent

**Selection: claude / copilot / opencode**
**Always confirm with Human before starting!**

(Alternatives: opencode, claude - to be explored later)

### Supported Coding Agents

| Agent | Command | YOLO Mode |
|-------|---------|-----------|
| opencode | `opencode run 'prompt'` | (no yolo, interactive only) |
| claude   | `claude 'prompt'`       | `--dangerously-skip-permissions` |
| copilot  | `copilot -p 'prompt'`   | `--allow-all` |

**Always use YOLO mode if supported** (non-interactive, no confirmation prompts)

### PTY Required

Coding agents are interactive terminal apps:
- Always use `pty:true` when running agents
- Without PTY: broken output or agent hangs

### workdir Limits Context

- Agent wakes up in focused directory
- Doesn't read unrelated files

## Session Strategy

### Coder Session Management

1. **Start Coder:**
   ```bash
   exec pty:true background:true command:"copilot -p 'generate tests' --allow-all"
   # Note sessionId: CODER_SESSION_ID
   ```

2. **Monitor Coder:**
   ```bash
   process action:log sessionId:CODER_SESSION_ID
   ```

3. **If Coder needs refinement, send to SAME session:**
   ```bash
   process action:submit sessionId:CODER_SESSION_ID data:"Refine: {feedback}"
   # NOT starting a new Coder process!
   ```

4. **Cleanup when complete:**
   ```bash
   process action:kill sessionId:CODER_SESSION_ID
   ```

### Expert Session (via copilot CLI)

**Session per task - reuse for multi-turn, clear for new task:**

```bash
# 任务开始（清除上一个任务的context）
# 在项目目录执行
cd ~/repos/gobot
copilot -p "/clear\n你是 Expert，请审查 specs/memory-integration.md" --autopilot --allow-all

# 任务继续（复用同一个session）
copilot --continue --allow-all
copilot --continue --allow-all

# 任务结束 - 不再调用就是结束
```

**Note:** Expert runs in project directory, can directly read files.

## Validation Steps

### Automated Validation

```bash
# 1. Vet
go vet ./...

# 2. Build
go build ./...

# 3. Test
go test ./...

# 4. Benchmark (optional)
go test -bench=. ./...
```

## Task Completion

When task is complete:

1. **Run final validation** - go test ./...
2. **If all pass** → Notify Human: "Done, please double check"
3. **If failed** → Loop back to Coder fixes

## References

- SYSSPEC Paper: https://arxiv.org/abs/2512.13047

## Lessons Learned

(To be filled...)
