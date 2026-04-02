<div align="center">
  <h1>dalcenter</h1>
  <p><strong>Dal lifecycle manager — wake, sleep, sync AI agent containers</strong></p>
  <p>
    <a href="https://github.com/dalsoop/dalcenter"><img src="https://img.shields.io/badge/github-dalsoop%2Fdalcenter-181717?logo=github&logoColor=white" alt="GitHub repository"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-2563eb.svg" alt="AGPL-3.0 License"></a>
  </p>
  <p><a href="./README.ko.md">한국어</a></p>
</div>

dalcenter manages dal (AI puppets) — Docker containers with Claude Code, Codex, or Gemini installed, each with their own skills, instructions, and git identity. Templates live in git (localdal), dalcenter handles the runtime.

## Architecture

```
LXC: dalcenter
├── dalcenter serve              HTTP API + Docker management
│   ├── repo-watcher             git fetch/pull every 2min → auto sync on .dal/ changes
│   ├── cred-watcher             token expiry detection → auto refresh
│   ├── scheduled-dalroot        pipeline surveillance (orphan issues, stale PRs, merge directives)
│   ├── ops-watcher              multi-team health polling → auto wake empty teams
│   ├── leader-watcher           leader health check → auto recovery
│   └── issue-watcher            GitHub issue polling → auto wake on issue link
│
├── Docker: leader (claude)      dalcli-leader inside — team routing & task assignment
├── Docker: dev (claude)         dalcli inside — core development
├── Docker: reviewer (codex)     dalcli inside — independent code review
└── Docker: dev-2 (claude)       multiple instances supported

dalbridge (CT or same host)
└── MM outgoing webhook → SSE stream relay (:4280)

dalroot (Proxmox host)
└── Claude Code with infrastructure management charter
    ├── dalroot-listener          SSE stream → inbox file delivery
    ├── dalroot-check-notifications  inbox reader (Claude Code hook)
    └── dalroot-register          session registration + MM identity
```

### Binaries

| Binary | Location | Role |
|---|---|---|
| `dalcenter` | LXC host | Operator — infrastructure + lifecycle management |
| `dalcli-leader` | leader container | Team lead — member management + task assignment |
| `dalcli` | member containers | Team member — status + reporting |
| `dalbridge` | CT or same host | MM webhook → SSE stream relay |

### Scope Matrix

```
                    dalcenter (operator)   dalcli-leader (lead)    dalcli (member)
Infrastructure
  serve             O                     -                       -
  init              O                     -                       -
  validate          O                     -                       -
Lifecycle
  wake              O (all)               O (own team)            -
  sleep             O (all)               O (own team)            -
Observation
  ps                O (all)               O (own team)            O (own team)
  status            O (all)               O (own team)            O (self only)
  logs              O (all)               O (own team)            -
  attach            O (all)               O (own team)            -
Sync
  sync              O                     O                       -
Collaboration (Mattermost)
  assign            -                     O (to members)          -
  report            -                     -                       O (to leader)
```

## Dal Roles

Each dal runs in its own Docker container with a specific charter. Templates in `.dal/` (active roles) and `.dal/template/` (reusable cross-team templates).

### Active Roles

| Role | Player | Description |
|------|--------|-------------|
| **leader** | claude | Issue analysis, task routing, PR merge authority. Routes work to specialists via `dalcli-leader assign`. |
| **dev** | claude | Core Go development — features, bug fixes, refactoring. Works on `internal/`, `cmd/`, `dockerfiles/`. |
| **reviewer** | codex | Independent code review from a different AI perspective. Security, concurrency, simplicity checks. |
| **tester** | claude | Test strategy — unit tests, smoke tests, coverage analysis. No production code changes. |
| **verifier** | claude | Automated validation — `go vet`, `go test`, `go build`, `dalcenter validate`. Detects regressions. |
| **scribe** | claude | Shared memory manager — merges inbox → `decisions.md`/`wisdom.md`, compresses history, auto-commits. |
| **codex-dev** | codex | Alternative developer using Codex for a different implementation perspective. |
| **host** | - | User's representative across all teams. Coordinates via `#host` channel. |

### Template Roles (`.dal/template/`)

Cross-team roles synced to all repositories by config-manager.

