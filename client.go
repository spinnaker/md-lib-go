package mdlib

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/palantir/stacktrace"
)

var (
	//DefaultSpinnakerAPIBaseURL is the base url to be used for app spinnaker api calls. It can be set in code
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
		httpClient:          defaultHTTPClient,
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

func defaultHTTPClient(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp, stacktrace.Propagate(err, "failed to handle %q", req.URL.String())
	}
	return resp, nil
}

func commonParsedGet(cli *Client, u string, result interface{}) error {
	content, err := commonRequest(cli, "GET", u, nil)
	if err != nil {
		return stacktrace.Propagate(err, "failed to get content for %s", u)
	}

	err = json.Unmarshal(content, result)
	if err != nil {
		return stacktrace.Propagate(err, "expected JSON from %s, failed to parse %q as JSON", u, string(content))
	}

	return nil
}

func commonRequest(cli *Client, method string, u string, body io.Reader) ([]byte, error) {
	if cli.spinnakerAPIBaseURL == "" {
		return nil, stacktrace.NewError("SPINNAKER_API_BASE_URL environment variable not set")
	}
	u = fmt.Sprintf("%s%s", cli.spinnakerAPIBaseURL, u)

	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to create new request for %s", u)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := cli.httpClient(req)
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to %s %s", method, u)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return nil, stacktrace.NewError("Unexpected response from %s, expected 200 or 201 but got %d", u, resp.StatusCode)
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to read response body from %s", u)
	}
	return content, nil
}
