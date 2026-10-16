// Package main provides the entry point for the demo_app application.
//
// This program initializes logging, loads configuration, handles command-line flags,
// and starts the main application loop. It supports hot reloads of the config
// and provides about/version/help output.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/womat/golib/demo/demo_app/app"
	"github.com/womat/golib/v2/web"
	"github.com/womat/golib/v2/xlog"
	"gopkg.in/yaml.v3"
)

// Readme embeds the README.md file for the --help flag
//
//go:embed README.md
var Readme string

// exitConfig is the exit code for a config file that does not load or validate (EX_CONFIG from
// sysexits.h). The unit sets RestartPreventExitStatus=78: restarting cannot fix the file, so
// systemd leaves demo_app stopped and "systemctl status demo_app" shows why, instead of
// restarting it every few seconds.
const exitConfig = 78

// buildDate and buildCommit are injected at build time via -ldflags
var (
	buildDate   = "dev"
	buildCommit = "none"
)

// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				A token from "demo_app token add <name>", sent as "Bearer <token>".
func main() {
	defaultConfig := filepath.Join("/opt", app.MODULE, "etc", "config.yaml")
	if envCfg := os.Getenv("CONFIG_FILE"); envCfg != "" {
		defaultConfig = envCfg
	}

	// "demo_app token add|remove|list" manages the bearer tokens in webserver.tokenFile.
	if len(os.Args) > 1 && os.Args[1] == "token" {
		os.Exit(web.TokenCommand{
			App:           app.MODULE,
			DefaultConfig: defaultConfig,
			TokenFile:     app.TokenFile,
			Apply:         "Apply with: systemctl reload " + app.MODULE,
		}.Run(os.Args[2:]))
	}

	// Parse command line flags.
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.SetOutput(os.Stdout)

	about := flags.Bool("about", false, "Print app details and exit")
	help := flags.Bool("help", false, "Print a help message and exit")
	version := flags.Bool("version", false, "Print the app version and exit")
	debug := flags.Bool("debug", false, "Enable debug logging to stdout (overrides log settings from the config file)")
	configFile := flags.String("config", defaultConfig, "Specify the path to the config file")

	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Println("Error parsing flags:", err)
		flags.Usage()
		os.Exit(1)
	}

	switch {
	case *about:
		fmt.Println(About())
		os.Exit(0)
	case *version:
		fmt.Println(app.VERSION)
		os.Exit(0)
	case *help:
		fmt.Println(Readme)
		os.Exit(0)
	}

	// Run main application loop
	os.Exit(run(*configFile, *debug))
}

// run initializes configuration, logging, and the application loop.
// It supports hot-reloading of the configuration and handles graceful shutdown.
func run(configFile string, debug bool) int {

	var logger *xlog.LoggerWrapper
	defer func() { closeLogger(logger) }()

	fmt.Printf("Starting %s %s\n", app.MODULE, app.VERSION)

	for {
		// Reload configuration on every restart
		config, err := loadConfig(configFile, debug)
		if err != nil {
			fmt.Printf("Failed to load config file %s: %s\n", configFile, err.Error())
			return exitConfig
		}

		// Close previous logger if exists
		closeLogger(logger)

		// Initialize logger
		if logger, err = xlog.Init(config.LogDestination, config.LogLevel); err != nil {
			fmt.Printf("Failed to initialize logger: %s\n", err.Error())
			return exitConfig
		}

		slog.SetDefault(logger.Logger)
		slog.Info("Logging initialized/reloaded", "logLevel", config.LogLevel)

		// Create and run the application
		a, err := app.New(config, filepath.Join("/opt", app.MODULE)).Run()
		if err != nil {
			slog.Error("Critical error occurred, shutting down", "error", err)
			return 1
		}

		// Wait for restart or shutdown signals
		select {
		case <-a.Restart():
			slog.Info("Reloading configuration", "configFile", configFile)
			time.Sleep(time.Second) // prevent tight restart loops
		case <-a.Shutdown():
			slog.Info("Shutdown requested")
			return 0
		}
	}
}

// closeLogger releases the log file and reports a failure on stderr.
//
// Stderr, not slog: this runs while the logger is being torn down or replaced,
// so the log itself is exactly the channel that cannot be trusted here. A
// failing Close means buffered lines did not make it to disk, which is worth
// knowing.
func closeLogger(logger *xlog.LoggerWrapper) {
	if logger == nil {
		return
	}

	if err := logger.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to close the log file: %s\n", err)
	}
}

func About() string {
	info := map[string]string{
		"Author":   "Wolfgang Mathe",
		"Binary":   filepath.Join("/opt", app.MODULE, "bin", app.MODULE),
		"Date":     buildDate,
		"Commit":   buildCommit,
		"Desc":     app.MODULE + " is a demo app",
		"Help":     filepath.Join("/opt", app.MODULE, "bin", app.MODULE) + " --help",
		"Main":     filepath.Join("/opt/src", app.MODULE, "cmd", app.MODULE, "main.go"),
		"ProgLang": runtime.Version(),
		"Repo":     "https://github.com/womat/" + app.MODULE + ".git",
		"Version":  app.VERSION,
	}

	b, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Sprintf("Failed to marshal About info: %v", err)
	}
	return string(b)
}

// loadConfig loads the configuration from a YAML file, applies overrides and validates the configuration values.
//
// If debug mode is enabled, log level is forced to "debug" and logs are written to stdout.
func loadConfig(configFile string, debug bool) (*app.Config, error) {

	config, err := app.LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	if debug {
		config.LogLevel = "debug"
		config.LogDestination = "stdout"
	}

	if err = config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}
