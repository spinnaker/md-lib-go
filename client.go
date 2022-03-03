package mdlib

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/xerrors"
)

var (
	// DefaultSpinnakerAPIBaseURL is the base url to be used for app spinnaker api calls. It can be set in code
	// or overridden with the SPINNAKER_API_BASE_URL environment variable.
	DefaultSpinnakerAPIBaseURL = os.Getenv("SPINNAKER_API_BASE_URL")
)

// Client holds details for connecting to the spinnaker REST API.
type Client struct {
	spinnakerAPIBaseURL string
	httpClient          func(*http.Request) (*http.Response, error)
}

// ClientOpt is an interface for variadic options when constructing a Client via NewClient
type ClientOpt func(*Client)

// NewClient constructs a Client and applies any provided ClientOpt
func NewClient(opts ...ClientOpt) *Client {
	c := &Client{
		spinnakerAPIBaseURL: DefaultSpinnakerAPIBaseURL,
		httpClient:          http.DefaultClient.Do,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithBaseURL is a ClientOpt to set the spinnakerAPIBaseURL via NewClient
func WithBaseURL(baseURL string) ClientOpt {
	return func(c *Client) {
		c.spinnakerAPIBaseURL = baseURL
	}
}

// WithHTTPClient is a ClientOpt to set the httpClient via NewClient
func WithHTTPClient(client func(*http.Request) (*http.Response, error)) ClientOpt {
	return func(c *Client) {
		c.httpClient = client
	}
}

func commonParsedGet(cli *Client, u string, result interface{}) error {
	content, err := commonRequest(cli, "GET", u, requestBody{})
	if err != nil {
		return xerrors.Errorf("failed to get content for %s: %w", u, err)
	}

	err = json.Unmarshal(content, result)
	if err != nil {
		return xerrors.Errorf(
			"expected JSON from %s, failed to parse %q as JSON: %w", u, string(content),
			ErrorInvalidContent{Content: content, ParseError: err},
		)
	}

	return nil
}

type requestBody struct {
	Content     io.Reader
	ContentType string
}

func commonRequest(cli *Client, method string, u string, body requestBody) ([]byte, error) {
	if cli.spinnakerAPIBaseURL == "" {
		return nil, xerrors.New("SPINNAKER_API_BASE_URL environment variable not set")
	}
	u = cli.spinnakerAPIBaseURL + u

	req, err := http.NewRequest(method, u, body.Content)
	if err != nil {
		return nil, xerrors.Errorf("unable to create new request for %s: %w", u, err)
	}

	req.Header.Set("Accept", "application/json")
	if body.ContentType != "" {
		req.Header.Set("Content-Type", body.ContentType)
	}

	resp, err := cli.httpClient(req)
	if err != nil {
		return nil, xerrors.Errorf("failed to %s %s: %w", method, u, err)
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.Errorf("failed to read response body from %s: %w", u, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err := ErrorUnexpectedResponse{
			StatusCode: resp.StatusCode,
			URL:        u,
			Content:    content,
		}
		return nil, xerrors.Errorf("api request: %w", err)
	}

	return content, nil
}
