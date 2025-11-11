# Status Probe Lite

A lightweight monitoring tool built in Go for tracking web service availability, latency, and outage states. The system automatically detects failures, classifies outage reasons, and provides a simple browser based dashboard for visualization.

## Architecture

Status Probe Lite consists of two main components:

### Central Server

The central server acts as the data collection and analysis hub:

- Stores monitoring data in an embedded SQLite database
- Provides REST APIs for target management, health check ingestion, and metrics retrieval
- Detects outages based on consecutive check failures
- Calculates uptime percentages and availability metrics
- Serves a web dashboard for visualization

### Agent

A lightweight Go binary that performs the actual health checks:

- Runs anywhere with network access to both targets and the central server
- Periodically probes configured URLs and reports results
- Authenticates with the central server using API keys
- Detects various failure types including timeouts, DNS errors, TLS issues, and non-2xx responses
- Generates detailed logs for each check

## Technology Stack

- **Language**: Go 1.22+
- **Web Framework**: Gin
- **Database**: SQLite (modernc.org/sqlite)
- **HTTP Client**: Go standard library net/http
- **Frontend**: Vanilla JavaScript served via static routes

## Prerequisites

- Go version 1.22 or higher
- curl (for API testing)
- git

Note: SQLite is embedded and requires no separate installation.

## Installation and Setup

### Clone the Repository

```bash
git clone https://github.com/niranjini-kathiravan/status-probe-lite.git
cd status-probe-lite/backend
```

### Configure Git Ignore

```bash
cat > .gitignore <<'EOF'
*.db
.env
targets.json
.vscode/
EOF
```

### Start the Central Server

```bash
PORT=8080 DB_PATH=./status.db VERSION=v0.2 go run ./cmd/server
```

You should see output confirming the server has started:

```
Loaded config → PORT=8080 DB_PATH=./status.db VERSION=v0.2
Central on :8080 (db=./status.db)
```

Verify the server is running:

```bash
curl http://localhost:8080/healthz
# Response: ok
```

### Register an Agent

Create an agent that will perform health checks:

```bash
curl -s -X POST http://localhost:8080/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"name": "agent-local"}' | jq
```

The response will include an API key:

```json
{
  "agent_id": 1,
  "api_key": "d917741381c859387e07948f39c1e8f4901b21d2bf842386ce7f272264bd229c"
}
```

Save this API key as it will be needed to run the agent.

### Add Monitoring Targets

Register the URLs to monitor:

```bash
curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"smelinx","url":"https://www.smelinx.com","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"httpbin-503","url":"https://httpbin.org/status/503","timeout_ms":4000}'
```

Verify targets were registered:

```bash
curl -s http://localhost:8080/api/targets | jq
```

### Export Targets Configuration

Generate a targets file for the agent:

```bash
curl -s http://localhost:8080/api/targets > ./cmd/agent/targets.json
```

The file will contain the registered targets:

```json
[
  {"id": 1, "name": "smelinx", "url": "https://www.smelinx.com", "timeout_ms": 4000},
  {"id": 2, "name": "httpbin-503", "url": "https://httpbin.org/status/503", "timeout_ms": 4000}
]
```

### Run the Agent

Start the agent with the API key:

```bash
CENTRAL_BASE_URL=http://localhost:8080 \
API_KEY=<YOUR_API_KEY> \
TARGETS_FILE=./cmd/agent/targets.json \
POLL_INTERVAL_SEC=15 \
go run ./cmd/agent
```

The agent will begin sending health check data to the server every 15 seconds.

## Using the API

### View Metrics

Check the availability and performance metrics for a target:

```bash
curl -s "http://localhost:8080/api/metrics?target_id=1" | jq
```

Example response for a healthy target:

```json
{
  "availability_percent_checks": 100,
  "availability_percent_time": 100,
  "average_latency_ms": 85,
  "failed_checks": 0,
  "failures_by_reason": {},
  "outages": []
}
```

Example response for a failing target:

```json
{
  "availability_percent_checks": 0,
  "failures_by_reason": { "tls_error": 2 },
  "outages": [
    {
      "started_at": "2025-11-09T14:49:11Z",
      "reason": "tls_error"
    }
  ]
}
```

### View Logs

Retrieve historical logs for a target:

```bash
curl -s "http://localhost:8080/api/logs?target_id=1&limit=10" | jq
```

Stream logs in real-time using Server-Sent Events:

```bash
curl -N "http://localhost:8080/api/logs/stream?target_id=1"
```

## Outage Detection

The system uses the following rules to manage outage states:

| Event | Trigger | Action |
|-------|---------|--------|
| Open Outage | 2 consecutive failed checks | Creates a new outage record |
| Close Outage | 2 consecutive successful checks | Closes the existing outage |
| Reason | Determined at open time | Remains unchanged until outage closes |

## Web Dashboard

Access the dashboard by visiting:

```
http://localhost:8080/dashboard
```

The dashboard provides:

- Target management (add and delete targets)
- Agent registration with API key generation
- Live outage status indicators (Healthy / Issue)
- Real-time availability metrics and failure reasons

## Testing Outage Scenarios

The system includes a demo mode for testing outage detection:

Simulate an outage:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/503"
```

Resolve the outage:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/200"
```

This allows you to verify the outage detection logic and dashboard behavior without waiting for real failures.

## Project Structure

```
backend/
├── cmd/
│   ├── server/       # Central server entrypoint
│   └── agent/        # Agent binary
├── internal/
│   ├── api/          # HTTP routes (targets, agents, ingest, logs, metrics)
│   ├── store/        # SQLite layer (tables: targets, checks, outages, agents, logs)
│   ├── config/       # Config loader (env-based)
│   └── web/static/   # Dashboard (HTML/JS)
├── targets.json      # Exported targets configuration
├── status.db         # SQLite database
└── go.mod
```

## API Endpoints

- `GET /healthz` - Health check
- `POST /api/targets` - Register a monitoring target
- `GET /api/targets` - List all targets
- `POST /api/agents/register` - Create a new agent
- `POST /api/ingest/checks` - Submit health check results (agent endpoint)
- `GET /api/metrics` - Retrieve metrics for a target
- `GET /api/logs` - Get historical logs
- `GET /api/logs/stream` - Stream logs in real-time
- `GET /dashboard` - Web dashboard interface

