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

## Quick Start (Docker)

This is the recommended approach for development and testing.

1. Create a `.env` file from the example template to contain your secrets:
   ```bash
   cp .env.example .env
   ```
2. Edit `.env` to include your actual tokens:
   ```env
   QRADAR_API_TOKEN=your_secure_token_here
   DESTINATION_API_KEY=your_secure_destination_key
   ```
3. Start the collector:
   ```bash
   make docker-up
   # or
   docker compose up --build -d
   ```
3. View the logs:
   ```bash
   docker compose logs -f
   ```

---

## Production Deployment (Native/Systemd)

For maximum performance on an Ubuntu/Debian production asset:

1. Compile the binary for Linux:
   ```bash
   make build-linux
   # Output: qradar-collector
   ```

2. Prepare the production directory:
   ```bash
   sudo mkdir -p /opt/qradar-collector
   sudo useradd -rs /bin/false qradar
   sudo chown qradar:qradar /opt/qradar-collector
   ```

3. Deploy the binary and configuration:
   ```bash
   sudo cp qradar-collector /opt/qradar-collector/
   sudo cp config.yaml /opt/qradar-collector/
   
   # Setup environment overrides securely
   echo "QRADAR_API_TOKEN=your_token" | sudo tee /opt/qradar-collector/.env
   echo "DESTINATION_API_KEY=your_key" | sudo tee -a /opt/qradar-collector/.env
   ```

4. Install and enable the systemd service:
   ```bash
   sudo cp deploy/qradar-collector.service /etc/systemd/system/
   sudo systemctl daemon-reload
   sudo systemctl enable qradar-collector
   sudo systemctl start qradar-collector
   ```

5. Monitor the service logs:
   ```bash
   sudo journalctl -fu qradar-collector
   ```
