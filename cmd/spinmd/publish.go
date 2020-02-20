package main

import (
	"log"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

func publishCmd(opts *options) error {
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

	err := mdProcessor.Publish(cli)
	if err != nil {
		return err
	}

	log.Println("OK")
	return nil
}