| Role | Description |
|------|-------------|
| **auditor** | Monitors dalroot for guardrail violations (direct infra manipulation, unauthorized merges). Reports to `dal-control` channel, auto-creates GitHub issues. |
| **config-manager** | Syncs `.dal/template/` to all team repos. Detects template drift, creates sync PRs, audits tool installations. |
| **dalops** | CCW-based workflow orchestrator. Runs coded workflows (`workflow-lite-plan`, `workflow-tdd-plan`, etc.). |
| **test-writer** | Auto-writes tests for PR changes. Scans open PRs for `.go` files missing `_test.go` companions. |
| **dal-infra** | Infrastructure dal for host-level operations (binary builds, systemctl, credential sync). |
| **dal** (base) | Base template with common charter principles shared by all roles. |

## API Endpoints

Default port: `:11190`. Write endpoints require Bearer token (`DALCENTER_TOKEN`).

### Read (no auth)

| Endpoint | Description |
|----------|-------------|
| `GET /api/health` | Server health + connected client count |
| `GET /api/ps` | List all containers (name, UUID, player, role, idle_for, status) |
| `GET /api/status` | All dal statuses |
| `GET /api/status/{name}` | Single dal status (git diff, last activity, idle duration) |
| `GET /api/logs/{name}` | Docker container logs |
| `GET /api/tasks` | List all tasks |
| `GET /api/task/{id}` | Task detail (output, events, verification) |
| `GET /api/claims` | Claims submitted by dals |
| `GET /api/claims/{id}` | Single claim detail |
| `GET /api/feedback` | Feedback entries |
| `GET /api/feedback/stats` | Feedback statistics |
| `GET /api/costs` | Cost tracking entries |
| `GET /api/costs/summary` | Cost summary |
| `GET /api/escalations` | Escalation list |
| `GET /api/issues` | GitHub issue tracking |
| `GET /api/provider-status` | Provider circuit breaker state (claude/codex/gemini) |
| `GET /api/issue-workflow/{id}` | Issue workflow status |
| `GET /api/agent-config/{name}` | Agent configuration (CLAUDE.md, skills, git config) |
| `GET /runs/{id}` | HTML page for task run result |

### Write (Bearer token required)

| Endpoint | Description |
|----------|-------------|
| `POST /api/wake/{name}` | Start dal container (optional `--issue` for branch tracking) |
| `POST /api/sleep/{name}` | Stop dal container |
| `POST /api/restart/{name}` | Restart dal (sleep → wake) |
| `POST /api/replace/{name}` | Replace Docker image (hard restart) |
| `POST /api/sync` | Sync .dal/ changes to all running containers |
| `POST /api/validate` | Validate localdal CUE schema + skill references |
| `POST /api/message` | Relay message to Mattermost (from, message) |
| `POST /api/activity/{name}` | Record dal activity (idle tracking) |
| `POST /api/task` | Create direct task (dal, task) |
| `POST /api/task/start` | Start task execution |
| `POST /api/task/{id}/event` | Add event to task (kind, message) |
| `POST /api/task/{id}/metadata` | Update task metadata (git_diff, verified, completion) |
| `POST /api/task/{id}/finish` | Mark task complete (status, output, error) |
| `POST /api/claim` | File claim (type: env/blocked/bug/improvement) |
| `POST /api/claims/{id}/respond` | Respond to claim (resolution, notes) |
| `POST /api/feedback` | Submit feedback (positive/neutral/negative) |
| `POST /api/cost` | Record cost entry |
| `POST /api/escalate` | Create escalation (title, detail, priority) |
| `POST /api/escalations/{id}/resolve` | Resolve escalation |
| `POST /api/provider-trip` | Manually trip provider circuit breaker |
| `POST /api/issue-workflow` | Trigger issue workflow (issue_id, member, task) |

## dalbridge — MM Webhook-to-SSE Relay

dalbridge receives Mattermost outgoing webhooks and broadcasts them as Server-Sent Events (SSE) for dal containers and dalroot to subscribe.

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/webhook` | POST | Receive MM outgoing webhook, normalize, broadcast to SSE clients |
| `/stream` | GET | SSE stream (optional `?gateway=<name>` filter) |
| `/api/message` | POST | Relay dal→stream message (from dalcli-leader, daemon) |
| `/health` | GET | Health check (`{"status":"ok","clients":N}`) |

### Message Flow

```
User types in Mattermost channel
  → MM outgoing webhook → dalbridge POST /webhook
  → Normalize to streamMessage {text, username, channel, gateway, post_id, timestamp}
  → SSE broadcast to all connected clients
  → dal containers (GET /stream) receive and process mentions
  → dalroot-listener (GET /stream) writes to inbox files
