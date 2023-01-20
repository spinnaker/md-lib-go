package mdcli

import (
	"io"
	"log"
	"net/http"
	"os"
)

// CommandOptions are global options available for each command
type CommandOptions struct {
	ConfigDir  string
	ConfigFile string
	BaseURL    string
	HTTPClient func(*http.Request) (*http.Response, error)
	Logger     Logger
	Stdout     FdWriter
	Stderr     io.Writer
	Stdin      FdReader
}

// NewCommandOptions creates a new CommandOptions struct with a default logger and stdio
func NewCommandOptions() *CommandOptions {
	return &CommandOptions{
		HTTPClient: http.DefaultClient.Do,
		Logger:     defaultLogger{log.New(os.Stderr, "", log.LstdFlags)},
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Stdin:      os.Stdin,
	}
}

type defaultLogger struct {
	*log.Logger
}

var _ Logger = (*defaultLogger)(nil)

func (l defaultLogger) Noticef(format string, v ...any) {
	l.Printf("NOTICE: "+format, v...)
}

func (l defaultLogger) Errorf(format string, v ...any) {
	l.Printf("ERROR: "+format, v...)
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
	Printf(format string, v ...any)
	Noticef(format string, v ...any)
	Errorf(format string, v ...any)
}
