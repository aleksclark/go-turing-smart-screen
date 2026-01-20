# Agent Status Reporting Standard

A simple, UNIX-philosophy approach to coding agent status reporting using the filesystem.

## Overview

Agents report their status by writing JSON files to a shared directory. Monitors read these files to display agent activity. No daemons, sockets, or infrastructure required.

## Quick Start

Write a status file:

```bash
mkdir -p ~/.agent-status
cat > ~/.agent-status/my-agent-$$.json << 'EOF'
{
  "v": 1,
  "agent": "my-agent",
  "instance": "abc123",
  "status": "working",
  "task": "implementing feature",
  "updated": $(date +%s)
}
EOF
```

Read active agents:

```bash
find ~/.agent-status -name '*.json' -mmin -5 -exec cat {} \; | jq -s '.[] | "\(.agent): \(.status)"'
```

## Directory Structure

```
~/.agent-status/
├── crush-a1b2c3.json
├── cursor-d4e5f6.json
└── claude-code-g7h8i9.json
```

**Location**: `~/.agent-status/` (or `$AGENT_STATUS_DIR` if set)

**Filename**: `{agent_type}-{instance_id}.json`
- `agent_type`: lowercase identifier (e.g., `crush`, `cursor`, `claude-code`, `aider`, `copilot`)
- `instance_id`: unique identifier for this instance (PID, UUID prefix, or session hash)

## JSON Schema

A formal JSON Schema is available at [`agent-status.schema.json`](./agent-status.schema.json).

### Validation

```bash
# Using ajv-cli
npx ajv validate -s agent-status.schema.json -d ~/.agent-status/*.json

# Using check-jsonschema
check-jsonschema --schemafile agent-status.schema.json ~/.agent-status/*.json

# Using Go package
go run github.com/aleksclark/go-turing-smart-screen/cmd/validate-status@latest ~/.agent-status/*.json
```

### Go API

```go
import "github.com/aleksclark/go-turing-smart-screen/pkg/agentstat"

// Read all active agents (max 5 min old)
statuses, _ := agentstat.ReadAll(5 * time.Minute)
for _, s := range statuses {
    fmt.Printf("%s: %s - %s\n", s.Agent, s.Status, s.Task)
}

// Read with error reporting for invalid files
statuses, errors, _ := agentstat.ReadAllWithErrors(5 * time.Minute)
for _, e := range errors {
    log.Printf("invalid file %s: %v", e.File, e.Err)
}

// Validate a status struct
status := agentstat.Status{
    Version:  1,
    Agent:    "my-agent",
    Instance: "abc123",
    Status:   "working",
    Updated:  time.Now().Unix(),
}
if err := status.Validate(); err != nil {
    log.Printf("validation error: %v", err)
}

// Get all validation errors at once
if errs := status.ValidateAll(); len(errs) > 0 {
    for _, err := range errs {
        log.Printf("  %v", err)
    }
}
```

## File Format

```json
{
  "v": 1,
  "agent": "crush",
  "instance": "a1b2c3",
  "pid": 12345,
  "project": "turing-smart-screen-python",
  "cwd": "/home/user/work/turing-smart-screen-python",
  "status": "working",
  "task": "implementing agent status monitor",
  "model": "claude-sonnet-4-20250514",
  "provider": "anthropic",
  "tools": {
    "active": "edit",
    "recent": ["view", "grep", "edit"],
    "counts": {
      "edit": 5,
      "view": 12,
      "grep": 3,
      "bash": 2
    }
  },
  "tokens": {
    "input": 125000,
    "output": 15000,
    "cache_read": 80000,
    "cache_write": 45000
  },
  "cost_usd": 0.42,
  "started": 1737276000,
  "updated": 1737276300
}
```

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `v` | int | Schema version (currently `1`) |
| `agent` | string | Agent type identifier |
| `instance` | string | Unique instance identifier |
| `status` | string | Current status (see Status Values) |
| `updated` | int | Unix timestamp of last update |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `pid` | int | Process ID of the agent |
| `project` | string | Project/repo name |
| `cwd` | string | Current working directory |
| `task` | string | Human-readable current task description |
| `model` | string | AI model identifier (e.g., `claude-sonnet-4-20250514`, `gpt-4o`) |
| `provider` | string | API provider (`anthropic`, `openai`, `bedrock`, `vertex`, `ollama`, `local`) |
| `tools` | object | Tool usage information (see Tools Object) |
| `tokens` | object | Token usage counters (see Tokens Object) |
| `cost_usd` | float | Estimated cost in USD for this session |
| `started` | int | Unix timestamp when agent session started |
| `error` | string | Error message (when status is `error`) |
| `context` | object | Additional context (agent-specific, freeform) |

### Status Values

| Status | Description |
|--------|-------------|
| `idle` | Agent is running but waiting for user input |
| `thinking` | Agent is processing/reasoning (no tool calls yet) |
| `working` | Agent is actively executing tools |
| `waiting` | Agent is waiting for external resource (API, build, test) |
| `error` | Agent encountered an error |
| `done` | Agent completed its task |
| `paused` | Agent is paused by user |

### Tools Object

