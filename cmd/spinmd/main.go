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
		fmt.Printf("Usage: %s [flags] export|publish|diff|pause|resume|delete\n", filepath.Base(os.Args[0]))
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

		if exportFlags.NArg() > 0 || appName == "" || serviceAccount == "" {
			fmt.Printf("Usage: export -app <name> -service-account <account>\n")
			fmt.Printf("Flags:\n")
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
		var quiet, brief bool
		diffFlags := flag.NewFlagSet("diff", flag.ExitOnError)
		diffFlags.BoolVar(&quiet, "quiet", false, "suppress output, exit code will indicate differences")
		diffFlags.BoolVar(&brief, "brief", false, "only print resources status, do not print differences")
		diffFlags.Parse(args[1:])

		if diffFlags.NArg() > 0 {
			fmt.Printf("Usage: diff\n")
			fmt.Printf("Flags:\n")
			diffFlags.Usage()
			return
		}
		exitCode, err = mdcli.Diff(opts, mdcli.DiffOptions{
			Brief: brief,
			Quiet: quiet,
		})
	case "pause":
		var appName string
		pauseFlags := flag.NewFlagSet("pause", flag.ExitOnError)
		pauseFlags.StringVar(&appName, "app", "", "spinnaker application name")
		pauseFlags.Parse(args[1:])

		if pauseFlags.NArg() > 0 || appName == "" {
			fmt.Printf("Usage: pause -app <name>\n")
			fmt.Printf("Flags:\n")
			pauseFlags.Usage()
			return
		}

		err = mdcli.Pause(opts, appName)
	case "resume":
		var appName string
		resumeFlags := flag.NewFlagSet("resume", flag.ExitOnError)
		resumeFlags.StringVar(&appName, "app", "", "spinnaker application name")
		resumeFlags.Parse(args[1:])

		if resumeFlags.NArg() > 0 || appName == "" {
			fmt.Printf("Usage: resume -app <name>\n")
			fmt.Printf("Flags:\n")
			resumeFlags.Usage()
			return
		}

		err = mdcli.Resume(opts, appName)
	default:
		log.Fatalf(`Unexpected command %q, expected "export", "publish", or "diff" command`, args[0])
	}

	if err != nil {
		log.Fatalf("ERROR: %s", stacktrace.RootCause(err))
	}
	os.Exit(exitCode)
}
