package mdlib

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/palantir/stacktrace"
	"gopkg.in/yaml.v3"
)

var (
	DefaultDeliveryConfigFileName             = "spinnaker.yml"
	DefaultDeliveryConfigDirName              = "."
	DefaultEnvironmentConstraint  interface{} = map[string]string{
		"type": "manual-judgement",
	}
)

// DeliveryConfig holds the structure for the manage delivery config stored in .netflix/spinnaker.yml
type DeliveryConfig struct {
	Name         string
	Application  string
	Artifacts    []DeliveryArtifact
	Environments []DeliveryEnvironment
}

// DeliveryEnvironment contains the resources per environment.
type DeliveryEnvironment struct {
	Name      string
	Resources []DeliveryResource
}

// DeliveryArtifact holds artifact details used for managed delivery
type DeliveryArtifact struct {
	Name               string
	Type               string
	TagVersionStrategy string `json:"tagVersionStrategy.omitempty" yaml:"tagVersionStrategy,omitempty"`
}

// DeliveryResource contains the necessary configuration for a managed delivery resource
type DeliveryResource struct {
	ApiVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       string
	Spec       DeliveryResourceSpec
}

// Name returns the name for the type of delivery resource
func (r DeliveryResource) Name() string {
	if r.Kind == "image" {
		return r.Spec.ArtifactName
	}
	return r.Spec.Moniker.String()
}

// Account return the account for the delivery resource
func (r DeliveryResource) Account() string {
	if r.Kind == "image" {
		// image does not have a location, fake it out now
		return "bakery"
	}
	return r.Spec.Locations.Account
}

func (r DeliveryResource) CloudProvider() string {
	if strings.Contains(r.ApiVersion, "titus") {
		return "titus"
	}
	return "ec2"
}

// DeliveryResourceSpec is the spec for the delivery resource
type DeliveryResourceSpec struct {
	Moniker       Moniker
	Locations     DeliveryResourceLocations
	ImageProvider DeliveryImageProvider     `json:"imageProvider" yaml:"imageProvider"`
	ArtifactName  string                    `json:"artifactName" yaml:"artifactName"`
	Container     DeliveryResourceContainer `json:"container" yaml:"container"`
}

// DeliveryResourceLocations contains location details for delivery resources
type DeliveryResourceLocations struct {
	Account string
}

// DeliveryImageProvider contains the artifact details used to make the image
type DeliveryImageProvider struct {
	DeliveryArtifact DeliveryArtifact `json:"deliveryArtifact" yaml:"deliveryArtifact"`
}

// DeliveryResourceContainer contains details about the image deployed for a container.
type DeliveryResourceContainer struct {
	Image              string `json:"image" yaml:"image"`
	Organization       string `json:"organization" yaml:"organization"`
	TagVersionStrategy string `json:"tagVersionStrategy" yaml:"tagVersionStrategy"`
}

// map current resources in delivery config
// create update/export prompt message
// prompt for resources to export
// create list of environment names, append defaults if not present
// for each resource to export:
// - inject magic image resource
// - export resource if not image
// if resource not in delivery config
// - prompt for environment
// - else update delivery config
// set delivery config defaults if not present

type DeliveryConfigProcessor struct {
	fileName          string
	dirName           string
	appName           string
	rawDeliveryConfig map[string]interface{}
	deliveryConfig    DeliveryConfig
}

type ProcessorOption func(p *DeliveryConfigProcessor)

func NewDeliveryConfigProcessor(appName string, opts ...ProcessorOption) *DeliveryConfigProcessor {
	p := &DeliveryConfigProcessor{
		fileName: DefaultDeliveryConfigFileName,
		dirName:  DefaultDeliveryConfigDirName,
		appName:  appName,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithDirectory(d string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.dirName = d
	}
}

func WithFile(f string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.fileName = f
	}
}

func (p *DeliveryConfigProcessor) Load() error {
	deliveryFile := filepath.Join(p.dirName, p.fileName)
	if _, err := os.Stat(deliveryFile); err != nil && os.IsNotExist(err) {
		// file does not exist, skip
		return nil
	} else if err != nil {
		return stacktrace.Propagate(err, "failed to stat %s", deliveryFile)
	}
	content, err := ioutil.ReadFile(deliveryFile)
	if err != nil {
		return stacktrace.Propagate(err, "failed to read %s", deliveryFile)
	}

	p.rawDeliveryConfig = map[string]interface{}{}

	err = yaml.Unmarshal(content, &p.rawDeliveryConfig)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to parse contents of %s as yaml", deliveryFile)
	}

	err = yaml.Unmarshal(content, &p.deliveryConfig)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to parse contents of %s as yaml", deliveryFile)
	}
	return nil
}

