package mdcli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	mdlib "github.com/spinnaker/md-lib-go"
)

// PublishError is the format of an error message upon publishing a delivery
// config.
type PublishError struct {
	Timestamp int64
	Status    int64
	Error     string
	Message   string
	Body      PublishErrorBody
	URL       string
}

// PublishErrorBody is the embedded error message that will be sent upon
// publishing a delivery config.
type PublishErrorBody struct {
	Message   string
	Timestamp string
	Status    int64
	Error     string
}

// type wrapper to prevent recursive unmarshalling with our custom
// UnmarshalJSON implementation
type jsonPublishErrorBody PublishErrorBody

// UnmarshalJSON satisfies the json.Unmarshaller
func (body *PublishErrorBody) UnmarshalJSON(b []byte) error {
	// The body will be escaped JSON so first we read the string
	// out, then attempt to parse the string value as json
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	return json.Unmarshal([]byte(s), (*jsonPublishErrorBody)(body))
}

// Publish is a command line interface for publishing a local delivery config
// to be managed by Spinnaker.
func Publish(opts *CommandOptions, force bool) (int, error) {
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

	err := mdProcessor.Publish(cli, force)
	if err != nil {
		var e mdlib.ErrorUnexpectedResponse
		if errors.As(err, &e) {
			pe := &PublishError{}
			e.Parse(pe)
			opts.Logger.Errorf("Failed to publish delivery config.  Spinnaker responded with:")
			opts.Logger.Errorf(pe.Body.Message)
			return 1, nil
		}
		return 1, err
	}

	opts.Logger.Noticef("OK")
	return 0, nil
}