```

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DALBRIDGE_LISTEN` | `:4280` | Listen address |
| `DALBRIDGE_WEBHOOK_TOKEN` | (optional) | Outgoing webhook verification token |

### ACK Flow

1. Dal encounters issue → `POST /api/claim` (type: env/blocked/bug/improvement)
2. Leader reviews → `GET /api/claims`
3. Leader responds → `POST /api/claims/{id}/respond`
4. Dal receives ACK via Mattermost mention in stream

## dalroot — Infrastructure Management

dalroot is a Claude Code instance running directly on the Proxmox host (not in a Docker container). It manages the infrastructure layer: LXC containers, networking, host coordination, and service management.

### Notification Pipeline

```
Mattermost (#dalcenter, #host, ...)
    │
    ▼ outgoing webhook
dalbridge (:4280/webhook)
    │
    ▼ SSE stream (:4280/stream)
dalroot-listener (host daemon)
    │
    ▼ file write
/var/lib/dalroot/inbox/{dalroot-id}/*.msg
    │
    ▼ Claude Code hook (UserPromptSubmit)
dalroot-check-notifications → stdout → dalroot processes
```

### Scripts (`proxmox-host-setup/dalroot/`)

| Script | Description |
|--------|-------------|
| `dalroot-id` | Generates unique ID from tmux pane: `dalroot-{session}-{window}-{pane}` |
| `dalroot-check-notifications` | Reads unread messages from inbox, outputs to stdout, deletes processed files |
| `dalroot-register` | SessionStart hook — creates inbox directory, registers identity with MM |
| `dalroot-listener` | Daemon that subscribes to dalbridge SSE stream and writes messages to inbox |
| `dalroot-task` | dalcenter task wrapper with team routing and callback support |
| `install.sh` | Installs host scripts, dalbridge service, and MM webhooks |
| `setup-webhooks.sh` | Creates MM outgoing webhooks for each team channel |

### Claude Code Hooks (dalroot)

dalroot uses Claude Code hooks for real-time notification delivery:

```json
{
  "hooks": {
    "SessionStart": [{"type": "command", "command": "dalroot-register"}],
    "UserPromptSubmit": [{"type": "command", "command": "dalroot-check-notifications"}],
    "Notification": [{"type": "command", "command": "dalroot-check-notifications"}]
  }
}
```

### Guardrail Hooks

PreToolUse hooks detect potentially dangerous operations and emit warnings:

| Pattern | Warning |
|---------|---------|
| `pct exec`, `qm start/stop/set` | Direct infrastructure manipulation — delegate to dal via issue |
| `go build -o /usr/local/bin/` | Binary build — dal-infra/host-ops handles |
| `systemctl restart/stop/start` | systemctl manipulation — host-ops delegation |
| `gh pr merge` | PR merge — reviewer review → leader merge |
| `curl.*mattermost`, `POST.*/api/v4/` | Direct MM API call — use dalbridge/talk |

### Scheduled Pipeline Surveillance

dalcenter runs periodic checks (30min interval + daily 09:00 KST summary):

| Check | Condition | Action |
|-------|-----------|--------|
| Orphan issues | Open issue with no linked PR (>1h) | Post to dal-control, notify team |
| Stale PRs | Open PR with no review (>12h) | Post to dal-control, notify reviewer |
| Approved PRs | LGTM review, not yet merged | Post to dal-control, notify leader to merge |
| Daily summary | 09:00 KST | Issue/PR counts to dal-control channel |

### Automation Pipeline

```
Human (creates GitHub issue)
  → scheduled dalroot (watches issues, sends reminders)
  → dal team (implements via branch + PR)
  → dal-control channel (reports progress)
  → Human (approves/directs via emoji or comment)
```

## Installation

### Prerequisites

- **Go** 1.25.0+
- **Docker**
- **Git**
- **Mattermost** server with a bot token

