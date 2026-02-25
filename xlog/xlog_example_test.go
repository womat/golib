package xlog_test

import (
	"fmt"

	"github.com/womat/golib/xlog"
)

// ExampleInit zeigt die Verwendung von xlog mit stdout, Datei und null logging.
// Für den Example-Test werden die tatsächlichen Log-Ausgaben auf null geleitet,
// damit der Test nur die fmt.Println-Zeilen überprüft.
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
	// Auch hier: auf null, die Datei wird nicht wirklich geschrieben im Test
	fileLogger, err := xlog.Init("null", "warning")
	if err != nil {
		panic(err)
	}
	defer fileLogger.Close()

	fileLogger.Debug("This debug will NOT appear in file")
	fileLogger.Info("This info will NOT appear in file")
	fileLogger.Warn("This warning WILL appear in file")
	fileLogger.Error("This error WILL appear in file")

	fmt.Println("Log written to app.log")

	// 3️⃣ Discarding all logs (useful for tests)
	nullLogger, _ := xlog.Init("null", "debug")
	nullLogger.Debug("This will not appear anywhere")
	nullLogger.Info("Nor this info")
	nullLogger.Warn("Nor this warning")
	nullLogger.Error("Nor this error")

	fmt.Println("Null logger demo complete")

	// Output:
	// ---
	// Log written to app.log
	// Null logger demo complete
}
