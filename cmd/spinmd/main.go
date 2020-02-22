package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
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

type exitCode int

func (e exitCode) Error() string { return "" }

func main() {
	opts := NewCommandOptions()
	flag.StringVar(&opts.AppName, "app", "", "spinnaker application name")
	flag.StringVar(&opts.ServiceAccount, "service-account", "", "spinnaker service account")
	flag.StringVar(&opts.ConfigDir, "dir", mdlib.DefaultDeliveryConfigDirName, "directory for delivery config file")
	flag.StringVar(&opts.ConfigFile, "file", mdlib.DefaultDeliveryConfigFileName, "delivery config file name")
	flag.StringVar(&opts.BaseURL, "baseurl", "", "base URL to reach spinnaker api")

	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		fmt.Printf("Usage: %s [flags] export|publish|diff\n", filepath.Base(os.Args[0]))
		fmt.Printf("Flags:\n")
		flag.PrintDefaults()
		return
	}

	var err error
	switch args[0] {
	case "export":
		exportOpts := ExportOptions{
			CommandOptions: *opts,
		}
		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		exportFlags.BoolVar(&exportOpts.All, "all", false, "export all options, skip prompt")
		exportFlags.StringVar(&exportOpts.EnvName, "env", "", "assing exported resources to given environment, skip prompt")
		exportFlags.Parse(args[1:])
		err = ExportCmd(&exportOpts)
	case "publish":
		err = PublishCmd(opts)
	case "diff":
		err = DiffCmd(opts)
	default:
		log.Fatalf(`Unexpected command %q, expected "export", "publish", or "diff" command`, args[0])
	}

	if err != nil {
		if code, ok := err.(*exitCode); ok {
			os.Exit(int(*code))
		}
		log.Fatalf("ERROR: %s", stacktrace.RootCause(err))
	}
}
