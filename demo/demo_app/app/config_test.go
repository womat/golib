package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	cfg.Webserver.ApiKey = "a-real-key"
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

	path := writeConfig(t, "webserver:\n  apiKey: ${DEMO_APP_TEST_KEY}\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.ApiKey != "from-the-environment" {
		t.Errorf("ApiKey = %q, want the expanded value", cfg.Webserver.ApiKey)
	}
}

func TestLoadConfigLeavesBareDollarAlone(t *testing.T) {
	// The whole point of the restricted expansion: a password keeps its dollars.
	t.Setenv("w0rd", "expanded")

	path := writeConfig(t, "webserver:\n  apiKey: pa$$w0rd\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.ApiKey != "pa$$w0rd" {
		t.Errorf("ApiKey = %q, want pa$$w0rd unchanged", cfg.Webserver.ApiKey)
	}
}

func TestLoadConfigUndefinedVariable(t *testing.T) {
	os.Unsetenv("DEMO_APP_DEFINITELY_NOT_SET")

	path := writeConfig(t, "webserver:\n  apiKey: ${DEMO_APP_DEFINITELY_NOT_SET}\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("LoadConfig accepted an undefined variable instead of reporting it")
	}
	if !strings.Contains(err.Error(), "DEMO_APP_DEFINITELY_NOT_SET") {
		t.Errorf("error %q does not name the missing variable", err)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	path := writeConfig(t, "webserver:\n\tapiKey: tabs are not yaml\n")

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
			name:    "missing api key",
			modify:  func(c *Config) { c.Webserver.ApiKey = "" },
			wantErr: "ApiKey is not configured",
		},
		{
			name: "placeholder api key in prod",
			modify: func(c *Config) {
				c.Env = ProdEnv
				c.Webserver.ApiKey = placeholderApiKey
				c.Webserver.CertFile = "/etc/cert.pem"
				c.Webserver.KeyFile = "/etc/key.pem"
			},
			wantErr: "placeholder",
		},
		{
			name:   "placeholder api key in dev is allowed",
			modify: func(c *Config) { c.Webserver.ApiKey = placeholderApiKey },
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
			name:    "jwt secret without id",
			modify:  func(c *Config) { c.Webserver.JwtSecret = "s3cr3t" },
			wantErr: "must be configured together",
		},
		{
			name:    "jwt id without secret",
			modify:  func(c *Config) { c.Webserver.JwtID = "an-id" },
			wantErr: "must be configured together",
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
			wantErr: "invalid IP address in allowedIPs",
		},
		{
			name:    "typo in a CIDR",
			modify:  func(c *Config) { c.Webserver.BlockedIPs = []string{"10.0.0.0/99"} },
			wantErr: "invalid CIDR in blockedIPs",
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

	path := writeConfig(t, "# reference variables as ${NAME}\nwebserver:\n  apiKey: a-key # or ${ALSO_UNSET}\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.ApiKey != "a-key" {
		t.Errorf("ApiKey = %q, want a-key", cfg.Webserver.ApiKey)
	}
}

func TestLoadConfigExpandsInsideQuotedValueWithHash(t *testing.T) {
	t.Setenv("DEMO_APP_HASH_KEY", "value-with")

	path := writeConfig(t, `webserver:`+"\n"+`  apiKey: "${DEMO_APP_HASH_KEY}#not-a-comment"`+"\n")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Webserver.ApiKey != "value-with#not-a-comment" {
		t.Errorf("ApiKey = %q, want value-with#not-a-comment", cfg.Webserver.ApiKey)
	}
}
