package mdlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	return "aws"
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

type DeliveryConfigProcessor struct {
	appName           string
	serviceAccount    string
	fileName          string
	dirName           string
	rawDeliveryConfig map[string]interface{}
	deliveryConfig    DeliveryConfig
	yamlMarshal       func(interface{}) ([]byte, error)
	yamlUnmarshal     func([]byte, interface{}) error
}

type ProcessorOption func(p *DeliveryConfigProcessor)

func NewDeliveryConfigProcessor(opts ...ProcessorOption) *DeliveryConfigProcessor {
	p := &DeliveryConfigProcessor{
		fileName:      DefaultDeliveryConfigFileName,
		dirName:       DefaultDeliveryConfigDirName,
		yamlMarshal:   defaultYAMLMarshal,
		yamlUnmarshal: yaml.Unmarshal,
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

func WithAppName(a string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.appName = a
	}
}

func WithServiceAccount(a string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.serviceAccount = a
	}
}

func WithYAMLMarshal(marshaller func(interface{}) ([]byte, error)) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.yamlMarshal = marshaller
	}
}

func WithYAMLUnmarshal(unmarshaller func([]byte, interface{}) error) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.yamlUnmarshal = unmarshaller
	}
}

func defaultYAMLMarshal(opts interface{}) ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := yaml.NewEncoder(buf)
	enc.SetIndent(2)
	err := enc.Encode(opts)
	return buf.Bytes(), err
}

