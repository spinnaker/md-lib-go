package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
)

type options struct {
	appName        string
	serviceAccount string
	configDir      string
	configFile     string
	baseURL        string
}

type exitCode int

func (e exitCode) Error() string { return "" }

func main() {
	opts := options{}
	flag.StringVar(&opts.appName, "app", "", "spinnaker application name")
	flag.StringVar(&opts.serviceAccount, "sevice-account", "", "spinnaker service account")
	flag.StringVar(&opts.configDir, "dir", mdlib.DefaultDeliveryConfigDirName, "directory for delivery config file")
	flag.StringVar(&opts.configFile, "file", mdlib.DefaultDeliveryConfigFileName, "delivery config file name")
	flag.StringVar(&opts.baseURL, "baseurl", "", "base URL to reach spinnaker api")

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
		exportOpts := exportOptions{}
		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		exportFlags.BoolVar(&exportOpts.all, "all", false, "export all options, skip prompt")
		exportFlags.StringVar(&exportOpts.envName, "env", "", "assing exported resources to given environment, skip prompt")
		exportFlags.Parse(args[1:])
		err = exportCmd(&opts, &exportOpts)
	case "publish":
		err = publishCmd(&opts)
	case "diff":
		err = diffCmd(&opts)
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
