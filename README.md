# Status Probe Lite - Monitoring Tool

A lightweight monitoring tool built in Go for tracking web service availability, latency, and outage states. It automatically detects failures, classifies outage reasons, and provides a simple browser based dashboard for visualization.

## Architecture

Status Probe Lite has two main components:

### Central Server

The central server stores monitoring data in SQLite and provides REST APIs for managing monitored targets, ingesting agent health checks, and retrieving metrics and outages. It detects outages based on consecutive failed checks, calculates uptime and latency metrics, and serves a static web dashboard.

### Agent

The agent is a lightweight Go binary that runs in any environment, whether public or private. It periodically probes URLs and pushes results to the central server using API key authentication. The agent detects various failure types including timeouts, DNS errors, TLS errors, and non-2xx responses, then pushes logs and metrics to the central server.

## Tech Stack

- **Language**: Go 1.22+
- **Web Framework**: Gin
- **Database**: SQLite (modernc.org/sqlite)
- **Client**: Go net/http
- **Dashboard**: Vanilla JS + HTML (served as static files)
- **Containerization**: Docker + Docker Compose

## Prerequisites

- Go 1.22 or higher 
- Docker and Docker Compose
- curl (for API testing)
- git

SQLite is embedded, so no external database setup is needed.

## Quick Start with Docker

### 1. Clone the Repository

```bash
git clone https://github.com/niranjini-kathiravan/status-probe-lite.git
cd status-probe-lite/backend
```

### 2. Create Environment File

Create a `.env` file with the following content:

```bash
# Central Server
PORT=8080
DB_PATH=/data/status.db
VERSION=v0.2

# Agent
API_KEY=<replace_with_api_key_after_register>
CENTRAL_BASE_URL=http://server:8080
POLL_INTERVAL_SEC=15
TARGETS_FILE=/app/targets.json
```

Note: You'll fill in the actual API_KEY after registering an agent in step 4.

### 3. Build Docker Images

```bash
docker compose build
```

You should see confirmation that both backend-server and backend-agent have been built.

### 4. Start the Server

```bash
docker compose up -d server
docker compose logs -f server
```

Check the server health:

```bash
curl http://localhost:8080/healthz
```

Expected response: `ok`

### 5. Register an Agent

```bash
curl -s -X POST http://localhost:8080/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"agent-docker"}'
```

Example output:

```json
{
  "agent_id": 1,
  "api_key": "db5f0ae83a29f1f6f7ea8be771fa9fcd..."
}
```

Copy the api_key value into your `.env` file as the API_KEY value and save.

### 6. Register Targets

Register your monitoring targets:

```bash
curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"smelinx","url":"https://www.smelinx.com","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"httpbin-503","url":"https://httpbin.org/status/503","timeout_ms":4000}'
```

List registered targets:

```bash
curl -s http://localhost:8080/api/targets | jq
```

### 7. Export Targets for Agent

```bash
curl -s http://localhost:8080/api/targets > ./cmd/agent/targets.json
```

This file will be bind-mounted into the agent container.

### 8. Start the Agent

```bash
docker compose up -d agent
docker compose logs -f agent
```

You should see messages like:

```
[agent] posting 2 checks...
[agent] posting 2 checks...
```

### 9. View Dashboard

Open your browser and navigate to:

```
http://localhost:8080/dashboard/
```

The dashboard provides:

- Target overview with health states
- Agent registration and keys
- Latency and failure reason charts
- Outage history and uptime metrics

### 10. Simulate Outage and Recovery

To simulate an outage:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/503"
```

To recover:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/200"
```

The dashboard will automatically show outages opening and closing after two failed or successful checks respectively.

## API Overview

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | /healthz | Health check for the server |
| POST | /api/targets | Register a new target |
| GET | /api/targets | List all targets |
| POST | /api/agents/register | Register a new agent |
| POST | /api/ingest/checks | Agent pushes health check results |
| GET | /api/metrics | Retrieve metrics for a target |
| GET | /api/logs | Fetch historical logs |
| GET | /api/logs/stream | Live log streaming (SSE) |
| GET | /dashboard/ | Web dashboard |
| GET | /demo/set | Toggle outage simulation |

## Outage Rules

| Event | Trigger | Action |
|-------|---------|--------|
| Open outage | 2 consecutive failed checks | Creates new outage |
| Close outage | 2 consecutive successful checks | Closes outage |
| Reason | Determined at open time | Remains until closed |

## Folder Structure

```
backend/
├── cmd/
│   ├── server/       # Central server entrypoint
│   └── agent/        # Agent binary
├── internal/
│   ├── api/          # HTTP handlers (targets, agents, ingest, logs, metrics)
│   ├── store/        # SQLite data layer
│   ├── config/       # Env-based configuration
│   └── web/static/   # Dashboard (HTML/JS)
├── docker-compose.yml
├── Dockerfile.server
├── Dockerfile.agent
├── targets.json       # Generated file
├── status.db          # SQLite database
└── .env               # Environment variables
```

## Useful Docker Commands

View logs:

```bash
docker compose logs -f server
docker compose logs -f agent
```

Restart agent only:

```bash
docker compose restart agent
```

Stop everything:

```bash
docker compose down
```

Verify running containers:

```bash
docker ps
```

## Future Improvements

- **Alerts**: Trigger notifications on outage events
- **User Authentication**: Multi-user dashboards and API key management
- **Agent Auto-Discovery**: Dynamically register new agents in distributed setups
- **Kubernetes Deployment**: for scalable environment