func (p *DeliveryConfigProcessor) Save() error {
	if _, ok := p.rawDeliveryConfig["name"]; !ok {
		p.rawDeliveryConfig["name"] = fmt.Sprintf("%s-manifest", p.appName)
	}
	if _, ok := p.rawDeliveryConfig["application"]; !ok {
		p.rawDeliveryConfig["application"] = p.appName
	}

	output, err := yaml.Marshal(&p.rawDeliveryConfig)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	err = os.MkdirAll(p.dirName, 0755)
	if err != nil {
		return stacktrace.Propagate(err, "failed to create directory %s", p.dirName)
	}

	deliveryFile := filepath.Join(p.dirName, p.fileName)

	err = ioutil.WriteFile(deliveryFile, output, 0644)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}
	return nil
}

func (p *DeliveryConfigProcessor) AllEnvironments() []string {
	environments := []string{}
	for _, env := range p.deliveryConfig.Environments {
		environments = append(environments, env.Name)
	}
	// ensure commonly used env names are available
	for _, envName := range []string{"testing", "staging", "production"} {
		found := false
		for _, env := range environments {
			if envName == env {
				found = true
				break
			}
		}
		if !found {
			environments = append(environments, envName)
		}
	}
	return environments
}

func (p *DeliveryConfigProcessor) WhichEnvironment(resource *ExportableResource) string {
	return "testing"
}

func (p *DeliveryConfigProcessor) UpsertResource(resource *ExportableResource, envName string, content []byte) error {
	data, err := bytesToData(content)
	if err != nil {
		return stacktrace.Propagate(err, "failed to parse content")
	}
	envIx := p.findEnvIndex(envName)
	if environments, ok := p.rawDeliveryConfig["environments"].([]interface{}); !ok || envIx < 0 {
		// new environment
		environments = append(environments, map[interface{}]interface{}{
			"name":          envName,
			"constraints":   []interface{}{DefaultEnvironmentConstraint},
			"notifications": []interface{}{},
			"resources":     []interface{}{data},
		})
		p.rawDeliveryConfig["environments"] = environments
	} else {
		if env, ok := environments[envIx].(map[interface{}]interface{}); ok {
			if _, ok := env["constraints"].([]interface{}); !ok {
				env["constraints"] = []interface{}{DefaultEnvironmentConstraint}
			}
			if _, ok := env["notifications"].([]interface{}); !ok {
				env["notifications"] = []interface{}{}
			}
			if resources, ok := env["resources"].([]interface{}); ok {
				resourceIx := p.findResourceIndex(resource, envIx)
				if len(resources) < resourceIx {
					resources = append(resources, data)
				} else {
					resources[resourceIx] = data
				}
				env["resources"] = resources
				environments[envIx] = env
				p.rawDeliveryConfig["environments"] = environments
			}
		}
	}
	return nil
}

func bytesToData(content []byte) (data interface{}, err error) {
	return &data, json.Unmarshal(content, &data)
}

func (p *DeliveryConfigProcessor) findEnvIndex(envName string) int {
	for ix, env := range p.deliveryConfig.Environments {
		if env.Name == envName {
			return ix
		}
	}
	return -1
}

func (p *DeliveryConfigProcessor) findResourceIndex(search *ExportableResource, envIx int) int {
	if len(p.deliveryConfig.Environments) < envIx {
		// env not found so no resources to find
		return -1
	}

	for ix, resource := range p.deliveryConfig.Environments[envIx].Resources {
		if resource.Kind != search.ResourceType ||
			resource.CloudProvider() != search.CloudProvider ||
			resource.Account() != search.Account ||
			resource.Name() != search.Name {
			continue
		}
		return ix
	}
	return -1
}

func (p *DeliveryConfigProcessor) InsertArtifact(artifact *DeliveryArtifact) {
	for _, current := range p.deliveryConfig.Artifacts {
		if current.Name == artifact.Name && current.Type == artifact.Type {
			// found an existing artifact, so do not insert this one
			return
		}
	}
	p.deliveryConfig.Artifacts = append(p.deliveryConfig.Artifacts, &artifact)
	p.rawDeliveryConfig["artifacts"] = p.deliveryConfig.Artifacts
}
