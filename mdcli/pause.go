package mdcli

import (
	"fmt"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// ResumePause is a command line interface for resuming or pausing the management
// of the provide application
func ResumePause(opts *CommandOptions, appName string, pause bool) error {
	configPath := filepath.Join(opts.ConfigDir, opts.ConfigFile)
	_, err := os.Stat(configPath)
	if err != nil {
		return err
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
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
