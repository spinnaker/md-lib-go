package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mgutz/ansi"
	mdlib "github.com/spinnaker/md-lib-go"
)

func diffCmd(opts *options) error {
	configPath := filepath.Join(opts.configDir, opts.configFile)
	if _, err := os.Stat(configPath); err != nil {
		return err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.baseURL),
	)

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.configDir),
		mdlib.WithFile(opts.configFile),
		mdlib.WithAppName(opts.appName),
		mdlib.WithServiceAccount(opts.serviceAccount),
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
		fmt.Printf("=> %s %s\n", status, diff.ResourceID)
		if len(diff.Diffs) > 0 {
			records := []string{}
			for name := range diff.Diffs {
				records = append(records, name)
			}
			sort.Strings(records)
			for _, name := range records {
				if diff.Diffs[name].Current != "" {
					fmt.Printf("%s%s%s\n", ansi.ColorCode("yellow"), name, ansi.ColorCode("reset"))
					fmt.Printf("%s--- current%s\n", ansi.ColorCode("yellow"), ansi.ColorCode("reset"))
					fmt.Printf("%s+++ desired%s\n", ansi.ColorCode("yellow"), ansi.ColorCode("reset"))
					fmt.Printf("%s- %s%s\n", ansi.ColorCode("red"), diff.Diffs[name].Current, ansi.ColorCode("reset"))
					fmt.Printf("%s+ %s%s\n", ansi.ColorCode("green"), diff.Diffs[name].Desired, ansi.ColorCode("reset"))
				}
			}
		}
	}
	return &exit
}
