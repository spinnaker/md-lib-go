package spincmds

import (
	"io"
	"log"
	"os"
)

// CommandOptions are global options available for each command
type CommandOptions struct {
	AppName        string
	ServiceAccount string
	ConfigDir      string
	ConfigFile     string
	BaseURL        string
	Logger         Logger
	Stdout         FdWriter
	Stderr         io.Writer
	Stdin          FdReader
}

// NewCommandOptions creates a new CommandOptions struct with a default logger and stdio
func NewCommandOptions() *CommandOptions {
	return &CommandOptions{
		Logger: log.New(os.Stderr, "", log.LstdFlags),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
	}
}

// FdWriter represents an io.Writer with a Fd property. (*os.File implements this)
type FdWriter interface {
	io.Writer
	Fd() uintptr
}

// FdReader represents an io.Reader with a Fd property. (*os.File implements this)
type FdReader interface {
	io.Reader
	Fd() uintptr
}

// Logger is a simple interface to abstract the logger implementation.  Go core `log` is used by default.
type Logger interface {
	Printf(format string, v ...interface{})
}
