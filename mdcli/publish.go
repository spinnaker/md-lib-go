package mdcli

import (
	"fmt"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// Publish is a command line interface for publishing a local delivery conifg
// to be managed by Spinnaker.
func Publish(opts *CommandOptions) error {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	if _, err := os.Stat(configPath); err != nil {
		return err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
		mdlib.WithHTTPClient(opts.HTTPClient),
	)

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
	)

	err := mdProcessor.Publish(cli)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.Stdout, "OK\n")
	return nil
}
