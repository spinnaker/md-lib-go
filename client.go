package mdlib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/palantir/stacktrace"
)

var (
	DefaultSpinnakerAPIBaseURL = os.Getenv("SPINNAKER_API_BASE_URL")
)

type Client struct {
	SpinnakerAPIBaseURL string
	HTTPGetter          func(string) (*http.Response, error)
}

type ClientOpt func(*Client)

func NewClient(opts ...ClientOpt) *Client {
	c := &Client{
		SpinnakerAPIBaseURL: DefaultSpinnakerAPIBaseURL,
		HTTPGetter:          defaultHTTPGetter,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithBaseURL(baseURL string) ClientOpt {
	return func(c *Client) {
		c.SpinnakerAPIBaseURL = baseURL
	}
}

func WithHTTPGetter(getter func(string) (*http.Response, error)) ClientOpt {
	return func(c *Client) {
		c.HTTPGetter = getter
	}
}

func defaultHTTPGetter(u string) (*http.Response, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to create http resquest from %q", u)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp, stacktrace.Propagate(err, "failed to GET %q", u)
	}
	return resp, nil
}

func commonParsedGet(cli *Client, u string, result interface{}) error {
	content, err := commonGet(cli, u)
	if err != nil {
		return stacktrace.Propagate(err, "failed to get content for %s", u)
	}

	err = json.Unmarshal(content, result)
	if err != nil {
		return stacktrace.Propagate(err, "expected JSON from %s, failed to parse %q as JSON", u, string(content))
	}

	return nil
}

func commonGet(cli *Client, u string) ([]byte, error) {
	if cli.SpinnakerAPIBaseURL == "" {
		return nil, stacktrace.NewError("SPINNAKER_API_BASE_URL environment variable not set")
	}
	u = fmt.Sprintf("%s%s", cli.SpinnakerAPIBaseURL, u)

	uri, err := url.Parse(u)
	if err != nil {
		return nil, stacktrace.Propagate(err, "unable to parse uri: %q", u)
	}

	resp, err := cli.HTTPGetter(uri.String())
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to GET %s", u)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, stacktrace.NewError("Unexpected response from %s, expected 200 but got %d", u, resp.StatusCode)
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to read response body from %s", u)
	}
	return content, nil
}
