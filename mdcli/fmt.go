package mdcli

import (
	"fmt"

	mdlib "github.com/spinnaker/md-lib-go"
)

// Format is a command line interface to format the delivery config file.
func Format(opts *CommandOptions) error {
	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
	)

	err := mdProcessor.Load()
	if err != nil {
		return err
	}

	err = mdProcessor.Save()
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.Stdout, "OK\n")
	return nil
}
