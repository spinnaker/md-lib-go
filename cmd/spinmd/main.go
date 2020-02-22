package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
	"github.com/spinnaker/md-lib-go/mdcli"
)

func main() {
	opts := mdcli.NewCommandOptions()

	globalFlags := flag.NewFlagSet("", flag.ContinueOnError)

	globalFlags.StringVar(&opts.ConfigDir, "dir", mdlib.DefaultDeliveryConfigDirName, "directory for delivery config file")
	globalFlags.StringVar(&opts.ConfigFile, "file", mdlib.DefaultDeliveryConfigFileName, "delivery config file name")
	globalFlags.StringVar(&opts.BaseURL, "baseurl", "", "base URL to reach spinnaker api")

	globalFlags.Parse(os.Args[1:])
	args := globalFlags.Args()

	if len(args) < 1 {
		fmt.Printf("Usage: %s [flags] export|publish|diff\n", filepath.Base(os.Args[0]))
		fmt.Printf("Flags:\n")
		globalFlags.PrintDefaults()
		return
	}

	exitCode := 0
	var err error
	switch args[0] {
	case "export":
		var appName, serviceAccount string
		exportAll := false
		envName := ""

		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		exportFlags.StringVar(&appName, "app", "", "spinnaker application name")
		exportFlags.StringVar(&serviceAccount, "service-account", "", "spinnaker service account")
		exportFlags.BoolVar(&exportAll, "all", false, "export all options, skip prompt")
		exportFlags.StringVar(&envName, "env", "", "assign exported resources to given environment, skip prompt")
		exportFlags.Parse(args[1:])

		if exportFlags.NArg() > 0 {
			exportFlags.Usage()
			return
		}
		err = mdcli.Export(
			opts,
			appName,
			serviceAccount,
			mdcli.ExportAll(exportAll),
			mdcli.AssumeEnvName(envName),
		)
	case "publish":
		err = mdcli.Publish(opts)
	case "diff":
		exitCode, err = mdcli.Diff(opts)
	default:
		log.Fatalf(`Unexpected command %q, expected "export", "publish", or "diff" command`, args[0])
	}

	if err != nil {
		log.Fatalf("ERROR: %s", stacktrace.RootCause(err))
	}
	os.Exit(exitCode)
}