### Build

```bash
go build -o /usr/local/bin/dalcenter ./cmd/dalcenter/
go build -o /usr/local/bin/dalcli ./cmd/dalcli/
go build -o /usr/local/bin/dalcli-leader ./cmd/dalcli-leader/
go build -o /usr/local/bin/dalbridge ./cmd/dalbridge/
```

### Docker Images

Build at least the base image before waking dals:

```bash
cd dockerfiles && docker build -t dalcenter/claude:latest -f claude.Dockerfile .
```

Other images: `claude-go.Dockerfile`, `claude-rust.Dockerfile`, `codex.Dockerfile`, `gemini.Dockerfile`.

## Quick Start

```bash
# 1. Start the daemon
dalcenter serve --addr :11190 --repo /path/to/your-project \
  --mm-url http://mattermost:8065 --mm-token TOKEN --mm-team myteam

# 2. Initialize localdal in your project
dalcenter init --repo /path/to/your-project

# 3. Create dal templates (via git)
# .dal/leader/dal.cue + instructions.md
# .dal/dev/dal.cue + instructions.md
# .dal/skills/code-review/SKILL.md

# 4. Validate
dalcenter validate

# 5. Wake dals
dalcenter wake leader
dalcenter wake dev
dalcenter ps

# 6. Sleep when done
dalcenter sleep --all
```

## CLI Reference

### dalcenter (operator)

```
dalcenter serve                   # daemon (HTTP API + watchers + Docker)
dalcenter init --repo <path>      # initialize localdal (.dal/ + subtree)
dalcenter wake <dal> [--all]      # create Docker container
dalcenter sleep <dal> [--all]     # stop Docker container
dalcenter sync                    # propagate .dal/ changes to running containers
dalcenter validate [path]         # CUE schema + reference validation
dalcenter status [dal]            # show dal status
dalcenter ps                      # list awake dals
dalcenter logs <dal>              # container logs
dalcenter attach <dal>            # enter container
dalcenter tell <repo> "<msg>"     # send message to another team's leader
```

### dalcli-leader (inside leader container)

```
dalcli-leader wake <dal>          # wake team member
dalcli-leader sleep <dal>         # sleep team member
dalcli-leader ps                  # list team members
dalcli-leader status <dal>        # member status
dalcli-leader logs <dal>          # member logs
dalcli-leader sync                # sync .dal/ changes
dalcli-leader assign <dal> <task> # assign task via @mention
```

### dalcli (inside member containers)

```
dalcli status                     # own status
dalcli ps                         # team member list
dalcli report <message>           # report to leader
```

## How It Works

### Wake Flow

```
dalcenter wake dev
  1. Read .dal/dev/dal.cue → player, skills, git config
  2. Create Docker container (dalcenter/claude:latest)
  3. Convert instructions.md → CLAUDE.md (bind mount)
  4. Mount skills/ → ~/.claude/skills/ (bind mount)
  5. Mount .credentials.json (read-only)
  6. Mount service repo → /workspace (bind mount)
  7. Inject GitHub token (from dal.cue git.github_token)
  8. Inject dalcli/dalcli-leader binary (docker cp)
  9. Create Mattermost bot account + join channel
  10. Set env: DAL_NAME, DAL_UUID, DAL_ROLE, DALCENTER_URL, GH_TOKEN
```

### Sync Flow

```
.dal/ change → git push (GitHub)
  → repo-watcher detects remote changes (within 2min)
  → git pull --ff-only
  → diff .dal/ → runSync()
  → bind mount files (instructions.md, skills/) → instant reflection
  → dal.cue structure change (player, skills) → container auto-restart
```

### Issue Workflow

```
POST /api/issue-workflow {issue_id, member, task}
  → pending → waking (wake member with issue branch)
  → assigned (send task via @mention)
  → working (track execution)
  → done/failed (notify dalroot via MM)
```

## localdal Structure

```
.dal/
  dal.spec.cue              schema definition
  leader/
    dal.cue                 uuid, player, role:leader
    charter.md              role charter
    instructions.md         → CLAUDE.md at wake
  dev/
    dal.cue                 uuid, player, role:member
    charter.md
    instructions.md
  skills/                   shared skill pool
    code-review/SKILL.md
    testing/SKILL.md
    git-workflow/SKILL.md
    ...
  template/                 cross-team reusable templates
    auditor/
    config-manager/
    dalops/
    test-writer/
    dal-infra/
    skills/                 shared template skills
  dalroot/
    dal.cue                 infrastructure management role
    charter.md
```

