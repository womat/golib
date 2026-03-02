# 🚀 demo_app — bla bla

description

---

## Features

- Exposes data via HTTPS REST API (with API key or JWT authentication)
- IP allowlist / blocklist support

---

## API

### Get meter data

```sh
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/data
```

### Get application version

```sh
curl -k https://localhost:8443/version
```

### Health check

```sh
curl -k https://localhost:8443/health
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


## Command Line Flags

| Flag        | Default                        | Description                                       |
|-------------|--------------------------------|---------------------------------------------------|
| `--config`  | `/opt/demo_app/etc/config.yaml` | Path to the config file                           |
| `--debug`   | `false`                        | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                        | Print the app version and exit                    |
| `--about`   | `false`                        | Print app details and exit                        |
| `--help`    | `false`                        | Print a help message and exit                     |

---

## Configuration

The configuration file is located at `/opt/demo_app/etc/config.yaml`. See the included `config.yaml` for all available
options and documentation.

---

## License

MIT
