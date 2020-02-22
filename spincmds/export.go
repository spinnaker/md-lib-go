package spincmds

import (
	"fmt"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	mdlib "github.com/spinnaker/md-lib-go"
	"golang.org/x/crypto/ssh/terminal"
)

// ExportOptions are options specifically for the Export Command.
type ExportOptions struct {
	CommandOptions
	All     bool
	EnvName string
}

// Export is a command line interface to discover exportable Spinnaker resources and then
// optional add those resources to a local delivery config file to be later managed by Spinnaker.
func Export(opts *ExportOptions) error {
	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.BaseURL),
	)

	opts.Logger.Printf("Loading spinnaker resources for %s", opts.AppName)

	appData, err := mdlib.FindApplicationResources(cli, opts.AppName)
	if err != nil {
		return err
	}

	exportable := mdlib.ExportableApplicationResources(appData)
	artifacts := mdlib.ReferencedArtifacts(appData)

	if len(exportable) == 0 {
		opts.Logger.Printf("Found no resources to export for Spinnaker app %q", opts.AppName)
		return nil
	}

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.ConfigDir),
		mdlib.WithFile(opts.ConfigFile),
		mdlib.WithAppName(opts.AppName),
		mdlib.WithServiceAccount(opts.ServiceAccount),
	)

	err = mdProcessor.Load()
	if err != nil {
		return err
	}

	environments := mdProcessor.AllEnvironments()

	sort.Sort(mdlib.ResourceSorter(exportable))

	selected := []int{}
	if opts.All {
		for i := range exportable {
			selected = append(selected, i)
		}
	} else {
		options := []string{}
		defaults := []string{}
		for _, resource := range exportable {
			var option string
			if mdProcessor.ResourceExists(resource) {
				option = fmt.Sprintf("Update %s", resource)
			} else {
				option = fmt.Sprintf("Export %s", resource)
				defaults = append(defaults, option)
			}
			options = append(options, option)
		}

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

	selectedEnvironments := map[string]string{}
	for _, selection := range selected {
		resource := exportable[selection]
		opts.Logger.Printf("Exporting %s", resource)
		content, err := mdlib.ExportResource(cli, resource, opts.ServiceAccount)
		if err != nil {
			return err
		}

		envName := opts.EnvName
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
					Default: selectedEnvironment,
				},
				&selectedEnvironment,
				survey.WithStdio(opts.Stdin, opts.Stdout, opts.Stderr),
			)
			if err != nil {
				return err
			}
			envName = selectedEnvironment
		}

		mdProcessor.UpsertResource(resource, envName, content)
	}

	sort.Sort(mdlib.ArtifactSorter(artifacts))
	for _, artifact := range artifacts {
		mdProcessor.InsertArtifact(artifact)
	}

	err = mdProcessor.Save()
	if err != nil {
		return err
	}
	return nil
}
