package mdcli

import (
	mdlib "github.com/spinnaker/md-lib-go"
)

// Format is a command line interface to format the delivery config file.
func Format(opts *CommandOptions) error {
	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
		mdlib.WithLogger(opts.Logger),
	)

	err := mdProcessor.Load()
	if err != nil {
		return err
	}

	err = mdProcessor.Save()
	if err != nil {
		return err
	}

	opts.Logger.Noticef("OK")
	return nil
}
