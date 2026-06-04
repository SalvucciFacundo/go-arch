package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/mgutz/ansi"
)

// Out is the destination for UI logging/printing. In MCP mode, it should be set to os.Stderr.
var Out io.Writer = os.Stdout

// Success prints a success message in bold green.
func Success(msg string) {
	fmt.Fprintf(Out, "%s %s\n", ansi.Color("SUCCESS:", "green+b"), msg)
}

// Warning prints a warning message in bold yellow.
func Warning(msg string) {
	fmt.Fprintf(Out, "%s %s\n", ansi.Color("WARNING:", "yellow+b"), msg)
}

// Error prints an error message in bold red.
func Error(msg string) {
	fmt.Fprintf(Out, "%s %s\n", ansi.Color("ERROR:", "red+b"), msg)
}

// Info prints an info message in bold blue.
func Info(msg string) {
	fmt.Fprintf(Out, "%s %s\n", ansi.Color("INFO:", "blue+b"), msg)
}

// Analyzing prints a special message for the "check" command.
func Analyzing(architecture string) {
	fmt.Fprintf(Out, "🔍 %s **%s**...\n\n", ansi.Color("Analizando arquitectura", "cyan+b"), architecture)
}

// Fatal prints an error message and exits the program.
func Fatal(err error) {
	fmt.Fprintf(Out, "%s %v\n", ansi.Color("FATAL:", "red+b"), err)
	os.Exit(1)
}

// SuccessMsg returns a success message in bold green.
func SuccessMsg(msg string) string {
	return fmt.Sprintf("%s %s", ansi.Color("SUCCESS:", "green+b"), msg)
}

// WarningMsg returns a warning message in bold yellow.
func WarningMsg(msg string) string {
	return fmt.Sprintf("%s %s", ansi.Color("WARNING:", "yellow+b"), msg)
}

// ErrorMsg returns an error message in bold red.
func ErrorMsg(msg string) string {
	return fmt.Sprintf("%s %s", ansi.Color("ERROR:", "red+b"), msg)
}

// InfoMsg returns an info message in bold blue.
func InfoMsg(msg string) string {
	return fmt.Sprintf("%s %s", ansi.Color("INFO:", "blue+b"), msg)
}
