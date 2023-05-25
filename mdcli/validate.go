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
		if valErr != nil {
			for i := 0; i < len(valErr); i++ {
				opts.Logger.Errorf("%s\nReason: %s", valErr[i].Message)
			}
			return 1, nil
		}
		return 1, err
	}

	opts.Logger.Noticef("PASSED")
	return 0, nil
}
