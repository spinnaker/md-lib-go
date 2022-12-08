package mdcli

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/mgutz/ansi"
	mdlib "github.com/spinnaker/md-lib-go"
	"github.com/xlab/treeprint"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"
)

// exportOptions are options specifically for the Export Command.
type exportOptions struct {
	all                    bool
	envName                string
	onlyAccount            string
	clusters               []string
	customResourceScanner  func(*mdlib.ApplicationResources) []*mdlib.ExportableResource
	customResourceExporter func(*mdlib.Client, *mdlib.ExportableResource) ([]byte, error)
	constraintsProvider    func(envName string, current mdlib.DeliveryConfig) []interface{}
	notificationsProvider  func(envName string, current mdlib.DeliveryConfig) []interface{}
	verifyWithProvider     func(envName string, current mdlib.DeliveryConfig) []interface{}
	postDeployProvider     func(envName string, current mdlib.DeliveryConfig) []interface{}
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
func CustomResourceExporter(f func(*mdlib.Client, *mdlib.ExportableResource) ([]byte, error)) ExportOption {
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

// VerifyWithProvider is an override to Export that can be used to customizing how a default
// verifyWith configuration is generated for new and existing environments.
func VerifyWithProvider(vp func(envName string, current mdlib.DeliveryConfig) []interface{}) ExportOption {
	return func(o *exportOptions) {
		o.verifyWithProvider = vp
	}
}

// PostDeployProvider is an override to Export that can be used to customizing a default
// postDeploy configuration is generated for new and existing environments.
func PostDeployProvider(pdp func(envName string, current mdlib.DeliveryConfig) []interface{}) ExportOption {
	return func(o *exportOptions) {
		o.postDeployProvider = pdp
	}
}

// SetEnvironment sets the environment name
func SetEnvironment(envName string) ExportOption {
	return func(o *exportOptions) {
		o.envName = envName
	}
}

// SetClusters sets the clusters to export
func SetClusters(clusters []string) ExportOption {
	return func(o *exportOptions) {
		o.clusters = clusters
	}
}

// Export is a command line interface to discover exportable Spinnaker resources and then
// optional add those resources to a local delivery config file to be later managed by Spinnaker.
func Export(opts *CommandOptions, appName string, overrides ...ExportOption) (int, error) {
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
		return 1, err
	}

	exportable := exportOpts.customResourceScanner(appData)

	if len(exportable) == 0 {
		opts.Logger.Printf("Found no resources to export for Spinnaker app %q", appName)
		return 0, nil
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
		mdlib.WithConstraintsProvider(exportOpts.constraintsProvider),
		mdlib.WithNotificationsProvider(exportOpts.notificationsProvider),
		mdlib.WithVerifyProvider(exportOpts.verifyWithProvider),
		mdlib.WithPostDeployProvider(exportOpts.postDeployProvider),
	)

	err = mdProcessor.Load()
	if err != nil {
		return 1, err
	}

	environments := mdProcessor.AllEnvironments()

	sort.Sort(mdlib.ResourceSorter(exportable))

	options := []string{}
	defaults := []string{}
	optionsIndexByName := map[string]int{}
	for ix, resource := range exportable {
		var option string
		switch {
		case mdProcessor.ResourceExists(resource):
			option = fmt.Sprintf("Export %s", resource)
		case resource.ResourceType == mdlib.NetworkLoadBalancerResourceType:
			opts.Logger.Printf("WARNING cannot export %s", resource)
			continue
		default:
			option = fmt.Sprintf("Export %s", resource)
			defaults = append(defaults, option)
		}
		options = append(options, option)
		optionsIndexByName[option] = ix
	}

	selected := []string{}
	switch {
	case exportOpts.all:
		selected = options
	case exportOpts.clusters != nil:
		for ix, resource := range exportable {
			if resource.ResourceType == "cluster" {
				for _, cluster := range exportOpts.clusters {
					if cluster == resource.Name {
						selected = append(selected, options[ix])
					}
				}
			}
		}
	default:
		_, h, err := terminal.GetSize(int(opts.Stdout.Fd()))
		if err != nil {
			return 1, err
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
			return 1, err
		}
	}

	errors := []error{}

	selectedEnvironments := map[string]string{}
	modifiedResources := map[*mdlib.ExportableResource]bool{}
	addedArtifacts := []*mdlib.DeliveryArtifact{}
	for _, selection := range selected {
		resource := exportable[optionsIndexByName[selection]]
		opts.Logger.Printf("Exporting %s", resource)
		content, err := exportOpts.customResourceExporter(cli, resource)
		if err != nil {
			errors = append(errors, xerrors.Errorf("Failed to export resource %s: %w", resource, err))
			continue
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
				errors = append(errors, xerrors.Errorf("Failed to read prompt for environment on resource %s: %w", resource, err))
				continue
			}
			envName = selectedEnvironment
			selectedEnvironments[resource.Account] = selectedEnvironment
		}

		added, err := mdProcessor.UpsertResource(resource, envName, content)
		if err != nil {
			errors = append(errors, xerrors.Errorf("Failed to upsert delivery config for resource %s: %w", resource, err))
			continue
		}
		modifiedResources[resource] = added

		if resource.ResourceType == mdlib.ClusterResourceType {
			opts.Logger.Printf("Exporting Artifact for %s", resource)
			artifact := &mdlib.DeliveryArtifact{}
			err := mdlib.ExportArtifact(cli, resource, artifact)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			if added, updatedRef := mdProcessor.InsertArtifact(artifact); added || updatedRef != "" {
				if added {
					found := false
					for _, a := range addedArtifacts {
						if a.Equal(artifact) && a.RefName() == artifact.RefName() {
							found = true
							break
						}
					}
					if !found {
						addedArtifacts = append(addedArtifacts, artifact)
					}
				}
				if updatedRef != "" {
					opts.Logger.Printf("WARNING updating artifact reference name for %s due to collision", resource)
					opts.Logger.Printf("WARNING artifact reference changed to %s to prevent collision", updatedRef)
					err := mdProcessor.UpdateArtifactReference(&content, updatedRef)
					if err != nil {
						errors = append(errors, err)
						continue
					}
					added, err = mdProcessor.UpsertResource(resource, envName, content)
					if err != nil {
						errors = append(errors, xerrors.Errorf("Failed to upsert delivery config for resource %s: %w", resource, err))
						continue
					}
					modifiedResources[resource] = added
				}
			}
		}
	}

	err = mdProcessor.Save()
	if err != nil {
		return 1, err
	}

	// reload delivery config so we can print out the tree structure
	delivery := mdlib.DeliveryConfig{}
	contents, err := ioutil.ReadFile(filepath.Join(opts.ConfigDir, opts.ConfigFile))
	if err != nil {
		return 1, err
	}
	err = yaml.Unmarshal(contents, &delivery)
	if err != nil {
		return 1, err
	}

	// start building a tree view of resources
	tree := treeprint.New()
	tree.SetValue(fmt.Sprintf("🦄 %s%s%s", ansi.ColorCode("default+hb"), appName, ansi.Reset))

	// reset the tree outline to be magenta
	treeprint.EdgeTypeLink = treeprint.EdgeType(fmt.Sprintf("%s%s%s", ansi.Magenta, "│", ansi.Reset))
	treeprint.EdgeTypeMid = treeprint.EdgeType(fmt.Sprintf("%s%s%s", ansi.Magenta, "├──", ansi.Reset))
	treeprint.EdgeTypeEnd = treeprint.EdgeType(fmt.Sprintf("%s%s%s", ansi.Magenta, "└──", ansi.Reset))

	artNode := tree.AddBranch(fmt.Sprintf("%s%s%s", ansi.ColorCode("blue+b"), "artifacts", ansi.Reset))
	for _, art := range delivery.Artifacts {
		added := false
		for _, a := range addedArtifacts {
			if a.Equal(art) {
				added = true
				break
			}
		}
		if added {
			artNode.AddMetaNode(fmt.Sprintf("%s%s%s", ansi.Green, "added", ansi.Reset), art.RefName())
		} else {
			artNode.AddNode(art.RefName())
		}
	}

	envNode := tree.AddBranch(fmt.Sprintf("%s%s%s", ansi.ColorCode("blue+b"), "environments", ansi.Reset))
	for _, env := range delivery.Environments {
		envBranch := envNode.AddBranch(fmt.Sprintf("%s%s%s", ansi.ColorCode("default+hb"), env.Name, ansi.Reset))

		// collect all the types so we can print the resources in order by type
		uniqTypes := map[string]struct{}{}
		for _, resource := range env.Resources {
			uniqTypes[resource.ResourceType()] = struct{}{}
		}
		types := []string{}
		for t := range uniqTypes {
			types = append(types, t)
		}
		sort.Strings(types)
		for _, resourceType := range types {
			// add branch for this resource type
			rsrcBranch := envBranch.AddBranch(fmt.Sprintf("%s%ss%s", ansi.ColorCode("blue+b"), resourceType, ansi.Reset))
			for _, resource := range env.Resources {
				if resource.ResourceType() != resourceType {
					continue
				}
				// it will not be found it not modified (already existed in delivery config)
				found := false
				for expRsrc, added := range modifiedResources {
					if resource.Match(expRsrc) {
						meta := "updated"
						if added {
							meta = "added"
						}
						rsrcBranch.AddMetaNode(
							fmt.Sprintf("%s%s%s", ansi.Green, meta, ansi.Reset),
							fmt.Sprintf("%s [%s]", resource.Name(), resource.Account()),
						)
						found = true
						break
					}
				}
				if !found {
					rsrcBranch.AddNode(
						fmt.Sprintf("%s [%s]", resource.Name(), resource.Account()),
					)
				}
			}
		}
	}

	fmt.Fprintf(opts.Stdout, "Export Summary:\n%s", tree.String())

	if len(errors) > 0 {
		fmt.Fprintf(opts.Stderr, "ERROR: Some errors occurred during export:\n")
		for _, err := range errors {
			fmt.Fprintf(opts.Stderr, "ERROR: %s\n", err)
		}
		// we handled the errors here, just return non-zero exit code
		return 1, nil
	}
	return 0, nil
}
