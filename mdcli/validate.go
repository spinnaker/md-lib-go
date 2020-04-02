package mdcli

import (
	"fmt"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// Validate is a command line interface for validating a local delivery conifg.
func Validate(opts *CommandOptions) (int, error) {
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

	valErr, err := mdProcessor.Validate(cli)
	if err != nil {
		return 0, err
	}
	if valErr == nil {
		fmt.Fprintf(opts.Stdout, "%s: OK\n", configPath)
		return 0, nil
	}

	fmt.Fprintf(opts.Stderr,
		"%s: [%s] %s at %s\n",
		configPath,
		valErr.Error,
		valErr.Message,
		valErr.PathExpression,
	)

	return 1, nil
}
