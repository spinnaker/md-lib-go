package main

import (
	"fmt"
	"log"
	"sort"
	"syscall"

	"github.com/AlecAivazis/survey/v2"
	mdlib "github.com/spinnaker/md-lib-go"
	"golang.org/x/crypto/ssh/terminal"
)

type exportOptions struct {
	all     bool
	envName string
}

func exportCmd(opts *options, exportOpts *exportOptions) error {
	cli := mdlib.NewClient(
		mdlib.WithBaseURL(opts.baseURL),
	)

	log.Printf("Loading spinnaker resources for %s", opts.appName)

	appData, err := mdlib.FindApplicationResources(cli, opts.appName)
	if err != nil {
		return err
	}

	exportable := mdlib.ExportableApplicationResources(appData)
	artifacts := mdlib.ReferencedArtifacts(appData)

	if len(exportable) == 0 {
		log.Printf("Found no resources to export for Spinnaker app %q", opts.appName)
		return nil
	}

	mdProcessor := mdlib.NewDeliveryConfigProcessor(
		mdlib.WithDirectory(opts.configDir),
		mdlib.WithFile(opts.configFile),
		mdlib.WithAppName(opts.appName),
		mdlib.WithServiceAccount(opts.serviceAccount),
	)

	err = mdProcessor.Load()
	if err != nil {
		return err
	}

	environments := mdProcessor.AllEnvironments()

	sort.Sort(mdlib.ResourceSorter(exportable))

	selected := []int{}
	if exportOpts.all {
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

		_, h, err := terminal.GetSize(syscall.Stdout)
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
		)

		if err != nil {
			return err
		}
	}

	selectedEnvironments := map[string]string{}
	for _, selection := range selected {
		resource := exportable[selection]
		log.Printf("Exporting %s", resource)
		content, err := mdlib.ExportResource(cli, resource, opts.serviceAccount)
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
					Default: selectedEnvironment,
				},
				&selectedEnvironment,
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
