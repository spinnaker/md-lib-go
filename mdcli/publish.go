package mdcli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
)

type PublishError struct {
	Timestamp int64
	Status    int64
	Error     string
	Message   string
	Body      PublishErrorBody
	URL       string
}

type PublishErrorBody struct {
	Message   string
	Timestamp string
	Status    int64
	Error     string
}

// type wrapper to prevent recursive unmarshalling with our custom
// UnmarshalJSON implementation
type jsonPublishErrorBody PublishErrorBody

func (body *PublishErrorBody) UnmarshalJSON(b []byte) error {
	// The body will be escaped JSON so first we read the string
	// out, then attempt to parse the string value as json
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), (*jsonPublishErrorBody)(body))
}

// Publish is a command line interface for publishing a local delivery conifg
// to be managed by Spinnaker.
func Publish(opts *CommandOptions) (int, error) {
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
	)

	err := mdProcessor.Publish(cli)
	if err != nil {
		if e, ok := stacktrace.RootCause(err).(mdlib.ErrorUnexpectedResponse); ok {
			pe := &PublishError{}
			e.Parse(pe)
			fmt.Fprintf(opts.Stderr, "ERROR: Failed to publish delivery config.  Spinnaker responded with:\n")
			fmt.Fprintf(opts.Stderr, "ERROR: %s\n", pe.Body.Message)
			return 1, nil
		}
		return 1, err
	}

	fmt.Fprintf(opts.Stdout, "OK\n")
	return 0, nil
}
