package mdcli

import (
	"io"
	"net/http"
	"os"

	mdlib "github.com/spinnaker/md-lib-go"
)

// CommandOptions are global options available for each command
type CommandOptions struct {
	ConfigDir  string
	ConfigFile string
	BaseURL    string
	HTTPClient func(*http.Request) (*http.Response, error)
	Logger     mdlib.Logger
	Stdout     FdWriter
	Stderr     io.Writer
	Stdin      FdReader
}

// NewCommandOptions creates a new CommandOptions struct with a default logger and stdio
func NewCommandOptions() *CommandOptions {
	return &CommandOptions{
		HTTPClient: http.DefaultClient.Do,
		Logger:     mdlib.NewDefaultLogger(),
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Stdin:      os.Stdin,
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
