package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/womat/golib/v2/web"
)

// writeConfig puts content into a temporary file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// validConfig is a configuration that passes Validate, as a starting point for
// the cases that change one thing about it.
func validConfig() *Config {
	cfg := NewConfig()
	cfg.Webserver.TokenFile = "/opt/demo_app/etc/token-hashes"
	return cfg
}

func TestLoadConfigDefaults(t *testing.T) {
	path := writeConfig(t, "logLevel: debug\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.Env != DevEnv {
		t.Errorf("Env = %q, want the default %q", cfg.Env, DevEnv)
	}
	if cfg.Webserver.ListenPort != 8443 {
		t.Errorf("ListenPort = %d, want the default 8443", cfg.Webserver.ListenPort)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("LoadConfig accepted a path that does not exist")
	}
}

func TestLoadConfigDirectory(t *testing.T) {
	if _, err := LoadConfig(t.TempDir()); err == nil {
		t.Error("LoadConfig accepted a directory")
	}
}

func TestLoadConfigExpandsBracedVariables(t *testing.T) {
	t.Setenv("DEMO_APP_TEST_KEY", "from-the-environment")

	path := writeConfig(t, "webserver:\n  certFile: ${DEMO_APP_TEST_KEY}\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.CertFile != "from-the-environment" {
		t.Errorf("CertFile = %q, want the expanded value", cfg.Webserver.CertFile)
	}
}

func TestLoadConfigLeavesBareDollarAlone(t *testing.T) {
	// The whole point of the restricted expansion: a password keeps its dollars.
	t.Setenv("w0rd", "expanded")

	path := writeConfig(t, "webserver:\n  certFile: pa$$w0rd\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.CertFile != "pa$$w0rd" {
		t.Errorf("CertFile = %q, want pa$$w0rd unchanged", cfg.Webserver.CertFile)
	}
}

func TestLoadConfigUndefinedVariable(t *testing.T) {
	os.Unsetenv("DEMO_APP_DEFINITELY_NOT_SET")

	path := writeConfig(t, "webserver:\n  certFile: ${DEMO_APP_DEFINITELY_NOT_SET}\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted an undefined variable instead of reporting it")
	}
	if !strings.Contains(err.Error(), "DEMO_APP_DEFINITELY_NOT_SET") {
		t.Errorf("error %q does not name the missing variable", err)
	}
}

// A misspelled or renamed key must not silently leave its setting at the default.
func TestLoadConfigRefusesUnknownKeys(t *testing.T) {
	path := writeConfig(t, "webserver:\n  listenport: 9000\n")

	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "listenport") {
		t.Errorf("err = %v, want one naming the unknown key", err)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	path := writeConfig(t, "webserver:\n\tcertFile: tabs are not yaml\n")

	if _, err := LoadConfig(path); err == nil {
		t.Error("LoadConfig accepted invalid YAML")
	}
}

func TestValidateAcceptsADefaultDevConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr string
	}{
		{
			name:    "unknown environment",
			modify:  func(c *Config) { c.Env = "staging" },
			wantErr: "invalid environment",
		},
		{
			name:    "missing token file",
			modify:  func(c *Config) { c.Webserver.TokenFile = "" },
			wantErr: "tokenFile is not configured",
		},
		{
			name:    "unknown log level",
			modify:  func(c *Config) { c.LogLevel = "verbose" },
			wantErr: "invalid log level",
		},
		{
			name:    "empty log destination",
			modify:  func(c *Config) { c.LogDestination = "" },
			wantErr: "logDestination is not configured",
		},
		{
			name:    "log file in a directory that does not exist",
			modify:  func(c *Config) { c.LogDestination = "/definitely/not/here/app.log" },
			wantErr: "invalid logDestination",
		},
		{
			name:    "port out of range",
			modify:  func(c *Config) { c.Webserver.ListenPort = 70000 },
			wantErr: "invalid port",
		},
		{
			name:    "listen host is a name",
			modify:  func(c *Config) { c.Webserver.ListenHost = "demo.example" },
			wantErr: "invalid listenHost",
		},
		{
			name:   "listen host localhost is fine",
			modify: func(c *Config) { c.Webserver.ListenHost = "localhost" },
		},
		{
			name: "prod without certificate",
			modify: func(c *Config) {
				c.Env = ProdEnv
			},
			wantErr: "certFile and keyFile",
		},
		{
			name: "prod with certificate",
			modify: func(c *Config) {
				c.Env = ProdEnv
				c.Webserver.CertFile = "/etc/cert.pem"
				c.Webserver.KeyFile = "/etc/key.pem"
			},
		},
		{
			name:    "typo in allowedIPs",
			modify:  func(c *Config) { c.Webserver.AllowedIPs = []string{"192.168.0.1", "192.168.0.300"} },
			wantErr: "allowedIPs: invalid IP address",
		},
		{
			name:    "typo in a CIDR",
			modify:  func(c *Config) { c.Webserver.BlockedIPs = []string{"10.0.0.0/99"} },
			wantErr: "blockedIPs: invalid CIDR",
		},
		{
			name: "valid ip lists",
			modify: func(c *Config) {
				c.Webserver.AllowedIPs = []string{"192.168.0.1", "10.0.0.0/8", "::1"}
				c.Webserver.BlockedIPs = []string{"2001:db8::/32"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(cfg)

			err := cfg.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate: %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate accepted the config, want an error about %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestIsDevEnv(t *testing.T) {
	if !(&Config{Env: DevEnv}).IsDevEnv() {
		t.Error("dev is not recognised as the development environment")
	}
	if (&Config{Env: ProdEnv}).IsDevEnv() {
		t.Error("prod is recognised as the development environment")
	}
}

func TestLoadConfigIgnoresComments(t *testing.T) {
	// Documenting the syntax in the file itself must not demand that the
	// variable exists.
	os.Unsetenv("NAME")

	path := writeConfig(t, "# reference variables as ${NAME}\nwebserver:\n  certFile: a-key # or ${ALSO_UNSET}\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.CertFile != "a-key" {
		t.Errorf("CertFile = %q, want a-key", cfg.Webserver.CertFile)
	}
}

func TestLoadConfigExpandsInsideQuotedValueWithHash(t *testing.T) {
	t.Setenv("DEMO_APP_HASH_KEY", "value-with")

	path := writeConfig(t, `webserver:`+"\n"+`  certFile: "${DEMO_APP_HASH_KEY}#not-a-comment"`+"\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.CertFile != "value-with#not-a-comment" {
		t.Errorf("CertFile = %q, want value-with#not-a-comment", cfg.Webserver.CertFile)
	}
}

func TestLoadConfigIgnoresCommentAfterApostrophe(t *testing.T) {
	// The apostrophe is part of a plain value, not the start of a quoted one,
	// so the "#" behind it still starts a comment.
	os.Unsetenv("DEMO_APP_OLD_CERT")

	path := writeConfig(t, "webserver:\n  certFile: wolfgang's.pem # was ${DEMO_APP_OLD_CERT}\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.CertFile != "wolfgang's.pem" {
		t.Errorf("CertFile = %q, want wolfgang's.pem", cfg.Webserver.CertFile)
	}
}

func TestSplitYAMLComment(t *testing.T) {
	tests := []struct {
		line, value, comment string
	}{
		{"# whole line", "", "# whole line"},
		{"key: value # c", "key: value ", "# c"},
		{"key: a#b", "key: a#b", ""},
		{`key: "a # b" # c`, `key: "a # b" `, "# c"},
		{"key: 'a # b' # c", "key: 'a # b' ", "# c"},
		{"key: it's # c", "key: it's ", "# c"},
		{`key: say "hi # there"`, `key: say "hi `, `# there"`},
		{`key: "a\" # b" # c`, `key: "a\" # b" `, "# c"},
		{"key: 'it''s # x' # c", "key: 'it''s # x' ", "# c"},
		{"  - 'a # b' # c", "  - 'a # b' ", "# c"},
		{`key: [ 'a # b', "c # d" ] # e`, `key: [ 'a # b', "c # d" ] `, "# e"},
		{"key: {k: 'a # b'} # c", "key: {k: 'a # b'} ", "# c"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			value, comment := splitYAMLComment(tt.line)
			if value != tt.value || comment != tt.comment {
				t.Errorf("got (%q, %q), want (%q, %q)", value, comment, tt.value, tt.comment)
			}
		})
	}
}

func TestLoadConfigReadsTheTokenFile(t *testing.T) {
	dir := t.TempDir()
	hash := web.HashToken("the-token")
	if err := web.AddTokenToFile(filepath.Join(dir, "token-hashes"), "nodered", hash); err != nil {
		t.Fatalf("AddTokenToFile: %v", err)
	}
	path := filepath.Join(dir, "config.yaml")
	// A relative tokenFile is resolved against the directory of the config file.
	if err := os.WriteFile(path, []byte("webserver:\n  tokenFile: token-hashes\n"), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if want := filepath.Join(dir, "token-hashes"); cfg.Webserver.TokenFile != want {
		t.Errorf("TokenFile = %q, want %q", cfg.Webserver.TokenFile, want)
	}
	if len(cfg.Webserver.Tokens) != 1 || cfg.Webserver.Tokens[0].Name != "nodered" || cfg.Webserver.Tokens[0].Hash != hash {
		t.Errorf("Tokens = %+v, want the one token of the file", cfg.Webserver.Tokens)
	}
}

func TestLoadConfigMissingTokenFile(t *testing.T) {
	path := writeConfig(t, "webserver:\n  tokenFile: token-hashes\n")

	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "token file") {
		t.Errorf("LoadConfig = %v, want an error about the missing token file", err)
	}

	// "demo_app token add" has to work before the file exists.
	got, err := TokenFile(path)
	if err != nil {
		t.Fatalf("TokenFile: %v", err)
	}
	if want := filepath.Join(filepath.Dir(path), "token-hashes"); got != want {
		t.Errorf("TokenFile = %q, want %q", got, want)
	}
}

// The config file that ships with the template must load: with unknown keys refused, a renamed
// field would otherwise break every copy of it.
func TestShippedConfigHasOnlyKnownKeys(t *testing.T) {
	if _, err := TokenFile("../config/config.yaml"); err != nil {
		t.Fatal(err)
	}
}
