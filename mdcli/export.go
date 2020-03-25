package mdcli

import (
	"fmt"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	mdlib "github.com/spinnaker/md-lib-go"
	"golang.org/x/crypto/ssh/terminal"
)

// exportOptions are options specifically for the Export Command.
type exportOptions struct {
	all                    bool
	envName                string
	onlyAccount            string
	customResourceScanner  func(*mdlib.ApplicationResources) []*mdlib.ExportableResource
	customResourceExporter func(*mdlib.Client, *mdlib.ExportableResource, string) ([]byte, error)
	constraintsProvider    func(envName string, current mdlib.DeliveryConfig) []interface{}
	notificationsProvider  func(envName string, current mdlib.DeliveryConfig) []interface{}
}

// ExportOption is an interface to provide custom overrides for the Export command.
type ExportOption func(o *exportOptions)

// ExportAll is an override to Export, when true Export will not prompt and will
// export all resources found.
func ExportAll(b bool) ExportOption {
	return func(o *exportOptions) {
		o.all = b
	}
}

// AssumeEnvName is an override to Export, if a non-empty string Export will not
// prompt for the environment name when exporting a new resource not already
// found in the delivery config.
func AssumeEnvName(envName string) ExportOption {
	return func(o *exportOptions) {
		o.envName = envName
	}
}

// OnlyAccount is an override to Export, if a non-empty string Export will only
// attempt to export resources that are found in the provided account.
func OnlyAccount(acct string) ExportOption {
	return func(o *exportOptions) {
		o.onlyAccount = acct
	}
}

// CustomResourceScanner is an override to Export that can be used to implement a resource scanner
// if you Spinnaker deployment can manage custom resource types. For example, maybe Spinnaker can
// manage a "bake" resource to automatically generate an AWS AMI.
// The default scanner is mdlib.ExportableApplicationResources
func CustomResourceScanner(f func(*mdlib.ApplicationResources) []*mdlib.ExportableResource) ExportOption {
	return func(o *exportOptions) {
		o.customResourceScanner = f
	}
}

// CustomResourceExporter is an override to Export that can be used to implement a custom resource exporter.
// The default exporter is mdlib.ExportResource
func CustomResourceExporter(f func(*mdlib.Client, *mdlib.ExportableResource, string) ([]byte, error)) ExportOption {
	return func(o *exportOptions) {
		o.customResourceExporter = f
	}
}

// ConstraintsProvider is an override to Export that can be used to customizing how a default
// environment constraint is generated for newly created environments.
func ConstraintsProvider(cp func(envName string, current mdlib.DeliveryConfig) []interface{}) ExportOption {
	return func(o *exportOptions) {
		o.constraintsProvider = cp
	}
}

// NotificationsProvider is an override to Export that can be used to customizing how a default
// environment notification is generated for newly created environments.
func NotificationsProvider(np func(envName string, current mdlib.DeliveryConfig) []interface{}) ExportOption {
	return func(o *exportOptions) {
		o.notificationsProvider = np
	}
}

// Export is a command line interface to discover exportable Spinnaker resources and then
// optional add those resources to a local delivery config file to be later managed by Spinnaker.
func Export(opts *CommandOptions, appName string, serviceAccount string, overrides ...ExportOption) error {
	exportOpts := &exportOptions{
		customResourceScanner:  mdlib.ExportableApplicationResources,
		customResourceExporter: mdlib.ExportResource,
	}
	for _, override := range overrides {
		override(exportOpts)
	}

	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
		mdlib.WithHTTPClient(opts.HTTPClient),
	)

	opts.Logger.Printf("Loading spinnaker resources for %s", appName)

	appData, err := mdlib.FindApplicationResources(cli, appName)
	if err != nil {
		return err
	}

	exportable := exportOpts.customResourceScanner(appData)
	artifacts := mdlib.ReferencedArtifacts(appData)

	if len(exportable) == 0 {
		opts.Logger.Printf("Found no resources to export for Spinnaker app %q", appName)
		return nil
	}

	if exportOpts.onlyAccount != "" {
		filtered := []*mdlib.ExportableResource{}
		for _, resource := range exportable {
			if resource.Account != exportOpts.onlyAccount {
				continue
			}
			filtered = append(filtered, resource)
		}
		exportable = filtered
	}

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
		mdlib.WithAppName(appName),
		mdlib.WithServiceAccount(serviceAccount),
		mdlib.WithConstraintsProvider(exportOpts.constraintsProvider),
		mdlib.WithNotificationsProvider(exportOpts.notificationsProvider),
	)

	err = mdProcessor.Load()
	if err != nil {
		return err
	}

	environments := mdProcessor.AllEnvironments()

	sort.Sort(mdlib.ResourceSorter(exportable))

	options := []string{}
	defaults := []string{}
	optionsIndexByName := map[string]int{}
	for ix, resource := range exportable {
		var option string
		if mdProcessor.ResourceExists(resource) {
			option = fmt.Sprintf("Update %s", resource)
		} else {
			option = fmt.Sprintf("Export %s", resource)
			defaults = append(defaults, option)
		}
		options = append(options, option)
		optionsIndexByName[option] = ix
	}

	selected := []string{}
	if exportOpts.all {
		selected = options
	} else {
		_, h, err := terminal.GetSize(int(opts.Stdout.Fd()))
		if err != nil {
			return err
		}
		pageSize := len(options)
		if pageSize+2 > h {
			pageSize = h - 2
		}

		err = survey.AskOne(
			&survey.MultiSelect{
				Message:  "Select resources to export",
				Options:  options,
				Default:  defaults,
				PageSize: pageSize,
			},
			&selected,
			survey.WithStdio(opts.Stdin, opts.Stdout, opts.Stderr),
		)

		if err != nil {
			return err
		}
	}

	sort.Sort(mdlib.ArtifactSorter(artifacts))
	for _, artifact := range artifacts {
		mdProcessor.InsertArtifact(artifact)
	}

	selectedEnvironments := map[string]string{}
	for _, selection := range selected {
		resource := exportable[optionsIndexByName[selection]]
		opts.Logger.Printf("Exporting %s", resource)
		content, err := exportOpts.customResourceExporter(cli, resource, serviceAccount)
		if err != nil {
			return err
		}

		envName := exportOpts.envName
		if envName == "" {
			// not overridden via options so default to current delivery config env
			envName = mdProcessor.WhichEnvironment(resource)
		}
		if envName == "" {
			// no env for resource, so prompt
			selectedEnvironment := selectedEnvironments[resource.Account]
			err = survey.AskOne(
				&survey.Select{
					Message: fmt.Sprintf("Select environment for %s", resource),
					Options: environments,
					Default: func(d string) interface{} {
						if d != "" {
							return d
						}
						return nil
					}(selectedEnvironment),
				},
				&selectedEnvironment,
				survey.WithStdio(opts.Stdin, opts.Stdout, opts.Stderr),
			)
			if err != nil {
				return err
			}
			envName = selectedEnvironment
			selectedEnvironments[resource.Account] = selectedEnvironment
		}

		mdProcessor.UpsertResource(resource, envName, content)
	}

	err = mdProcessor.Save()
	if err != nil {
		return err
	}
	return nil
}
