package mdcli

import (
	"fmt"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// Pause is a command line interface to pause the management of a Spinnaker application.
func Pause(opts *CommandOptions, appName string) error {
	return resumePause(opts, appName, true)
}

// Resume is a command line interface to resume the paused management of a Spinnaker application.
func Resume(opts *CommandOptions, appName string) error {
	return resumePause(opts, appName, false)
}

func resumePause(opts *CommandOptions, appName string, pause bool) error {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	_, err := os.Stat(configPath)
	if err != nil {
		return err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
		mdlib.WithHTTPClient(opts.HTTPClient),
	)

	if pause {
		err = mdlib.PauseManagement(cli, appName)
	} else {
		err = mdlib.ResumeManagement(cli, appName)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.Stdout, "OK")
	return nil
}
