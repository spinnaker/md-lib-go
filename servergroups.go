package mdlib

import (
	"fmt"
	"strings"
)

// ServerGroup is a collection of instances of the running software deployed
// from spinnaker. It identifies the deployable artifact and contain basic
// configuration settings suchas number of instances, autoscaling policies
// metadata, etc.
type ServerGroup struct {
	Name           string     `json:"name"`
	Application    string     `json:"application"`
	Region         string     `json:"region"`
	Account        string     `json:"account"`
	Type           string     `json:"type"`
	Moniker        Moniker    `json:"moniker"`
	Instances      []Instance `json:"instances"`
	LoadBalancers  []string   `json:"loadBalancers"`
	TargetGroups   []string   `json:"targetGroups"`
	SecurityGroups []string   `json:"securityGroups"`
	BuildInfo      BuildInfo  `json:"buildInfo"`
}

// Instance is a spinnaker instance of a deployable artifact. This can
// represent an AWS EC2 instance or a titus container.
type Instance struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Health           []InstanceHealth `json:"health"`
	HealthState      string           `json:"healthState"`
	LaunchTime       int64            `json:"launchTime"`
	AvailabilityZone string           `json:"availabilityZone"`
}

// InstanceHealth is the health of a spinnaker instance for its deployment
// targets.
type InstanceHealth struct {
	Type  string `json:"type"`
	State string `json:"state"`
}

// BuildInfo contains information about the build artifact that is deployed to the server group
type BuildInfo struct {
	PackageName string                      `json:"package_name"`
	Jenkins     ServerGroupJenkinsBuildInfo `json:"jenkins"`
	Docker      ServerGroupDockerBuildInfo  `json:"docker"`
}

// ServerGroupJenkinsBuildInfo contains jenkins specific information from the server group buildInfo
type ServerGroupJenkinsBuildInfo struct {
	Name   string `json:"name"`
	Number string `json:"number"`
	Host   string `json:"host"`
}

// ServerGroupDockerBuildInfo contains docker specific information from the server group buildInfo
type ServerGroupDockerBuildInfo struct {
	Image  string `json:"image"`
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

// Moniker is a spinnaker naming strategy. Every resource is assigned a Moniker
// which contains identification metadata.
type Moniker struct {
	App      string `json:"app"`
	Cluster  string `json:"cluster"`
	Detail   string `json:"detail"`
	Stack    string `json:"stack"`
	Sequence int    `json:"sequence"`
}

func (m Moniker) String() string {
	parts := []string{}
	for _, part := range []string{m.App, m.Stack, m.Detail} {
		if part == "" {
			break
		}
		parts = append(parts, part)
	}
	if m.Sequence > 0 {
		parts = append(parts, fmt.Sprintf("v%03d", m.Sequence))
	}
	return strings.Join(parts, "-")
}

// GetServerGroups populates the server groups result structure for spinnaker application appName.
// Unless a custom result type is required, *[]ServerGroup is recommended.
func GetServerGroups(cli *Client, appName string, result interface{}) error {
	return commonParsedGet(cli, fmt.Sprintf("/applications/%s/serverGroups", appName), result)
}
