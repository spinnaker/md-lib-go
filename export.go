package mdlib

import (
	"fmt"
	"strings"

	"github.com/palantir/stacktrace"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

const (
	// ClusterResourceType is the keyword used to classify the resource type for clusters.
	ClusterResourceType = "cluster"
	// LoadBalancerResourceType is the keyword used to classify the resource type for classic elastic load balancers.
	LoadBalancerResourceType = "classic-load-balancer"
	// ApplicationLoadBalancerResourceType is the keyword used to classify the resource type for an application load balancer.
	ApplicationLoadBalancerResourceType = "application-load-balancer"
	// SecurityGroupResourceType is the keyword used to classify the resource type for security groups.
	SecurityGroupResourceType = "security-group"

	// AWSCloudProvider is the keyword used to classify that a resource is intended for AWS
	AWSCloudProvider = "aws"
	// TitusCloudProvider is the keyword used to classify that a resource is intended for Titus
	TitusCloudProvider = "titus"

	// DebianArtifactType is the keyword to to classify a debian artifact
	DebianArtifactType = "deb"
	// DockerArtifactType is the keyword to to classify a docker image artifact
	DockerArtifactType = "docker"
)

// ExportableResource is structure to contain the necessary information to uniquely identify a resource stored
// in the delivery config or to export from Spinnaker API.
type ExportableResource struct {
	ResourceType  string
	CloudProvider string
	Account       string
	Name          string
}

// String returns a useful formatting string to display an ExportableResource.
func (r ExportableResource) String() string {
	return fmt.Sprintf("%s %s [%s/%s]", r.ResourceType, r.Name, r.CloudProvider, r.Account)
}

// HasKind will return true if the resource matches the provided kind.
func (r ExportableResource) HasKind(kind string) bool {
	kindProvider := r.CloudProvider
	if kindProvider == "aws" {
		kindProvider = "ec2"
	}
	// does it match ec2/cluster@v1
	return strings.HasPrefix(kind, fmt.Sprintf("%s/%s@", kindProvider, r.ResourceType))
}

// ResourceSorter is a wrapper to help sort ExportableResources
type ResourceSorter []*ExportableResource

// Len fulfills the sort.Interface requirement
func (s ResourceSorter) Len() int { return len(s) }

// Swap fulfills the sort.Interface requirement
func (s ResourceSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less fulfills the sort.Interface requirement
func (s ResourceSorter) Less(i, j int) bool {
	if s[i].ResourceType != s[j].ResourceType {
		return s[i].ResourceType < s[j].ResourceType
	}
	if s[i].Name != s[j].Name {
		return s[i].Name < s[j].Name
	}
	if s[i].CloudProvider != s[j].CloudProvider {
		return s[i].CloudProvider < s[j].CloudProvider
	}
	return s[i].Account < s[j].Account
}

// ArtifactSorter is a wrapper to help sort DeliveryArtifacts
type ArtifactSorter []*DeliveryArtifact

// Len fulfills the sort.Interface requirement
func (s ArtifactSorter) Len() int { return len(s) }

// Swap fulfills the sort.Interface requirement
func (s ArtifactSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less fulfills the sort.Interface requirement
func (s ArtifactSorter) Less(i, j int) bool {
	if s[i].Name != s[j].Name {
		return s[i].Name < s[j].Name
	}
	return s[i].Type < s[j].Type
}

// Account is just a string type, used to make code more readable
type Account = string

// ApplicationResources is used to track all the resources for an application
// as populated from FinndApplicationResources.
type ApplicationResources struct {
	AppName        string
	ServerGroups   []ServerGroup
	LoadBalancers  []LoadBalancer
	SecurityGroups map[Account]SecurityGroups
}

// FindApplicationResources will collect application resources from various Spinnaker REST
// APIs, loading resources in parallel when possible.
func FindApplicationResources(cli *Client, appName string) (*ApplicationResources, error) {
	var g errgroup.Group
	data := &ApplicationResources{
		AppName:        appName,
		SecurityGroups: make(map[Account]SecurityGroups),
	}

	g.Go(func() error {
		return GetServerGroups(cli, appName, &data.ServerGroups)
	})
	g.Go(func() error {
		return GetLoadBalancers(cli, appName, &data.LoadBalancers)
	})

	err := g.Wait()
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to load resources")
	}

	uniqAccounts := map[string]struct{}{}

	for _, asg := range data.ServerGroups {
		uniqAccounts[asg.Account] = struct{}{}
	}

	for _, lb := range data.LoadBalancers {
		uniqAccounts[lb.Account] = struct{}{}
	}

	// accounts might not primary or secondary.  Secondary accounts are usually logical partitions within
	// an existing "primary" account.  But security groups are only on the primary accounts, so
	// if we have any secondary accounts, we need to remap it back to the primary for the secGroup lookup.
	uniqPrimaryAccounts := map[string]struct{}{}
	for account := range uniqAccounts {
		cred := Credential{}
		err := GetCredential(cli, account, &cred)
		if err != nil {
			return nil, stacktrace.Propagate(err, "failed to load credential data for account %q", account)
		}
		if cred.AWSAccount == "" {
			uniqPrimaryAccounts[account] = struct{}{}
		} else {
			uniqPrimaryAccounts[cred.AWSAccount] = struct{}{}
		}
	}

	g = errgroup.Group{}
	for account := range uniqPrimaryAccounts {
		account := account
		sgs := make(SecurityGroups)
		// TODO do we need defer?
		data.SecurityGroups[account] = sgs
		g.Go(func() error {
			return GetSecurityGroups(cli, account, &sgs)
		})
	}
	err = g.Wait()
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to load security group resources")
	}

	return data, nil
}

// ExportableApplicationResources will return a list of ExportableResources that
// are found from the currently deployed application resources.
func ExportableApplicationResources(appData *ApplicationResources) []*ExportableResource {
	uniqResources := map[ExportableResource]struct{}{}

	for _, asg := range appData.ServerGroups {
		uniqResources[ExportableResource{ClusterResourceType, asg.Type, asg.Account, asg.Moniker.Cluster}] = struct{}{}
	}

	for _, lb := range appData.LoadBalancers {
		resourceType := LoadBalancerResourceType
		if len(lb.TargetGroups) > 0 {
			resourceType = ApplicationLoadBalancerResourceType
		}

		// only export things by default that look like the belong to this app
		if strings.HasPrefix(lb.Name, appData.AppName) {
			uniqResources[ExportableResource{resourceType, lb.Type, lb.Account, lb.Name}] = struct{}{}
		}
	}

	for account, securityGroupRegions := range appData.SecurityGroups {
		for region := range securityGroupRegions {
			for _, sg := range securityGroupRegions[region] {
				if strings.HasPrefix(sg.Name, appData.AppName) {
					uniqResources[ExportableResource{SecurityGroupResourceType, AWSCloudProvider, account, sg.Name}] = struct{}{}
				}
			}
		}
	}

	exportable := []*ExportableResource{}
	for resource := range uniqResources {
		resource := resource
		exportable = append(exportable, &resource)
	}
	return exportable
}

// ExportResource will contact the Spinnaker REST API to collect the YAML delivery config representation for
// a specific resource.
func ExportResource(cli *Client, resource *ExportableResource, serviceAccount string) ([]byte, error) {
	return commonRequest(cli, "GET",
		fmt.Sprintf("/managed/resources/export/%s/%s/%s/%s?serviceAccount=%s",
			resource.CloudProvider,
			resource.Account,
			resource.ResourceType,
			resource.Name,
			serviceAccount,
		),
		requestBody{},
	)
}

// ExportArtifact will contact the Spinnaker REST API to collect the YAML delivery config representation for
// the artifacts for the given cluster
func ExportArtifact(cli *Client, resource *ExportableResource, result interface{}) error {
	content, err := commonRequest(cli, "GET",
		fmt.Sprintf("/managed/resources/export/artifact/%s/%s/%s",
			resource.CloudProvider,
			resource.Account,
			resource.Name,
		),
		requestBody{},
	)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}
	err = yaml.Unmarshal(content, result)
	if err != nil {
		return stacktrace.Propagate(ErrorInvalidContent{Content: content, ParseError: err}, "")
	}
	return nil
}
