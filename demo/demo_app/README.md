# 🚀 demo_app — the service template

`demo_app` is the reference skeleton for a service built on `golib`. It is not a
product and it is not operated anywhere: its job is to be copied. Everything a
small HTTPS service in this ecosystem needs is wired up once, correctly, and
documented — YAML configuration with defaults and environment expansion, an
`App` that owns the HTTP server and its goroutines, signal-driven graceful
shutdown and config reload, build metadata injected via `-ldflags`, and an
optional Swagger UI that production builds do not contain.

Start a new service by copying this directory and stripping what it does not
need.

---

## Features

- Exposes a secured **HTTPS REST API** (bearer tokens from a token file, `demo_app token add`)
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
| GET    | `/health`  | Token   | Runtime metrics (memory, uptime …) |

Authentication with a bearer token, `Authorization: Bearer <token>`. Each caller
gets a token of its own; the service keeps only its SHA-256 hash in the token
file and logs the token's name with every request (`user=nodered`).

```sh
# Create a token; it is printed once and then exists only with the caller.
sudo /opt/demo_app/bin/demo_app token add nodered
sudo /opt/demo_app/bin/demo_app token list
sudo /opt/demo_app/bin/demo_app token remove nodered
sudo systemctl reload demo_app     # applies the change
```

### Examples

```sh
# Application version (no auth required)
curl -k https://localhost:8443/version

# Health check
curl -k -H "Authorization: Bearer <token>" https://localhost:8443/health
```

---

## Command-line Flags

See [`cmd/README.md`](cmd/README.md#command-line-flags). The config file path
can also be set via the environment variable `CONFIG_FILE`.

---

## Configuration

### Environment variables

Values may reference environment variables as `${NAME}`:

```yaml
webserver:
  certFile: ${DEMO_APP_CERT}
```

Only that braced form is expanded. A bare `$NAME` is left alone, so a value like
`pa$$w0rd` survives unchanged. A referenced variable that is not set is an error naming it, not
an empty string, because an empty secret is the kind of mistake that only shows
up in production.

### Secrets

The demo holds no secret of its own: callers authenticate with tokens whose
hashes are in the token file, and a hash is not a secret. Write access to that
file is what must be protected — whoever can write it adds a token of their
own — hence the permissions under [Installation](#installation).

A service built from this template that *sends* a secret (an MQTT password, a
cloud login) references it as `${NAME}` and gets it from an `EnvironmentFile`
(`/opt/<svc>/etc/.env`, mode 600, owned by root), never in plain text in
`config.yaml` and never in a repository.

### What is validated at startup

Beyond the environment, the log level and the port range:

| Check | Applies |
|---|---|
| `tokenFile` is configured, and the file exists and parses | always |
| `certFile` and `keyFile` are configured | `env: prod` only |
| `logDestination` is a known name, or a path whose directory exists | always |
| `listenHost` is empty, an IP address or `localhost` | always |
| every entry of `allowedIPs`/`blockedIPs` parses as IP or CIDR | always |

The IP lists are the reason for the last row: a typo there would otherwise
become a rule that silently never matches.

### Example configuration

Default location: `/opt/demo_app/etc/config.yaml`. Environment variables are
expanded inside the file as described [above](#environment-variables).

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

  # Token file, managed with "demo_app token add|remove|list"; relative to this file
  tokenFile: token-hashes

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

Everything under `/opt/demo_app` belongs to root; the group `demo_app` may only
read `etc/`. The service user must not be able to write its configuration, its
token file or its program — whoever can write the token file adds a token of
their own. Only `data/` belongs to the service, for a service that stores
something.

### 1. Create system user and directories

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin demo_app
sudo install -d -o root -g root -m 0755 /opt/demo_app /opt/demo_app/bin
sudo install -d -o root -g demo_app -m 0750 /opt/demo_app/etc
sudo install -d -o demo_app -g demo_app -m 0750 /opt/demo_app/data
```

### 2. Copy files and create a token

```sh
sudo install -o root -g root -m 0755 demo_app /opt/demo_app/bin/demo_app
sudo install -o root -g demo_app -m 0640 config.yaml /opt/demo_app/etc/config.yaml
sudo install -o root -g demo_app -m 0640 key.pem /opt/demo_app/etc/key.pem
sudo install -o root -g root -m 0644 cert.pem /opt/demo_app/etc/cert.pem
sudo /opt/demo_app/bin/demo_app token add browser   # creates etc/token-hashes
```

### 3. Create systemd service

```sh
sudo tee /etc/systemd/system/demo_app.service > /dev/null <<'EOF'
[Unit]
Description=demo_app — golib service template
After=network-online.target
Wants=network-online.target

[Service]
User=demo_app
Group=demo_app
Type=simple
ExecStart=/opt/demo_app/bin/demo_app
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5

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

Send `SIGHUP` to reload the configuration and the token file without restarting
the process:

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
sudo systemctl restart demo_app
```

---

## License

MIT