func (p *DeliveryConfigProcessor) Load() error {
	p.rawDeliveryConfig = map[string]interface{}{}
	p.deliveryConfig = DeliveryConfig{}

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
	log.Printf("Saving")
	if _, ok := p.rawDeliveryConfig["name"]; !ok && p.appName != "" {
		p.rawDeliveryConfig["name"] = fmt.Sprintf("%s-manifest", p.appName)
	}
	if _, ok := p.rawDeliveryConfig["application"]; !ok && p.appName != "" {
		p.rawDeliveryConfig["application"] = p.appName
	}
	if _, ok := p.rawDeliveryConfig["serviceAccount"]; !ok && p.serviceAccount != "" {
		p.rawDeliveryConfig["serviceAccount"] = p.serviceAccount
	}

	output, err := p.yamlMarshal(&p.rawDeliveryConfig)
	if err != nil {
		return stacktrace.Propagate(err, "")
	}

	err = os.MkdirAll(p.dirName, 0755)
	if err != nil {
		return stacktrace.Propagate(err, "failed to create directory %s", p.dirName)
	}

	deliveryFile := filepath.Join(p.dirName, p.fileName)

	log.Printf("Writing to %s", deliveryFile)
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
	data, err := p.bytesToData(content)
	if err != nil {
		return stacktrace.Propagate(err, "failed to parse content")
	}
	envIx := p.findEnvIndex(envName)
	if environments, ok := p.rawDeliveryConfig["environments"].([]interface{}); !ok || envIx < 0 {
		// new environment
		environments = append(environments, map[string]interface{}{
			"name":          envName,
			"constraints":   []interface{}{DefaultEnvironmentConstraint},
			"notifications": []interface{}{},
			"resources":     []interface{}{data},
		})
		p.rawDeliveryConfig["environments"] = environments
		// update in memory struct in case we look for this environment again later
		p.deliveryConfig.Environments = append(p.deliveryConfig.Environments, DeliveryEnvironment{
			Name:      envName,
			Resources: []DeliveryResource{DeliveryResource{}},
		})
	} else {
		if env, ok := environments[envIx].(map[string]interface{}); ok {
			if _, ok := env["constraints"].([]interface{}); !ok {
				env["constraints"] = []interface{}{DefaultEnvironmentConstraint}
			}
			if _, ok := env["notifications"].([]interface{}); !ok {
				env["notifications"] = []interface{}{}
			}
			if resources, ok := env["resources"].([]interface{}); ok {
				resourceIx := p.findResourceIndex(resource, envIx)
				if resourceIx < 0 {
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

func (p *DeliveryConfigProcessor) bytesToData(content []byte) (data interface{}, err error) {
	return data, p.yamlUnmarshal(content, &data)
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
		// log.Printf("Resource Kind: %s CloudProvider: %s Account: %s Name: %s", resource.Kind, resource.CloudProvider(), resource.Account(), resource.Name())
		// log.Printf("Search   Kind: %s CloudProvider: %s Account: %s Name: %s", search.ResourceType, search.CloudProvider, search.Account, search.Name)
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

func (p *DeliveryConfigProcessor) ResourceExists(search *ExportableResource) bool {
	for eix := range p.deliveryConfig.Environments {
		rix := p.findResourceIndex(search, eix)
		if rix >= 0 {
			return true
		}
	}
	return false
}

func (p *DeliveryConfigProcessor) InsertArtifact(artifact *DeliveryArtifact) {
	for _, current := range p.deliveryConfig.Artifacts {
		if current.Name == artifact.Name && current.Type == artifact.Type {
			// found an existing artifact, so do not insert this one
			return
		}
	}
	p.deliveryConfig.Artifacts = append(p.deliveryConfig.Artifacts, *artifact)
	p.rawDeliveryConfig["artifacts"] = p.deliveryConfig.Artifacts
}

func (p *DeliveryConfigProcessor) Publish(cli *Client) error {
	if p.rawDeliveryConfig == nil {
		err := p.Load()
		if err != nil {
			return stacktrace.Propagate(err, "Failed to load delivery config")
		}
	}

	encoded, err := json.Marshal(&p.rawDeliveryConfig)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to serialized delivery config")
	}

	_, err = commonRequest(cli, "POST", "/managed/delivery-configs", bytes.NewReader(encoded))

	if err != nil {
		return stacktrace.Propagate(err, "Failed to post delivery config to spinnaker")
	}

	return nil
}

// ResourceDiff contains the exact records that differ
type ResourceDiff struct {
	State   string `json:"state" yaml:"state"`
	Desired string `json:"desired" yaml:"desired"`
	Current string `json:"current" yaml:"current"`
}

// ManagedResourceDiff contains the details about a specific resource and if it has diffs
type ManagedResourceDiff struct {
	Status     string                  `json:"status" yaml:"status"`
	ResourceID string                  `json:"resourceId" yaml:"resourceId"`
	Resource   DeliveryResource        `json:"resource" yaml:"resource"`
	Diffs      map[string]ResourceDiff `json:"diff" yaml:"diff"`
}

func (p *DeliveryConfigProcessor) Diff(cli *Client) ([]*ManagedResourceDiff, error) {
	if p.rawDeliveryConfig == nil {
		err := p.Load()
		if err != nil {
			return nil, stacktrace.Propagate(err, "Failed to load delivery config")
		}
	}

	encoded, err := json.Marshal(&p.rawDeliveryConfig)
	if err != nil {
		return nil, stacktrace.Propagate(err, "Failed to serialized delivery config")
	}

	content, err := commonRequest(cli, "POST", "/managed/delivery-configs/diff", bytes.NewReader(encoded))

	if err != nil {
		return nil, stacktrace.Propagate(err, "Failed to diff delivery config with spinnaker")
	}

	data := []struct {
		ResourceDiffs []*ManagedResourceDiff `json:"resourceDiffs" yaml:"resourceDiffs"`
	}{}

	err = json.Unmarshal(content, &data)
	if err != nil {
		return nil, stacktrace.Propagate(err, "failed to parse response from diff api")
	}

	diffs := []*ManagedResourceDiff{}
	for _, accountDiffs := range data {
		diffs = append(diffs, accountDiffs.ResourceDiffs...)
	}

	sort.Slice(diffs, func(i, j int) bool {
		// split resource ID
		idPartsI := strings.Split(diffs[i].ResourceID, ":")
		idPartsJ := strings.Split(diffs[j].ResourceID, ":")
		if idPartsI[0] == idPartsJ[0] && len(idPartsI) > 2 && len(idPartsJ) > 2 && idPartsI[2] != idPartsJ[2] {
			return idPartsI[2] < idPartsJ[2]
		}
		return diffs[i].ResourceID < diffs[j].ResourceID
	})

	return diffs, nil
}
