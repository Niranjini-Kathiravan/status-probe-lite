# Docker Setup Guide

This guide provides step-by-step instructions for running the Status Probe Lite central server and agent using Docker Compose. This is the recommended approach for deployment as it provides a consistent, isolated environment.

## Prerequisites

Before starting, ensure to have the following installed:

- Docker and Docker Compose
- curl (for API testing)
- git

Clone the repository and navigate to the backend directory:

```bash
git clone https://github.com/niranjini-kathiravan/status-probe-lite.git
cd status-probe-lite/backend
```

## Setup Instructions

### Step 1: Create Environment Configuration

The agent requires an API key that won't be available until after registration. Start by creating a `.env` file with a placeholder value:

```bash
cat > .env <<'EOF'
# Central Server Configuration
PORT=8080
DB_PATH=/data/status.db
VERSION=v0.2

# Agent Configuration (update API_KEY after step 4)
API_KEY=set-after-register
CENTRAL_BASE_URL=http://server:8080
POLL_INTERVAL_SEC=15
TARGETS_FILE=/app/targets.json
EOF
```

The `.env` file is read by docker-compose.yml and should be kept out of version control.

### Step 2: Build Docker Images

Build both the server and agent images:

```bash
docker compose build
```

You should see confirmation that both images have been built successfully:

```
✔ backend-server  Built
✔ backend-agent   Built
```

### Step 3: Start the Central Server

Start only the server initially:

```bash
docker compose up -d server
docker compose logs -f server
```

Verify the server is running with a health check:

```bash
curl http://localhost:8080/healthz
```

Expected response: `ok`

You can also access the dashboard at:

```
http://localhost:8080/dashboard/
```

### Step 4: Register an Agent and Configure API Key

Request a new agent API key from the server:

```bash
curl -s -X POST http://localhost:8080/api/agents/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"agent-docker"}'
```

Example response:

```json
{
  "agent_id": 1,
  "api_key": "db5f0ae83a29f1f6f7ea8be771fa9fcd..."
}
```

Edit your `.env` file and update the API_KEY with the value returned:

```bash
API_KEY=db5f0ae83a29f1f6f7ea8be771fa9fcd...
```

Save the file after making this change.

### Step 5: Register Monitoring Targets

Add the URLs you want to monitor:

```bash
curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"smelinx","url":"https://www.smelinx.com","timeout_ms":4000}'

curl -s -X POST http://localhost:8080/api/targets \
  -H 'Content-Type: application/json' \
  -d '{"name":"httpbin-503","url":"https://httpbin.org/status/503","timeout_ms":4000}'
```

Confirm the targets were registered:

```bash
curl -s http://localhost:8080/api/targets | jq
```

### Step 6: Export Targets for the Agent

The agent reads monitoring targets from a JSON file. Export the current targets:

```bash
curl -s http://localhost:8080/api/targets > ./cmd/agent/targets.json
```

This file is bind-mounted into the agent container as specified in docker-compose.yml.

### Step 7: Start the Agent

Start the agent service:

```bash
docker compose up -d agent
docker compose logs -f agent
```

You should see periodic log messages indicating the agent is posting health checks:

```
[agent] posting 2 checks...
[agent] posting 2 checks...
```

### Step 8: Verify Metrics and Dashboard

Check metrics via the API:

```bash
TARGET_ID=1
curl -s "http://localhost:8080/api/metrics?target_id" | jq
```

View the dashboard in your browser:

```
http://localhost:8080/dashboard/
```

The dashboard displays targets, availability percentages, failure counts, and outage states (HEALTHY or ISSUE).

### Step 9: Simulate Outages and Recovery (Optional) 

The server includes a demo endpoint for testing outage detection. This affects a special demo target configured in the server.

To simulate an outage:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/503"
```

To simulate recovery:

```bash
curl -s "http://localhost:8080/demo/set?to=https://httpbin.org/status/200"
```

Note: Outages open after 2 consecutive failed checks and close after 2 consecutive successful checks. Watch the dashboard to see real-time updates.

## Useful Docker Commands

Follow logs for a specific service:

```bash
docker compose logs -f server
docker compose logs -f agent
```

Restart a single service:

```bash
docker compose restart agent
```

Stop all services:

```bash
docker compose down
```

View running containers:

```bash
docker ps
```

## Troubleshooting

### Dashboard Returns 404

Ensure the server has mounted the static assets correctly. Visit the dashboard URL with the trailing slash:

```
http://localhost:8080/dashboard/
```

Check server logs for errors:

```bash
docker compose logs -f server
```

### Agent Returns 401 (Unauthorized) on Ingest

The API_KEY in your `.env` file must match the key returned by the agent registration endpoint. Verify the key is correct and restart the agent:

```bash
docker compose up -d agent
docker compose logs -f agent
```

### No Data Available for Metrics

Ensure targets have been registered:

```bash
curl -s http://localhost:8080/api/targets | jq
```

Verify the agent is running and posting checks:

```bash
docker compose logs -f agent
```

If you added or removed targets, re-export the targets file:

```bash
curl -s http://localhost:8080/api/targets > ./cmd/agent/targets.json
docker compose restart agent
```

### Docker Compose Version Warning

If Docker Compose complains about the version field, remove it from docker-compose.yml as it is now obsolete in newer versions of Docker Compose.

## Reference: Docker Configuration Files

### docker-compose.yml

```yaml
services:
  server:
    build:
      context: .
      dockerfile: Dockerfile.server
    env_file: .env
    environment:
      - PORT=${PORT}
      - DB_PATH=${DB_PATH}
      - VERSION=${VERSION}
    ports:
      - "${PORT}:8080"
    volumes:
      - server-data:/data
      - ./internal/web/static:/app/internal/web/static:ro
    command: ["/app/server"]

  agent:
    build:
      context: .
      dockerfile: Dockerfile.agent
    env_file: .env
    environment:
      - CENTRAL_BASE_URL=${CENTRAL_BASE_URL}
      - API_KEY=${API_KEY}
      - POLL_INTERVAL_SEC=${POLL_INTERVAL_SEC}
      - TARGETS_FILE=${TARGETS_FILE}
    depends_on:
      - server
    volumes:
      - ./cmd/agent/targets.json:/app/targets.json:ro
    command: ["/app/agent"]

volumes:
  server-data:
```

### Dockerfile.server

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=build /server /app/server
COPY internal/web/static /app/internal/web/static
EXPOSE 8080
CMD ["/app/server"]
```

### Dockerfile.agent

```dockerfile
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /agent ./cmd/agent

FROM alpine:3.20
WORKDIR /app
COPY --from=build /agent /app/agent
# /app/targets.json will be mounted by docker-compose
CMD ["/app/agent"]
```

Note: The Dockerfiles reference Go 1.25 to match the module's go directive. Ensure consistency between your local Go version and the Docker build images.

## Next Steps

After completing this setup, you can:

- Add more monitoring targets via the API
- Configure additional agents in different environments
- Customize polling intervals in the `.env` file
- Integrate the metrics API with external monitoring tools

For more information about the API endpoints and architecture, refer to the main README.md file.