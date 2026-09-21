package app

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ProdEnv = "prod"
	DevEnv  = "dev"

	// placeholderApiKey is what config/config.yaml ships with. It must not
	// survive into a production deployment.
	placeholderApiKey = "changeme!"
)

// envVarPattern matches ${NAME}, the only form LoadConfig expands. A bare $NAME
// is left alone on purpose: a password like "pa$$w0rd" must survive the config
// file unchanged.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Config holds the main application configuration.
type Config struct {
	Env            string          `yaml:"env"`            // Application environment: dev | prod
	LogLevel       string          `yaml:"logLevel"`       // Log level: debug | info | warning | error
	LogDestination string          `yaml:"logDestination"` // Log output: stdout | stderr | /path/to/logfile
	Webserver      WebserverConfig `yaml:"webserver"`      // Webserver configuration
}

// WebserverConfig holds HTTPS server settings.
type WebserverConfig struct {
	ListenHost string   `yaml:"listenHost"` // Host address for web server
	ListenPort int      `yaml:"listenPort"` // Port for web server
	ApiKey     string   `yaml:"apiKey"`     // API key for requests
	JwtSecret  string   `yaml:"jwtSecret"`  // Secret for JWT tokens
	JwtID      string   `yaml:"jwtID"`      // Unique JWT ID
	KeyFile    string   `yaml:"keyFile"`    // SSL private key file
	CertFile   string   `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string `yaml:"allowedIPs"` // Allowed IP addresses or networks
}

// NewConfig returns a Config with sane defaults
func NewConfig() *Config {
	return &Config{
		Env:            DevEnv,
		LogLevel:       "info",
		LogDestination: "stdout",
		Webserver: WebserverConfig{
			ListenHost: "0.0.0.0",
			ListenPort: 8443,
			BlockedIPs: []string{},
			AllowedIPs: []string{},
		},
	}
}

// LoadConfig loads configuration from a YAML file and expands environment variables.
func LoadConfig(fileName string) (*Config, error) {
	cfg := NewConfig()

	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return cfg, err
	}
	if fileInfo.IsDir() {
		return cfg, errors.New("config path is a directory, not a file")
	}

	content, err := os.ReadFile(fileName)
	if err != nil {
		return cfg, err
	}

	// Replace environment variables in the YAML
	replaced, err := expandEnv(string(content))
	if err != nil {
		return cfg, err
	}

	// Unmarshal YAML into the config struct
	if err = yaml.Unmarshal([]byte(replaced), cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// expandEnv replaces every ${NAME} with the value of that environment variable.
//
// Unlike os.ExpandEnv it does not touch a bare $NAME, so a "$" inside a value -
// in a password, say - stays what it is. An undefined variable is an error
// rather than an empty string, because an empty secret is the kind of mistake
// that only shows up in production.
func expandEnv(content string) (string, error) {
	var (
		missing  []string
		expanded strings.Builder
	)

	for line := range strings.SplitSeq(content, "\n") {
		if expanded.Len() > 0 {
			expanded.WriteString("\n")
		}

		// Comments are left alone, so that documenting the ${NAME} syntax in
		// the configuration file does not itself demand that NAME is set.
		value, comment := splitYAMLComment(line)

		expanded.WriteString(envVarPattern.ReplaceAllStringFunc(value, func(match string) string {
			name := envVarPattern.FindStringSubmatch(match)[1]

			env, ok := os.LookupEnv(name)
			if !ok {
				missing = append(missing, name)
				return match
			}
			return env
		}))
		expanded.WriteString(comment)
	}

	if len(missing) > 0 {
		return "", fmt.Errorf("undefined environment variable(s) in config: %s", strings.Join(missing, ", "))
	}

	return expanded.String(), nil
}

// splitYAMLComment splits a line into its value part and its trailing comment.
//
// In YAML a "#" starts a comment when it is at the start of the line or
// preceded by a blank, and never inside a quoted scalar - which is exactly what
// is tracked here.
func splitYAMLComment(line string) (value, comment string) {
	var inSingle, inDouble bool

	for i, r := range line {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == '#' && !inSingle && !inDouble && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i], line[i:]
		}
	}

	return line, ""
}

// IsDevEnv returns true if the environment is development.
func (c *Config) IsDevEnv() bool {
	return c.Env == DevEnv
}

// Validate checks the Config for invalid or missing values.
//
// It is stricter in ProdEnv: a production deployment has to name its TLS
// certificate and must not run with the placeholder API key of the shipped
// example configuration. Everything that can be decided without touching the
// network is decided here, so that the service fails at startup instead of on
// the first request.
func (c *Config) Validate() error {

	if c.Env != ProdEnv && c.Env != DevEnv {
		return fmt.Errorf("invalid environment: %s, must be %s or %s", c.Env, ProdEnv, DevEnv)
	}

	if c.Webserver.ApiKey == "" {
		return errors.New("ApiKey is not configured")
	}

	if c.Webserver.ApiKey == placeholderApiKey && c.Env == ProdEnv {
		return fmt.Errorf("ApiKey is still the placeholder %q from the example configuration", placeholderApiKey)
	}

	validLogLevels := []string{"debug", "info", "warning", "warn", "error"}
	if !slices.Contains(validLogLevels, c.LogLevel) {
		return fmt.Errorf("invalid log level: %s, must be one of %v", c.LogLevel, validLogLevels)
	}

	if err := validateLogDestination(c.LogDestination); err != nil {
		return err
	}

	if c.Webserver.ListenPort < 1 || c.Webserver.ListenPort > 65535 {
		return fmt.Errorf("invalid port: %d", c.Webserver.ListenPort)
	}

	if err := validateListenHost(c.Webserver.ListenHost); err != nil {
		return err
	}

	// A half configured JWT setup silently means "JWT disabled", which is not
	// what someone who filled in one of the two fields had in mind.
	if (c.Webserver.JwtSecret == "") != (c.Webserver.JwtID == "") {
		return errors.New("jwtSecret and jwtID must be configured together")
	}

	// The embedded development certificate is only a fallback for DevEnv, see
	// loadTLSCert. In production the files have to be named.
	if c.Env == ProdEnv {
		if c.Webserver.CertFile == "" || c.Webserver.KeyFile == "" {
			return errors.New("certFile and keyFile must be configured in " + ProdEnv)
		}
	}

	for _, list := range []struct {
		name    string
		entries []string
	}{
		{"allowedIPs", c.Webserver.AllowedIPs},
		{"blockedIPs", c.Webserver.BlockedIPs},
	} {
		if err := validateIPList(list.name, list.entries); err != nil {
			return err
		}
	}

	return nil
}

// validateLogDestination accepts the well known names and, for anything else,
// requires the directory of the log file to exist - otherwise the failure only
// surfaces inside xlog.Init.
func validateLogDestination(dest string) error {
	switch strings.ToLower(strings.TrimSpace(dest)) {
	case "":
		return errors.New("logDestination is not configured")
	case "stdout", "stderr", "null":
		return nil
	}

	dir := filepath.Dir(dest)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("invalid logDestination %q: %w", dest, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid logDestination %q: %s is not a directory", dest, dir)
	}

	return nil
}

// validateListenHost accepts an empty host (all interfaces), any literal IP and
// localhost. A name that has to be resolved is refused, because a DNS lookup at
// startup is a dependency this service does not want.
func validateListenHost(host string) error {
	if host == "" || host == "localhost" {
		return nil
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("invalid listenHost: %s, must be empty, an IP address or localhost", host)
	}
	return nil
}

// validateIPList reports the first entry that is neither an IP address nor a
// CIDR network. A typo there would otherwise become a rule that never matches.
func validateIPList(name string, entries []string) error {
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)

		if strings.Contains(trimmed, "/") {
			if _, _, err := net.ParseCIDR(trimmed); err != nil {
				return fmt.Errorf("invalid CIDR in %s: %q", name, entry)
			}
			continue
		}
		if net.ParseIP(trimmed) == nil {
			return fmt.Errorf("invalid IP address in %s: %q", name, entry)
		}
	}
	return nil
}
