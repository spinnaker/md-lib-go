package mdlib

import (
	"fmt"
	"strings"

	"github.com/palantir/stacktrace"
	"golang.org/x/sync/errgroup"
)

const (
	ClusterResourceType                 = "cluster"
	LoadBalancerResourceType            = "load-balancer"
	ApplicationLoadBalancerResourceType = "application-load-balancer"
	SecurityGroupResourceType           = "security-group"

	AWSCloudProvider   = "aws"
	TitusCloudProvider = "titus"

	DebianArtifactType = "deb"
	DockerArtifactType = "docker"
)

type ExportableResource struct {
	ResourceType  string
	CloudProvider string
	Account       string
	Name          string
}

func (r ExportableResource) String() string {
	return fmt.Sprintf("%s %s [%s/%s]", r.ResourceType, r.Name, r.CloudProvider, r.Account)
}

type ResourceSorter []*ExportableResource

func (s ResourceSorter) Len() int      { return len(s) }
func (s ResourceSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
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

type ArtifactSorter []*DeliveryArtifact

func (s ArtifactSorter) Len() int      { return len(s) }
func (s ArtifactSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ArtifactSorter) Less(i, j int) bool {
	if s[i].Name != s[j].Name {
		return s[i].Name < s[j].Name
	}
	return s[i].Type < s[j].Type
}

// Account is just a string type, used to make code more readable
type Account = string

type ApplicationResources struct {
	AppName        string
	ServerGroups   []ServerGroup
	LoadBalancers  []LoadBalancer
	SecurityGroups map[Account]SecurityGroups
}

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

	g = errgroup.Group{}
	for account := range uniqAccounts {
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

func ReferencedArtifacts(appData *ApplicationResources) []*DeliveryArtifact {
	uniqArtifacts := map[DeliveryArtifact]struct{}{}

	for _, asg := range appData.ServerGroups {
		if asg.BuildInfo.PackageName != "" {
			uniqArtifacts[DeliveryArtifact{
				Name: asg.BuildInfo.PackageName,
				Type: DebianArtifactType,
			}] = struct{}{}
			continue
		}
		if asg.BuildInfo.Docker.Image != "" {
			uniqArtifacts[DeliveryArtifact{
				Name: asg.BuildInfo.Docker.Image,
				Type: DockerArtifactType,
			}] = struct{}{}
			continue
		}
	}

	artifacts := []*DeliveryArtifact{}
	for artifact := range uniqArtifacts {
		artifact := artifact
		artifacts = append(artifacts, &artifact)
	}

	return artifacts
}

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

func ExportResource(cli *Client, resource *ExportableResource, serviceAccount string) ([]byte, error) {
	return commonRequest(cli, "GET",
		fmt.Sprintf("/managed/resources/export/%s/%s/%s/%s?serviceAccount=%s",
			resource.CloudProvider,
			resource.Account,
			resource.ResourceType,
			resource.Name,
			serviceAccount,
		),
		nil,
	)
}
