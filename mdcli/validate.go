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
			opts.Logger.Errorf("%s\nReason: %s", valErr.Error, valErr.Message)
			return 1, nil
		}
		return 1, err
	}

	opts.Logger.Noticef("PASSED")
	return 0, nil
}
