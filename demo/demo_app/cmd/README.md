# demo_app

**demo_app** is the runnable entry point of the service template: it parses the
flags below, loads the configuration, initialises logging, and hands control to
the `app` package. See [`../README.md`](../README.md) for what the template is
for and how it is put together.

---

## Usage

```text
demo_app [--config FILE] [--debug] [--version] [--about] [--help]
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

The configuration file is a YAML file. By default it is loaded from `/opt/demo_app/etc/config.yaml`.

Environment variables are expanded inside the file in the braced form only,
e.g. `apiKey: ${DEMO_APP_API_KEY}`; a referenced variable that is not set is a
startup error. See [`../README.md`](../README.md#environment-variables).
