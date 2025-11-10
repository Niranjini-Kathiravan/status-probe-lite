## November 7 — Phase 1: Initial Concept

**Goal:**  
Create a simple service to **monitor public endpoints**, detect outages, and visualize uptime on a dashboard.

**Architecture:**  
- A single **server** handling both polling and storage.
- The server periodically sends HTTP requests to configured targets (URLs).  
- Results and latency are **stored in a local SQLite database**.
- A **frontend dashboard** displays the health (UP/DOWN), latency, and outage windows in real time.

**Outcome:**  
This worked for small public URLs but lacked flexibility all checks ran from the central server, and it wasn’t suitable for testing endpoints inside private networks.

## November 8 — Phase 2: Private Network Challenge

**New problem identified:**  
How to monitor services that are only accessible within **private networks** (e.g., internal company systems) — the central server can’t directly reach them.

**Idea:**  
Introduce a **lightweight agent** that runs inside the private network.  
This agent performs checks locally and **pushes heartbeat and result data** to the central server.

**Architecture changes:**
- Central server hosts APIs and stores logs + outages.
- Agents (running in private networks) connect **outbound** to the central server.
- Agents push their health data, enabling visibility without inbound connections.
- Authentication via **API keys** for each registered agent.

## November 9 — Phase 3: Simplification & Scalability

**Reflection:**  
The initial setup was good but too complex for the challenge.  
The new goal became keeping it **simple yet realistic and scalable**.

**Refined architecture:**
- A central server manages:
  - Registered agents  
  - Target definitions  
  - Metrics and outages in the database  
- Each agent (public or private) polls its assigned targets.
- Agents push check results and logs to central server.
- Central server processes incoming data, stabilizes outages, and updates the dashboard.
- A static web UI allows adding/deleting targets and registering agents visually.

**Further steps to Implement:**

1. Create a simple frontend dashboard using HTML/CSS and Vanilla JS.
2. Demo file for testing the outage open and close.
3. Containerisation using Docker.

