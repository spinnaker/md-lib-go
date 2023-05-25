package mdcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgutz/ansi"
	mdlib "github.com/spinnaker/md-lib-go"
)

// Plan returns actuation plan for a local delivery config
func Plan(opts *CommandOptions) (int, error) {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		return 1, err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
		mdlib.WithHTTPClient(opts.HTTPClient),
	)

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
		mdlib.WithLogger(opts.Logger),
	)

	plan, err := mdProcessor.Plan(cli)
	if err != nil {
		return 1, err
	}
	if plan.Errors != nil && len(plan.Errors) > 0 {
		opts.Logger.Errorf("Failed to calculate actuation plan due to the following errors:")
		for _, err := range plan.Errors {
			opts.Logger.Errorf(err)
		}
		return 1, nil
	}

	// var output strings.Builder

	for _, environment := range plan.EnvironmentPlans {
		fmt.Fprintf(opts.Stdout, "Environment: %v\n---\n", environment.Environment)
		for _, resource := range environment.ResourcePlans {
			// Print in bold
			fmt.Fprintf(opts.Stdout, "%sResource: %v%s\n", ansi.ColorCode("default+hb"), resource.ResourceDisplayName, ansi.Reset)
			switch {
			case resource.IsPaused:
				fmt.Fprintf(opts.Stdout, "Resource is paused and no actions will be taken\n\n")
			case resource.Action == "NONE":
				fmt.Fprintf(opts.Stdout, "No changes\n\n")
			default:
				for key, diff := range resource.Diff {
					// Do switch case on diff.Type to determine what to do ADDED,CHANGED,REMOVED,
					switch diff.Type {
					case "ADDED":
						fmt.Fprintf(opts.Stdout, "%s+ %s\t\t%v%s\n", ansi.Green, key, diff.Desired, ansi.Reset)
					case "CHANGED":
						fmt.Fprintf(opts.Stdout, "%s~ %s\t\t%v => %v%s\n", ansi.Yellow, key, diff.Current, diff.Desired, ansi.Reset)
					case "REMOVED":
						fmt.Fprintf(opts.Stdout, "%s- %s\t\t%v%s\n", ansi.Red, key, diff.Current, ansi.Reset)
					default:
						// Should never get here
						fmt.Fprintf(opts.Stdout, "%v\tDesired: %v is %v\n", key, diff.Desired, diff.Type)
					}
				}
				fmt.Fprintf(opts.Stdout, "\n")
			}
		}
	}

	return 0, nil
}
