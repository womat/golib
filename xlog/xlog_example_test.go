package xlog_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/womat/golib/xlog"
)

// ExampleInit zeigt die Verwendung von xlog mit stdout, Datei und null logging.
// Die stdout-Ausgaben werden für den Example-Test auf null geleitet, damit nur
// die fmt.Println-Zeilen geprüft werden; das Dateiziel wird dagegen wirklich
// benutzt, in einem temporären Verzeichnis.
func ExampleInit() {
	// 1️⃣ Logging to stdout with debug messages (AddSource enabled)
	// Für den Example-Test auf null, damit Output-Match funktioniert
	stdoutLogger, err := xlog.Init("null", "debug")
	if err != nil {
		panic(err)
	}
	defer stdoutLogger.Close()

	stdoutLogger.Debug("Debug message on stdout")
	stdoutLogger.Info("Info message on stdout")
	stdoutLogger.Warn("Warning message on stdout")
	stdoutLogger.Error("Error message on stdout")

	fmt.Println("---")

	// 2️⃣ Logging to a file with warning level
	dir, err := os.MkdirTemp("", "xlog-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	logFile := filepath.Join(dir, "app.log")
	fileLogger, err := xlog.Init(logFile, "warning")
	if err != nil {
		panic(err)
	}

	fileLogger.Debug("This debug will NOT appear in file")
	fileLogger.Info("This info will NOT appear in file")
	fileLogger.Warn("This warning WILL appear in file")
	fileLogger.Error("This error WILL appear in file")

	// Close releases the file handle; read the file back afterwards.
	if err = fileLogger.Close(); err != nil {
		panic(err)
	}

	content, err := os.ReadFile(logFile)
	if err != nil {
		panic(err)
	}

	// Count the lines instead of comparing them: each one carries a timestamp.
	fmt.Printf("log file contains %d lines\n", bytes.Count(content, []byte("\n")))

	// 3️⃣ Discarding all logs (useful for tests)
	nullLogger, _ := xlog.Init("null", "debug")
	nullLogger.Debug("This will not appear anywhere")
	nullLogger.Info("Nor this info")
	nullLogger.Warn("Nor this warning")
	nullLogger.Error("Nor this error")

	fmt.Println("Null logger demo complete")

	// Output:
	// ---
	// log file contains 2 lines
	// Null logger demo complete
}