```json
{
  "active": "edit",
  "recent": ["view", "grep", "edit"],
  "counts": {
    "edit": 5,
    "view": 12,
    "grep": 3,
    "bash": 2
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `active` | string\|null | Currently executing tool (null if none) |
| `recent` | string[] | Last N tools used (most recent last), max 10 |
| `counts` | object | Map of tool name to invocation count this session |

**Common tool names** (normalize to these when possible):
- `view`, `edit`, `write`, `delete` - file operations
- `bash`, `shell`, `terminal` - command execution
- `grep`, `glob`, `find` - search operations
- `web_fetch`, `web_search` - web access
- `mcp_*` - MCP tool calls (preserve full name)

### Tokens Object

```json
{
  "input": 125000,
  "output": 15000,
  "cache_read": 80000,
  "cache_write": 45000
}
```

| Field | Type | Description |
|-------|------|-------------|
| `input` | int | Total input tokens consumed |
| `output` | int | Total output tokens generated |
| `cache_read` | int | Tokens read from cache (Anthropic) |
| `cache_write` | int | Tokens written to cache (Anthropic) |

All token fields are cumulative for the session.

## Writing Status Files

### Atomic Writes

Always write atomically to prevent partial reads:

```bash
# Write to temp file, then rename
tmp=$(mktemp ~/.agent-status/.tmp.XXXXXX)
echo "$json" > "$tmp"
mv "$tmp" ~/.agent-status/myagent-abc123.json
```

### Update Frequency

- Update on status change (idle → working → done)
- Update on tool start/completion
- Update at least every 30 seconds during active work
- Update token counts periodically (every ~10 tool calls or 30 seconds)

### Cleanup on Exit

Agents SHOULD delete their status file on clean exit:

```bash
trap 'rm -f ~/.agent-status/myagent-$$.json' EXIT
```

## Reading Status Files

### Freshness

Files are considered:
- **Active**: updated within last 60 seconds
- **Stale**: updated 60-300 seconds ago (agent may be hung or busy)
- **Dead**: updated >300 seconds ago (agent crashed without cleanup)

### Recommended Monitor Behavior

1. Read all `*.json` files in status directory
2. Parse JSON, skip malformed files
3. Filter by freshness (show active + stale, dim stale entries)
4. Sort by `updated` descending (most recent first)
5. Clean up dead files (>1 hour old) periodically

### Example Reader (Shell)

```bash
#!/bin/bash
# List active agents
find ~/.agent-status -name '*.json' -mmin -5 -exec cat {} \; | \
  jq -s 'sort_by(-.updated) | .[] | "\(.agent): \(.status) - \(.task // "no task")"'
```

### Example Reader (Python)

```python
import json
import time
from pathlib import Path

def get_agent_statuses(max_age=300):
    status_dir = Path.home() / '.agent-status'
    if not status_dir.exists():
        return []
    
    now = time.time()
    statuses = []
    
    for f in status_dir.glob('*.json'):
        try:
            data = json.loads(f.read_text())
            age = now - data.get('updated', 0)
            if age < max_age:
                data['_age'] = age
                data['_stale'] = age > 60
                statuses.append(data)
        except (json.JSONDecodeError, IOError):
            continue
    
    return sorted(statuses, key=lambda x: x.get('updated', 0), reverse=True)
```

## Shell Helper Library

Agents can source this helper for easy status reporting:

```bash
# ~/.agent-status.sh

AGENT_STATUS_DIR="${AGENT_STATUS_DIR:-$HOME/.agent-status}"

agent_status_init() {
    local agent_type="$1"
    local instance="${2:-$$}"
    
    mkdir -p "$AGENT_STATUS_DIR"
    export AGENT_STATUS_FILE="$AGENT_STATUS_DIR/${agent_type}-${instance}.json"
    export AGENT_STATUS_TYPE="$agent_type"
    export AGENT_STATUS_INSTANCE="$instance"
    export AGENT_STATUS_STARTED="$(date +%s)"
    
    trap 'rm -f "$AGENT_STATUS_FILE"' EXIT
}

agent_status_update() {
    local status="$1"
    local task="${2:-}"
    local model="${3:-}"
    local tool="${4:-}"
    
    local json=$(cat <<EOF
{
  "v": 1,
  "agent": "$AGENT_STATUS_TYPE",
  "instance": "$AGENT_STATUS_INSTANCE",
  "pid": $$,
  "status": "$status",
  "task": "$task",
  "model": "$model",
  "tools": {"active": ${tool:+\"$tool\"}${tool:-null}},
  "started": $AGENT_STATUS_STARTED,
  "updated": $(date +%s)
}
EOF
)
    
    local tmp=$(mktemp "$AGENT_STATUS_DIR/.tmp.XXXXXX")
    echo "$json" > "$tmp"
    mv "$tmp" "$AGENT_STATUS_FILE"
}

# Usage:
# source ~/.agent-status.sh
# agent_status_init "myagent" "session123"
# agent_status_update "working" "fixing bug" "claude-sonnet-4-20250514" "edit"
# agent_status_update "done"
```

## Integration Examples

### Crush/Claude Code

Report status on each tool call and thinking phase:

```
thinking → status: "thinking", tools.active: null
tool_use → status: "working", tools.active: "edit"
tool_result → status: "working", tools.active: null, tools.recent += "edit"
end_turn → status: "idle"
```

### Cursor

Report status based on composer state:

```
composer_open → status: "idle"
generating → status: "thinking"
applying_changes → status: "working", tools.active: "edit"
waiting_for_acceptance → status: "waiting"
```

### Aider

Report status based on command processing:

```
prompt → status: "idle"
sending_to_api → status: "thinking"
applying_edits → status: "working"
running_tests → status: "waiting", task: "running tests"
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_STATUS_DIR` | `~/.agent-status` | Directory for status files |
| `AGENT_STATUS_DISABLE` | unset | If set to `1`, disable status reporting |

## Security Considerations

- Status files contain project paths and task descriptions
- Files are readable by the user only (mode 0600 recommended)
- Avoid including sensitive data in `task` or `context` fields
- Consider `AGENT_STATUS_DIR` on tmpfs for ephemeral storage

## Versioning

The `v` field indicates schema version. Readers should:
- Accept `v: 1` (current)
- Ignore unknown fields (forward compatibility)
- Skip files with unsupported versions

Future versions will increment `v` and document migration.
