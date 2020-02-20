package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiffCmd(t *testing.T) {
	requests := map[string]int{}
	ts := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				requests[fmt.Sprintf("%s %s", r.Method, r.URL.String())] += 1
				responsePath := fmt.Sprintf("../../test-files/diff/responses%s/%s.json", r.URL.Path, r.Method)
				w.Header().Set("Content-Type", "application/json")
				if _, err := os.Stat(responsePath); err != nil {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fh, err := os.Open(responsePath)
				require.NoError(t, err)
				defer fh.Close()
				w.WriteHeader(http.StatusOK)
				io.Copy(w, fh)
			},
		),
	)
	defer ts.Close()

	opts := options{
		appName:        "myapp",
		serviceAccount: "myteam@example.com",
		baseURL:        ts.URL,
		configDir:      "../../test-files/diff",
		configFile:     "spinnaker.yml",
	}

	err := diffCmd(&opts)
	// we should get an exitCode(1) back since there are diffs for this test
	expectedError := exitCode(1)
	require.Equal(t, &expectedError, err)

	// we expect a single POST to delivery-configs/diff diff API
	require.Equal(t, map[string]int{
		"POST /managed/delivery-configs/diff": 1,
	}, requests)
}