## dal.cue

```cue
uuid:    "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
name:    "dev"
version: "1.0.0"
player:  "claude"
role:    "member"
channel_only: true
skills:  ["skills/code-review", "skills/testing"]
hooks:   []
git: {
    user:         "dal-dev"
    email:        "dal-dev@myproject.dev"
    github_token: "env:GITHUB_TOKEN"
}
```

## File Name Conversion

| Source | Player | In Container |
|---|---|---|
| instructions.md | claude | CLAUDE.md |
| instructions.md | codex | AGENTS.md |
| instructions.md | gemini | GEMINI.md |

## Communication

Dals communicate via Mattermost. One channel per project (auto-created on serve).

- `dalcli-leader assign dev "task"` → posts `@dal-dev task`
- `dalcli report "done"` → posts `[dev] report: done`
- **dal-control** channel — infrastructure dashboard for scheduled dalroot reports and human approval

## Credentials

dalcenter auto-mounts player credentials into containers (read-only bind mount). Expired tokens trigger a warning at wake time.

| Player | Host Path | Container Path | Expiry Check |
|--------|-----------|---------------|-------------|
| claude | `~/.claude/.credentials.json` | `~/.claude/.credentials.json` | `expiresAt` (ms) |
| codex | `~/.codex/auth.json` | `~/.codex/auth.json` | `tokens.expires_at` (RFC3339) |
| gemini | env `GEMINI_API_KEY` | env `GEMINI_API_KEY` | — |

### Token Refresh

- **Claude**: OAuth token. If expired, run `claude auth login` on the host, then `pve-sync-creds`.
- **Codex**: ChatGPT OAuth. If expired, run `codex auth login` on the host, then `pve-sync-creds`.
- **Gemini**: API key (no expiry). Set `GEMINI_API_KEY` env var.
- Running dals auto-file credential sync claims on auth failure.
- If `DALCENTER_CRED_OPS_ENABLED` is on (default), dalcenter auto-runs credential sync and reports to `dal-ops` channel.
- For LXC environments, configure `DALCENTER_CRED_OPS_HTTP_URL` and `DALCENTER_CRED_OPS_HTTP_TOKEN` for host bridge. Reference bridge: [`scripts/dalcenter-cred-ops-httpd.py`](./scripts/dalcenter-cred-ops-httpd.py).

## Environment Variables

| Variable | Description |
|----------|-------------|
| `DALCENTER_URL` | dalcenter API address (e.g. `http://localhost:11190`) |
| `DALCENTER_ADDR` | Listen address for HTTP API |
| `DALCENTER_REPO` | Service repository path |
| `DALCENTER_LOCALDAL_PATH` | localdal directory path |
| `DALCENTER_TOKEN` | Bearer token for write endpoints |
| `DALCENTER_MM_URL` | Mattermost server URL |
| `DALCENTER_MM_TOKEN` | Mattermost bot token |
| `DALCENTER_MM_TEAM` | Mattermost team name |
| `DALCENTER_GITHUB_REPO` | GitHub repo for issue polling (`owner/repo`) |
| `DALCENTER_NOTIFY_URL` | Task completion callback URL |
| `DALCENTER_CRED_OPS_ENABLED` | Enable credential sync ops (`1`) |
| `DALCENTER_CRED_OPS_HTTP_URL` | Host credential bridge URL |
| `DALCENTER_CRED_OPS_HTTP_TOKEN` | Host bridge Bearer token |
| `DALCENTER_SCHEDULED_DALROOT` | Enable scheduled pipeline surveillance (`1`) |
| `DALCENTER_DALBRIDGE_URL` | dalbridge SSE stream URL |
| `DALCENTER_BRIDGE_URL` | Matterbridge API URL |
| `DALCENTER_BRIDGE_GATEWAY` | Matterbridge gateway name |
| `DALCENTER_URLS` | Multi-project URL routing (comma-separated `name=url`) |
| `GITHUB_TOKEN` | GitHub authentication |

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md).
