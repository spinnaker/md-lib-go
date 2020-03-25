package mdcli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/mgutz/ansi"
	mdlib "github.com/spinnaker/md-lib-go"
)

//DiffOptions allows for optional flags to the Diff command.
type DiffOptions struct {
	// Brief will only print resources names and indicate the diff status
	Brief bool
	// Quiet exit without output, exit code will determine if there are diffs
	Quiet bool
}

// Diff is a command line interface to display differences between a delivery config on disk
// with what is actively deployed.
func Diff(opts *CommandOptions, diffOpts DiffOptions) (int, error) {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		return 0, err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
		mdlib.WithHTTPClient(opts.HTTPClient),
	)

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
	)

	diffs, err := mdProcessor.Diff(cli)
	if err != nil {
		return 0, err
	}

	var exit int
	for _, diff := range diffs {
		status := ansi.Color(diff.Status, "yellow")
		if diff.Status == "NO_DIFF" {
			status = ansi.Color("SAME", "green")
		} else {
			exit = 1
		}
		if diffOpts.Quiet {
			continue
		}
		fmt.Fprintf(opts.Stdout, "=> %s %s\n", status, diff.ResourceID)
		if diffOpts.Brief {
			continue
		}
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
	return exit, nil
}
