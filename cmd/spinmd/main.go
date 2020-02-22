package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
	"github.com/spinnaker/md-lib-go/spincmds"
)

func main() {
	opts := spincmds.NewCommandOptions()
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

	exitCode := 0
	var err error
	switch args[0] {
	case "export":
		exportOpts := spincmds.ExportOptions{
			CommandOptions: *opts,
		}
		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		exportFlags.BoolVar(&exportOpts.All, "all", false, "export all options, skip prompt")
		exportFlags.StringVar(&exportOpts.EnvName, "env", "", "assing exported resources to given environment, skip prompt")
		exportFlags.Parse(args[1:])
		err = spincmds.Export(&exportOpts)
	case "publish":
		err = spincmds.Publish(opts)
	case "diff":
		exitCode, err = spincmds.Diff(opts)
	default:
		log.Fatalf(`Unexpected command %q, expected "export", "publish", or "diff" command`, args[0])
	}

	if err != nil {
		log.Fatalf("ERROR: %s", stacktrace.RootCause(err))
	}
	os.Exit(exitCode)
}
