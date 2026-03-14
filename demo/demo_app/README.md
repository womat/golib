# 🚀 demo_app — bla bla

description

---

## Features

- Exposes data via HTTPS REST API (with API key or JWT authentication)
- IP allowlist / blocklist support

---

## API Endpoints

| Method | Path       | Auth    | Description                        |
|--------|------------|---------|------------------------------------|
| GET    | `/version` | —       | Application name and version       |
| GET    | `/health`  | API Key | Runtime metrics (memory, uptime …) |

Authentication via the `X-API-Key` header.

### Examples

```sh
# Application version (no auth required)
curl -k https://localhost:8443/version

# Health check
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/health
```

---

## Command-line Flags

| Flag        | Default                         | Description                                                         |
|-------------|---------------------------------|---------------------------------------------------------------------|
| `--config`  | `/opt/demo_app/etc/config.yaml` | Path to the configuration file                                      |
| `--debug`   | `false`                         | Enable debug logging to stdout (overrides log settings from config) |
| `--version` | `false`                         | Print the application version and exit                              |
| `--about`   | `false`                         | Print application details and exit                                  |
| `--help`    | `false`                         | Print this help message and exit                                    |

The config file path can also be set via the environment variable `CONFIG_FILE`.

**Examples:**

```bash
demo_app --config /etc/demo_app/config.yaml
demo_app --debug
demo_app --version
CONFIG_FILE=/etc/demo_app/config.yaml demo_app
```

---
## Configuration

Default location: `/opt/demo_app/etc/config.yaml`
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.


```yaml
# =============================================================================
# demo_app configuration
# =============================================================================

# logLevel defines the minimum log level.
# Allowed values: debug | info | warn | error
logLevel: info

# logDestination defines where logs are written to.
# Supported values: stdout | stderr | /path/to/logfile
logDestination: stdout

# =============================================================================
# Webserver configuration (HTTPS)
# =============================================================================
webserver:
  # Host address the HTTPS server listens on (0.0.0.0 = all interfaces)
  listenHost: 0.0.0.0

  # Port the HTTPS server listens on (default: 8443)
  listenPort: 8443

  # Global API key for protected endpoints
  apiKey: changeme!

  # TLS private key file
  keyFile: /opt/demo_app/etc/key.pem

  # TLS certificate file
  certFile: /opt/demo_app/etc/cert.pem

  # Blocked IP addresses or networks (empty = none blocked)
  blockedIPs: [ ]
  #  - 192.168.0.1
  #  - 192.168.0.0/16

  # Allowed IP addresses or networks (empty = all allowed)
  allowedIPs: [ ]
  #  - 127.0.0.1
  #  - ::1
  #  - 192.168.0.0/16

```

---


## TLS Certificate

Generate a self-signed certificate for development:

```sh
openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout /opt/demo_app/etc/key.pem \
  -out /opt/demo_app/etc/cert.pem \
  -days 825 \
  -subj "/C=AT/ST=Vienna/L=Vienna/O=MyCompany/OU=DEV/CN=localhost"
```

**Subject fields:**

| Field           | Example             | Description                                  |
|-----------------|---------------------|----------------------------------------------|
| `/C`            | `AT`                | Country code (2 letters)                     |
| `/ST`           | `Vienna`            | State or province (optional)                 |
| `/L`            | `Vienna`            | City (optional)                              |
| `/O`            | `MyCompany`         | Organization (optional)                      |
| `/OU`           | `DEV`               | Organizational unit (optional)               |
| `/CN`           | `localhost`         | **Common Name — your domain or `localhost`** |
| `/emailAddress` | `admin@example.com` | E-mail address (optional)                    |

> **Note:** Browsers enforce a maximum certificate validity of 825 days. Use `-days 365` for production-like setups.

---

## Installation
### 1. Create system user and directories

```sh
sudo groupadd -f demo_app
sudo useradd -r -s /usr/sbin/nologin -g demo_app demo_app
sudo usermod -aG gpio demo_app

sudo mkdir -p /opt/demo_app/{bin,etc,data}
sudo chown -R demo_app:demo_app /opt/demo_app
```

### 2. Copy files

```sh
sudo cp demo_app /opt/demo_app/bin/
sudo cp config.yaml /opt/demo_app/etc/
sudo cp cert.pem key.pem /opt/demo_app/etc/
sudo chown -R demo_app:demo_app /opt/demo_app
```

### 3. Create systemd service

```sh
sudo tee /etc/systemd/system/demo_app.service > /dev/null <<'EOF'
[Unit]
Description=demo_app — S0 Pulse Energy Monitor
After=network.target

[Service]
User=demo_app
Group=demo_app
Type=simple
ExecStart=/opt/demo_app/bin/demo_app 
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable demo_app
sudo systemctl start demo_app
sudo systemctl status demo_app
```

### 4. View logs

```sh
journalctl -u demo_app -n 50 -f
```

---

## Build

```sh
# Raspberry Pi 4/5 (64-bit OS)
make build_arm64

# Raspberry Pi 2/3/4 (32-bit OS)
make build_arm7

# Raspberry Pi 1 / Zero (32-bit OS)
make build_arm6

# Build with Swagger UI (dev only)
make build_arm64_dev

# Build and deploy to Raspberry Pi via SCP
make deploy
```

---

## Hot-Reload

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload demo_app
# or
kill -HUP $(pidof demo_app)
```

---

## Firewall

```sh
# Allow the configured port (default 8443)
sudo ufw allow 8443/tcp
sudo ufw status
```

---

## Backup & Restore

```sh
# Backup
sudo tar czf /tmp/demo_app-backup.tar.gz /opt/demo_app

# Restore
sudo tar xzf /tmp/demo_app-backup.tar.gz -C /
sudo chown -R demo_app:demo_app /opt/demo_app
sudo systemctl restart demo_app
```
---

## License

MIT
