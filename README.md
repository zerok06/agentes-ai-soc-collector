# QRadar Offense Collector & Forwarder

A production-grade microservice written in Go that acts as a bridge between IBM QRadar SIEM and an external ingestion API.

## Architecture

1. **Polls QRadar SIEM**: Periodically queries the `GET /siem/offenses` endpoint for new or updated offenses since the last successful poll.
2. **Event Enrichment**: For each active offense, generates an asynchronous AQL search via `POST /ariel/searches` to retrieve associated event details, polling until complete.
3. **Data Transformation**: Maps the QRadar API responses into a predefined custom JSON schema.
4. **Forwarding**: Sends the structured JSON payload to the external destination API via `POST` requests.
5. **State Management**: Persistently tracks the latest processed timestamp in `state.json` to guarantee precise resume capability after restarts.

## Features
- Highly concurrent event processing using a worker pool pattern.
- Robust HTTP client with connection pooling and timeouts.
- Exponential backoff retry logic for resilience against QRadar or Destination API transient failures.
- Zero dependencies outside of `go.uber.org/zap` (logging) and `gopkg.in/yaml.v3` (config).
- Memory efficient (`< 50MB` RAM) and lightweight container image (`~15MB`).

---

## Configuration

Configuration is provided via a `config.yaml` file natively, but all values can be securely overridden environment variables:

| Environment Variable | Description |
|----------------------|-------------|
| `QRADAR_BASE_URL` | Base URL of the QRadar API (e.g., `https://qradar.company.local/api`) |
| `QRADAR_API_TOKEN` | Authorized Service Token (SEC) |
| `QRADAR_VERSION` | Target API version (default: `20.0`) |
| `QRADAR_TLS_INSECURE` | Set to `true` to skip certificate validation (default: `false`) |
| `DESTINATION_URL` | External ingestion API endpoint |
| `DESTINATION_API_KEY`| API key sent in the `x-api-key` header |
| `POLL_INTERVAL_SECONDS`| How often to check for offenses (default: `60`) |
| `STATE_FILE` | Local file to persist timestamp (default: `./state.json`) |
| `LOG_LEVEL` | Logging level: `debug`, `info`, `warn`, `error` |

---

## Quick Start & Production Deployment (Docker Compose)

The recommended and fully supported deployment method for this collector is via **Docker Compose**. This ensures perfect environment isolation, automatic restarts, and zero host dependencies.

### Prerequisites
- Docker engine installed on the target server.
- Docker Compose v2 installed.

### Step-by-step Guide

1. **Clone or transfer the project** to your Ubuntu server:
   ```bash
   git clone <tu-repo> /opt/qradar-collector
   cd /opt/qradar-collector
   ```

2. **Configure your Secrets (`.env`)**
   Copy the example environment file and fill in your actual production tokens.
   ```bash
   cp .env.example .env
   nano .env
   ```
   *Note: Never commit `.env` to source control.*

3. **Start the Collector in the Background**
   Build the lightweight Alpine image and start the container in detached mode:
   ```bash
   docker compose up -d --build
   ```
   *The `--build` flag ensures the Go binary is freshly compiled.*

4. **Verify the Deployment**
   Check the logs to ensure it successfully connected to QRadar and loaded the state:
   ```bash
   docker compose logs -f
   ```
   *(Press `Ctrl+C` to exit the logs view)*

5. **Managing the Service**
   - **Stop the collector:** `docker compose down`
   - **Restart the collector:** `docker compose restart`
   - **Check status:** `docker compose ps`

### Troubleshooting & Logs
All structured JSON logs and connection errors are routed to `stdout` and captured by Docker. You can filter logs natively:
```bash
docker compose logs --tail=100 -f
```
