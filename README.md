# Status Probe lite — Monitoring Tool

Status Probe is a small, production-style monitoring system that tracks the availability of web services.  It automatically detects outages, classifies failure reasons, and calculates uptime percentage.

---

## Overview

The system is composed of two independent components:

### Central Server
- Stores all monitoring data in SQLite.
- Exposes REST APIs to:
  - Register URLs to monitor
  - Collect health check data
  - Compute availability and outages
  - Register monitoring agents

### Agent
- A small **Go binary** that can run anywhere — even in private networks.
- Periodically checks target URLs and reports results to the central server.
- Uses an API key for authentication.
- Detects multiple failure types:
  - `timeout`
  - `tls_error`
  - `dns_error`
  - `non_2xx`
---

## Tech Stack

| Layer | Technology |
|-------|-------------|
| Language | Go 1.22+ |
| Framework | Gin |
| Database | SQLite (`modernc.org/sqlite`) |
| HTTP Client | Go `net/http` |

---

## 1. Prerequisites

- **Go** ≥ 1.22  
- **curl** (for testing API calls)  
- **git**  
- **SQLite** (no manual setup needed; Go driver is embedded)

---

## 2. Clone and Setup

```bash
git clone https://github.com/niranjini-kathiravan/status-probe-lite.git
cd status-probe-lite/backend

# Setup & Demo Guide — Status Probe Lite
## 1. Create a `.gitignore`

Exclude DB and local config files so they aren't committed:

```bash
cat > .gitignore <<'EOF'
*.db
.env
targets.json
.vscode/
EOF
```

## 2. Run the Central Server

```bash
PORT=8080 DB_PATH=./status.db VERSION=v0.2 go run ./cmd/server
```

Expected output:

```
Loaded config → PORT=8080 DB_PATH=./status.db POLL_INTERVAL_SEC=15 GLOBAL_TIMEOUT_MS=5000 VERSION=v0.2
Server running on :8080
```

Health check:

```bash
curl http://localhost:8080/healthz
# ok
```

## 3. Register an Agent

```bash
curl -s -X POST http://localhost:8080/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"name": "agent-local", "location": "public", "version": "v0.1"}'
```

Response example:

```json
{"agent_id":1,"api_key":"d917741381c859387e07948f39c1e8f4901b21d2bf842386ce7f272264bd229c"}
```

Copy the `api_key` — the agent will need it.

## 4. Register Targets (URLs to Monitor)

```bash
curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"smelinx","url":"https://www.smelinx.com","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"httpbin-503","url":"https://httpbin.org/status/503","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"httpbin-delay","url":"https://httpbin.org/delay/10","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"badssl","url":"https://expired.badssl.com/","timeout_ms":4000}'
```

Verify the targets:

```bash
curl -s http://localhost:8080/api/targets | jq
```

## 5. Prepare and Run the Agent

Once targets are registered, a targets.json file is created automatically.

```json
[
  {"id": 1, "name": "smelinx", "url": "https://www.smelinx.com", "timeout_ms": 4000},
  {"id": 2, "name": "httpbin-503", "url": "https://httpbin.org/status/503", "timeout_ms": 4000},
  {"id": 3, "name": "httpbin-delay", "url": "https://httpbin.org/delay/10", "timeout_ms": 4000},
  {"id": 4, "name": "badssl", "url": "https://expired.badssl.com/", "timeout_ms": 4000}
]
```

Run the agent:

```bash
CENTRAL_BASE_URL=http://localhost:8080 \
API_KEY=<API_KEY> \
TARGETS_FILE=./targets.json \
POLL_INTERVAL_SEC=15 \
go run ./cmd/agent
```

Agent output example:

```
[check] target=smelinx ok=true status=200 latency_ms=85 reason=
[check] target=httpbin-503 ok=false status=503 latency_ms=1090 reason=non_2xx
[check] target=httpbin-delay ok=false status=0 latency_ms=4000 reason=timeout
[check] target=badssl ok=false status=0 latency_ms=500 reason=tls_error
```

## 6. View Metrics & Outages

Check metrics per target:

```bash
curl -s "http://localhost:8080/api/metrics?target_id=1" | jq
```

### Example (Healthy Target)

```json
{
  "availability_percent_checks": 100,
  "availability_percent_time": 100,
  "average_latency_ms": 77,
  "failed_checks": 0,
  "failures_by_reason": {},
  "outages": []
}
```

### Example (Failing Target)

```json
{
  "availability_percent_checks": 0,
  "availability_percent_time": 87.4,
  "failures_by_reason": { "tls_error": 10 },
  "outages": [
    {
      "started_at": "2025-11-09T14:49:11Z",
      "duration_ms": 452637,
      "reason": "tls_error"
    }
  ]
}
```

## Outage Rules

| State | Trigger | Action |
|-------|---------|--------|
| Open Outage | 2 consecutive failed checks | Creates a new outage |
| Close Outage | 2 consecutive successful checks | Closes the outage |
| Reason | Frozen at open | Remains constant until close |

## How It Works

- **Central server** hosts APIs & database.
- **Agents** run anywhere (public/private), poll URLs, and push results.
- **Server** detects outages & computes uptime/availability metrics.
- System scales horizontally — any number of agents can push to one central server.

## Example Demo Flow

| Step | Command | Purpose |
|------|---------|---------|
| 1 | `go run ./cmd/server` | Start central server |
| 2 | Register agent | Get API key |
| 3 | Add targets | Define URLs to monitor |
| 4 | Run agent | Start polling |
| 5 | `curl /api/metrics` | Inspect outages & uptime |

Folder Structure:

backend/
├── cmd/
│   ├── server/       # Central server entrypoint
│   └── agent/        # Agent entrypoint
├── internal/
│   ├── api/          # HTTP handlers and routes for agents, targets, ingest and metrics
│   ├── store/        # SQLite persistence layer
│   ├── config/       # env config file
├── targets.json       # Generated automatically
├── status.db          # SQLite database
└── go.mod


Further Steps:

1. Add a simple dashboard to see the logs 
2. Demo toggler to show the open and close of an outage 
3. Demo Agent in a private network 
4. Integrate containerisation (Docker)