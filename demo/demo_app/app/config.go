package app

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/womat/golib/v2/web"
	"gopkg.in/yaml.v3"
)

const (
	ProdEnv = "prod"
	DevEnv  = "dev"
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
	ListenHost string      `yaml:"listenHost"` // Host address for web server
	ListenPort int         `yaml:"listenPort"` // Port for web server
	TokenFile  string      `yaml:"tokenFile"`  // File with the accepted bearer tokens as SHA-256 hashes
	Tokens     []web.Token `yaml:"-"`          // Read from TokenFile by LoadConfig
	KeyFile    string      `yaml:"keyFile"`    // SSL private key file
	CertFile   string      `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string    `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string    `yaml:"allowedIPs"` // Allowed IP addresses or networks
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

// LoadConfig loads configuration from a YAML file and expands environment variables,
// then reads the bearer tokens from webserver.tokenFile.
//
// A token file that is missing or broken is an error: the service would otherwise
// start and reject every request with 401. Since a config reload goes through
// here, a token added with "demo_app token add" applies on reload.
func LoadConfig(fileName string) (*Config, error) {
	cfg, err := readConfig(fileName)
	if err != nil {
		return cfg, err
	}

	if cfg.Webserver.TokenFile != "" {
		if cfg.Webserver.Tokens, err = web.ReadTokenFile(cfg.Webserver.TokenFile); err != nil {
			return cfg, fmt.Errorf("token file: %w", err)
		}
	}

	return cfg, nil
}

// TokenFile returns webserver.tokenFile of the config file, without reading the
// token file itself - "demo_app token add" has to work before that file exists.
func TokenFile(fileName string) (string, error) {
	cfg, err := readConfig(fileName)
	if err != nil {
		return "", err
	}
	return cfg.Webserver.TokenFile, nil
}

// readConfig loads the config file alone. A relative tokenFile is resolved
// against the directory of the config file, so the service and "demo_app token"
// find the same file wherever they are started from.
func readConfig(fileName string) (*Config, error) {
	cfg := NewConfig()

	// A directory fails here too, with "is a directory".
	content, err := os.ReadFile(fileName)
	if err != nil {
		return cfg, err
	}

	// Replace environment variables in the YAML
	replaced, err := expandEnv(string(content))
	if err != nil {
		return cfg, err
	}

	// Unknown keys are an error rather than ignored, so a misspelled or renamed key cannot
	// silently leave its setting at the default.
	dec := yaml.NewDecoder(strings.NewReader(replaced))
	dec.KnownFields(true)
	if err = dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if f := cfg.Webserver.TokenFile; f != "" && !filepath.IsAbs(f) {
		cfg.Webserver.TokenFile = filepath.Join(filepath.Dir(fileName), f)
	}

	return cfg, nil
}

// expandEnv replaces every ${NAME} with the value of that environment variable.
//
// Unlike os.ExpandEnv it does not touch a bare $NAME, so a "$" inside a value -
// in a password, say - stays what it is. An undefined variable is an error
// rather than an empty string, because an empty secret is the kind of mistake
// that only shows up in production.
//
// Comments are found line by line, so in a value that spans lines - a quoted
// one or a block scalar (| or >) - a "#" behind a blank is taken for a comment
// and a ${NAME} after it stays as it is. No configuration of these services
// has such a value; that is why this is not worth a YAML tokenizer.
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
// is tracked here. A quote opens a scalar only where a value begins, so the
// apostrophe in a plain value like it's does not hide the comment behind it.
func splitYAMLComment(line string) (value, comment string) {
	var quote byte // the quote of the open scalar, 0 outside of one

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == '"' && c == '\\':
			i++ // an escaped character cannot close the scalar
		case quote != 0 && c == quote:
			if quote == '\'' && i+1 < len(line) && line[i+1] == '\'' {
				i++ // '' is a quote inside a single-quoted scalar
			} else {
				quote = 0
			}
		case quote == 0 && (c == '\'' || c == '"') && opensScalar(line[:i]):
			quote = c
		case quote == 0 && c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			return line[:i], line[i:]
		}
	}

	return line, ""
}

// opensScalar reports whether a quote that follows before starts a quoted
// scalar: at the start of the line, behind a flow indicator "[", "{" or ",",
// or behind ":", "-" or "?" and a blank. A tag (!!str) or an anchor (&name)
// in between belongs to the value and is skipped.
func opensScalar(before string) bool {
	trimmed := strings.TrimRight(before, " \t")
	for {
		i := strings.LastIndexAny(trimmed, " \t")
		if word := trimmed[i+1:]; word == "" || (word[0] != '!' && word[0] != '&') {
			break
		}
		trimmed = strings.TrimRight(trimmed[:i+1], " \t")
	}
	if trimmed == "" {
		return true
	}

	switch trimmed[len(trimmed)-1] {
	case '[', '{', ',':
		return true
	case ':', '-', '?':
		return len(trimmed) < len(before)
	}
	return false
}

// IsDevEnv returns true if the environment is development.
func (c *Config) IsDevEnv() bool {
	return c.Env == DevEnv
}

// Validate checks the Config for invalid or missing values.
//
// It is stricter in ProdEnv: a production deployment has to name its TLS
// certificate. Everything that can be decided without touching the
// network is decided here, so that the service fails at startup instead of on
// the first request.
func (c *Config) Validate() error {

	if c.Env != ProdEnv && c.Env != DevEnv {
		return fmt.Errorf("invalid environment: %s, must be %s or %s", c.Env, ProdEnv, DevEnv)
	}

	if c.Webserver.TokenFile == "" {
		return errors.New(`webserver.tokenFile is not configured; create a token with "demo_app token add <name>"`)
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

	// The embedded development certificate is only a fallback for DevEnv, see
	// loadTLSCert. In production the files have to be named.
	if c.Env == ProdEnv {
		if c.Webserver.CertFile == "" || c.Webserver.KeyFile == "" {
			return errors.New("certFile and keyFile must be configured in " + ProdEnv)
		}
	}

	// The IP filter only logs and skips an unusable entry; a typo here would
	// become a rule that never matches, so it stops the start instead.
	if err := web.ValidateIPList(c.Webserver.AllowedIPs); err != nil {
		return fmt.Errorf("allowedIPs: %w", err)
	}
	if err := web.ValidateIPList(c.Webserver.BlockedIPs); err != nil {
		return fmt.Errorf("blockedIPs: %w", err)
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
