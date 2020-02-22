package spincmds

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublish(t *testing.T) {
	requests := map[string]int{}
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				requests[fmt.Sprintf("%s %s", r.Method, r.URL.String())]++
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer ts.Close()

	opts := NewCommandOptions()
	opts.AppName = "myapp"
	opts.ServiceAccount = "myteam@example.com"
	opts.BaseURL = ts.URL
	opts.ConfigDir = "../test-files/publish"
	opts.ConfigFile = "spinnaker.yml"

	err := Publish(opts)
	require.NoError(t, err)

	// we expect a single POST to delivery-configs API
	require.Equal(t, map[string]int{
		"POST /managed/delivery-configs": 1,
	}, requests)
}
