package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishCmd(t *testing.T) {
	requests := map[string]int{}
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				requests[fmt.Sprintf("%s %s", r.Method, r.URL.String())] += 1
				w.WriteHeader(http.StatusOK)
			},
		),
	)
	defer ts.Close()

	opts := options{
		appName:        "myapp",
		serviceAccount: "myteam@example.com",
		baseURL:        ts.URL,
		configDir:      "../../test-files/publish",
		configFile:     "spinnaker.yml",
	}

	err := publishCmd(&opts)
	require.NoError(t, err)

	// we expect a single POST to delivery-configs API
	require.Equal(t, map[string]int{
		"POST /managed/delivery-configs": 1,
	}, requests)
}
