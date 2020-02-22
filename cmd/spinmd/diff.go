package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mgutz/ansi"
	mdlib "github.com/spinnaker/md-lib-go"
)

// DiffCmd is a command line interface to display differences between a delivery config on disk
// with what is actively deployed.
func DiffCmd(opts *CommandOptions) error {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		return err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
	)

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
		mdlib.WithAppName(opts.AppName),
		mdlib.WithServiceAccount(opts.ServiceAccount),
	)

	diffs, err := mdProcessor.Diff(cli)
	if err != nil {
		return err
	}

	var exit exitCode
	for _, diff := range diffs {
		status := ansi.Color(diff.Status, "yellow")
		if diff.Status == "NO_DIFF" {
			status = ansi.Color("SAME", "green")
		} else {
			exit = 1
		}
		fmt.Fprintf(opts.Stdout, "=> %s %s\n", status, diff.ResourceID)
		if len(diff.Diffs) > 0 {
			records := []string{}
			for name := range diff.Diffs {
				records = append(records, name)
			}
			sort.Strings(records)
			for _, name := range records {
				if diff.Diffs[name].Current != "" {
					fmt.Fprintf(opts.Stdout, "%s%s%s\n", ansi.ColorCode("yellow"), name, ansi.ColorCode("reset"))
					fmt.Fprintf(opts.Stdout, "%s--- current%s\n", ansi.ColorCode("yellow"), ansi.ColorCode("reset"))
					fmt.Fprintf(opts.Stdout, "%s+++ desired%s\n", ansi.ColorCode("yellow"), ansi.ColorCode("reset"))
					fmt.Fprintf(opts.Stdout, "%s- %s%s\n", ansi.ColorCode("red"), diff.Diffs[name].Current, ansi.ColorCode("reset"))
					fmt.Fprintf(opts.Stdout, "%s+ %s%s\n", ansi.ColorCode("green"), diff.Diffs[name].Desired, ansi.ColorCode("reset"))
				}
			}
		}
	}
	return &exit
}
