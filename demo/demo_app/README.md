# 🚀 demo_app — bla bla

description

---

## Features

- Exposes a secured **HTTPS REST API** (API key authentication)
- **IP allowlist / blocklist** support
- **Hot-reload** of configuration via `SIGHUP`
- Embedded self-signed TLS certificate for development (no setup required)
- Optional **Swagger UI** (build tag `swagger`, dev only)

---

## Where to start

- Runtime, API, build, deploy, and Swagger usage: [`cmd/README.md`](cmd/README.md)
- Example configuration: [`config/config.yaml`](config/config.yaml)
- Swagger generation script: [`docs/generate.sh`](docs/generate.sh)

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

### Environment variables

Values may reference environment variables as `${NAME}`:

```yaml
webserver:
  apiKey: ${DEMO_APP_API_KEY}
```

Only that braced form is expanded. A bare `$NAME` is left alone, so a value like
`pa$$w0rd` survives unchanged — `os.ExpandEnv` over the whole file used to turn
it into `pa`. A referenced variable that is not set is an error naming it, not
an empty string, because an empty secret is the kind of mistake that only shows
up in production.

### What is validated at startup

Beyond the environment, the API key, the log level and the port range:

| Check | Applies |
|---|---|
| `certFile` and `keyFile` are configured | `env: prod` only |
| `apiKey` is not the shipped placeholder `changeme!` | `env: prod` only |
| `jwtSecret` and `jwtID` are set together or not at all | always |
| `logDestination` is a known name, or a path whose directory exists | always |
| `listenHost` is empty, an IP address or `localhost` | always |
| every entry of `allowedIPs`/`blockedIPs` parses as IP or CIDR | always |

The IP lists are the reason for the last row: a typo there would otherwise
become a rule that silently never matches.


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

**The embedded development certificate is only used in `env: dev`.** The binary
carries a self-signed pair (`app/certs/`) so that a fresh checkout starts over
HTTPS without preparation. That pair is public — it lives in this repository,
its private key included — and it is issued for `localhost` and nothing else.
Treat it as scaffolding, never as protection.

With `env: prod` a missing `certFile` or `keyFile` is a startup error, not a
fallback: the service refuses to come up rather than serve a certificate whose
private key everybody has. `Validate()` additionally insists that both paths are
configured in `prod`, so the mistake is caught before the listener opens.

`make ensure_dev_certs` generates a fresh pair into `app/certs/` if you would
rather not ship the checked-in one.

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
