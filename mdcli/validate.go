package mdcli

import (
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// Validate is a command line interface for validating a local delivery conifg.
func Validate(opts *CommandOptions) (int, error) {
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

	valErr, err := mdProcessor.Validate(cli)
	if err != nil {
		opts.Logger.Errorf("Could not validate the configuration: %s\n", err)
		opts.Logger.Errorf("Exiting without failing\n")
		return 0, nil
	}
	if valErr != nil {
		exitWithFailure := false
		opts.Logger.Errorf("Found the following validation issues:\n")
		for i := 0; i < len(valErr); i++ {
			if valErr[i].Status == 1 { // only fail if there is a sev 1 issue
				exitWithFailure = true
			}
			opts.Logger.Errorf("%s\nReason: %s", valErr[i].Message)
		}
		if exitWithFailure {
			opts.Logger.Noticef("FAILED")
			return 1, nil
		}
	}

	opts.Logger.Noticef("PASSED")
	return 0, nil
}
