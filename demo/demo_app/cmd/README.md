# demo_app

**demo_app**  is an ...

## Features

- Persists counters to a YAML file for recovery after restart
- Exposes an HTTPS API for live readings
- Supports hot-reload of configuration

---

## Command-line Flags

| Flag        | Default                         | Description                                                         |
|-------------|---------------------------------|---------------------------------------------------------------------|
| `--config`  | `/opt/demo_app/etc/config.yaml` | Path to the configuration file                                      |
| `--debug`   | `false`                         | Enable debug logging to stdout (overrides log settings from config) |
| `--version` |                                 | Print the application version and exit                              |
| `--about`   |                                 | Print application details and exit                                  |
| `--help`    |                                 | Print this help message and exit                                    |

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

The configuration file is a YAML file. By default it is loaded from `/opt/demo_app/etc/config.yaml`.
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.
