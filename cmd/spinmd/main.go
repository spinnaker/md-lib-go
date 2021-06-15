package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"

	"github.com/palantir/stacktrace"
	mdlib "github.com/spinnaker/md-lib-go"
	"github.com/spinnaker/md-lib-go/mdcli"
	"github.com/spinnaker/spin/cmd/gateclient"
	"github.com/spinnaker/spin/config"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/util/homedir"
)

func main() {
	opts := mdcli.NewCommandOptions()

	cfg := config.Config{}

	configFile := filepath.Join(homedir.HomeDir(), ".spin", "config")
	if _, err := os.Stat(configFile); err == nil {
		yamlFile, err := ioutil.ReadFile(configFile)
		if err != nil {
			log.Fatalf("Unable to read %s: %s", configFile, err)
		}
		yamlIn := bytes.NewReader([]byte(os.ExpandEnv(string(yamlFile))))
		dec := yaml.NewDecoder(yamlIn)
		dec.KnownFields(true)
		err = dec.Decode(&cfg)
		if err != nil {
			log.Fatalf("Failed to parse %s as YAML: %s", configFile, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Unable to stat %s: %s", configFile, err)
	}

	httpClient, err := gateclient.InitializeHTTPClient(cfg.Auth)
	if err != nil {
		log.Fatalf("Failed to create client: %s", err)
	}

	ctx, err := gateclient.ContextWithAuth(context.Background(), cfg.Auth)
	if err != nil {
		log.Fatalf("Failed to extract valid login credentials from %s: %s", configFile, err)
	}

	output := func(msg string) {
		fmt.Println(msg)
	}

	updatedConfig, err := gateclient.Authenticate(output, httpClient, cfg.Gate.Endpoint, cfg.Auth)
	if err != nil {
		log.Fatalf("Failed to authenticate with Spinnaker: %s", err)
	}

	if updatedConfig {
		// config updated with credential information, so write it back out
		fd, err := os.Create(configFile)
		if err != nil {
			log.Fatalf("Failed to open %q: %s", configFile, err)
		}
		content, err := yaml.Marshal(cfg)
		if err != nil {
			log.Fatalf("Failed to write updated config file %q: %s", configFile, err)
		}
		_, err = fd.Write(content)
		if err != nil {
			log.Fatalf("Failed to write to configFile %q: %s", configFile, err)
		}
		err = fd.Close()
		if err != nil {
			log.Fatalf("'Failed to close configFile %q after writing: %s", configFile, err)
		}
	}

	opts.HTTPClient = func(req *http.Request) (*http.Response, error) {
		gateclient.AddAuthHeaders(ctx, req)
		return httpClient.Do(req)
	}

	verbose := false
	globalFlags := flag.NewFlagSet("", flag.ContinueOnError)

	globalFlags.StringVar(&opts.ConfigDir, "dir", mdlib.DefaultDeliveryConfigDirName, "directory for delivery config file")
	globalFlags.StringVar(&opts.ConfigFile, "file", mdlib.DefaultDeliveryConfigFileName, "delivery config file name")
	globalFlags.StringVar(&opts.BaseURL, "baseurl", cfg.Gate.Endpoint, "base URL to reach spinnaker api")
	globalFlags.BoolVar(&verbose, "v", false, "verbose logging for rest api requests")
	globalFlags.Parse(os.Args[1:])
	args := globalFlags.Args()

	if len(args) < 1 {
		fmt.Printf("Usage: %s [flags] export|publish|diff|pause|resume|delete|validate|fmt\n", filepath.Base(os.Args[0]))
		fmt.Printf("Flags:\n")
		globalFlags.PrintDefaults()
		return
	}

	if verbose {
		doer := opts.HTTPClient
		opts.HTTPClient = func(req *http.Request) (resp *http.Response, err error) {
			out, _ := httputil.DumpRequest(req, true)
			log.Printf(string(out))
			defer func() {
				out, _ := httputil.DumpResponse(resp, true)
				if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
					out, _ = ioutil.ReadAll(resp.Body)
					var data interface{}
					json.Unmarshal(out, &data)
					buf := bytes.NewBuffer(out)
					resp.Body = ioutil.NopCloser(buf)
					out, _ = json.MarshalIndent(data, "", "  ")
				}
				log.Printf(string(out))
			}()
			return doer(req)
		}
	}

	exitCode := 0
	switch args[0] {
	case "export":
		var appName, serviceAccount string
		exportAll := false
		envName := ""

		exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
		exportFlags.StringVar(&appName, "app", "", "spinnaker application name")
		exportFlags.StringVar(&serviceAccount, "service-account", "", "spinnaker service account")
		exportFlags.BoolVar(&exportAll, "all", false, "export all options, skip prompt")
		exportFlags.StringVar(&envName, "env", "", "assign exported resources to given environment, skip prompt")
		exportFlags.Parse(args[1:])

		if exportFlags.NArg() > 0 || appName == "" || serviceAccount == "" {
			fmt.Printf("Usage: export -app <name> -service-account <account>\n")
			fmt.Printf("Flags:\n")
			exportFlags.Usage()
			return
		}
		exitCode, err = mdcli.Export(
			opts,
			appName,
			serviceAccount,
			mdcli.ExportAll(exportAll),
			mdcli.AssumeEnvName(envName),
		)
	case "publish":
		exitCode, err = mdcli.Publish(opts)
	case "validate":
		exitCode, err = mdcli.Validate(opts)
	case "diff":
		var quiet, brief bool
		diffFlags := flag.NewFlagSet("diff", flag.ExitOnError)
		diffFlags.BoolVar(&quiet, "quiet", false, "suppress output, exit code will indicate differences")
		diffFlags.BoolVar(&brief, "brief", false, "only print resources status, do not print differences")
		diffFlags.Parse(args[1:])

		if diffFlags.NArg() > 0 {
			fmt.Printf("Usage: diff\n")
			fmt.Printf("Flags:\n")
			diffFlags.Usage()
			return
		}
		exitCode, err = mdcli.Diff(opts, mdcli.DiffOptions{
			Brief: brief,
			Quiet: quiet,
		})
	case "pause":
		var appName string
		pauseFlags := flag.NewFlagSet("pause", flag.ExitOnError)
		pauseFlags.StringVar(&appName, "app", "", "spinnaker application name")
		pauseFlags.Parse(args[1:])

		if pauseFlags.NArg() > 0 || appName == "" {
			fmt.Printf("Usage: pause -app <name>\n")
			fmt.Printf("Flags:\n")
			pauseFlags.Usage()
			return
		}

		err = mdcli.Pause(opts, appName)
	case "resume":
		var appName string
		resumeFlags := flag.NewFlagSet("resume", flag.ExitOnError)
		resumeFlags.StringVar(&appName, "app", "", "spinnaker application name")
		resumeFlags.Parse(args[1:])

		if resumeFlags.NArg() > 0 || appName == "" {
			fmt.Printf("Usage: resume -app <name>\n")
			fmt.Printf("Flags:\n")
			resumeFlags.Usage()
			return
		}

		err = mdcli.Resume(opts, appName)
	case "fmt":
		err = mdcli.Format(
			opts,
		)
	default:
		log.Fatalf(`Unexpected command %q, expected one of export|publish|diff|pause|resume|delete|validate|fmt`, args[0])
	}

	if err != nil {
		log.Fatalf("ERROR: %s", stacktrace.RootCause(err))
	}
	os.Exit(exitCode)
}
