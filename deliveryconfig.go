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
	// DefaultDeliveryConfigFileName is the default name of the delivery config file for spinnaker managed delivery.
	DefaultDeliveryConfigFileName = "spinnaker.yml"

	// DefaultDeliveryConfigDirName is the default directory where the delivery config file is read and written to.
	DefaultDeliveryConfigDirName = "."

	// DefaultEnvironmentConstraint is the default constraint for an added environment while exporting new resources.
	DefaultEnvironmentConstraint interface{} = map[string]string{
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
	Reference          string `json:"reference,omitempty" yaml:"reference,omitempty"`
	TagVersionStrategy string `json:"tagVersionStrategy.omitempty" yaml:"tagVersionStrategy,omitempty"`
}

// RefName returns the Reference value for comparisons.  it will use the
// Reference value if defined, otherwise default to the Name value.
func (a *DeliveryArtifact) RefName() string {
	if a.Reference != "" {
		return a.Reference
	}
	return a.Name
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

// CloudProvider retuurns the cloud provider for a resource.  Currently it
// only return titus or aws
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

// DeliveryConfigProcessor is a structure to manage operations on a delivery config.
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

// ProcessorOption is the interface to provide variadic options to NewDeliveryConfigProcessor
type ProcessorOption func(p *DeliveryConfigProcessor)

// NewDeliveryConfigProcessor will create a DeliveryConfigProcessor and apply all provided options.
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

// WithDirectory is a ProcessorOption to set the directory where the delivery config is stored.
func WithDirectory(d string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.dirName = d
	}
}

// WithFile is a ProcessorOption to set the name of the delivery config file.
func WithFile(f string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.fileName = f
	}
}

// WithAppName is a ProcessorOption to set the name of the Spinnaker application name that the delivery config corresponds to.
// It is only necessary to set when exporting/creating a delivery config.
func WithAppName(a string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.appName = a
	}
}

// WithServiceAccount is a ProcessorOption to set the service account used for access control for the delivery config operations.
func WithServiceAccount(a string) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.serviceAccount = a
	}
}

// WithYAMLMarshal is a ProcessorOption to allow customizing how the delivery config is serialized to disk.
func WithYAMLMarshal(marshaller func(interface{}) ([]byte, error)) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		p.yamlMarshal = marshaller
	}
}

// WithYAMLUnmarshal is a ProcessorOption to allow customizing how the delivery config is loaded from disk.
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

// Load will load the delivery config files from disk.
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

// Save will serialize the delivery config to disk.
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

// AllEnvironments will return a list of the names of all the environments in the delivery config as well
// as the default/recommended environment names: testing, staging, and production.
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

// WhichEnvironment will return the environment name for the given resource found in the delivery config.
// It will return an empty string if the resource is not found in any environment.
func (p *DeliveryConfigProcessor) WhichEnvironment(resource *ExportableResource) string {
	// TODO
	return "testing"
}

// UpsertResource will update (if exists) or insert (if new) a resource into the delivery config.  The resource will
// be added to the environment that corresponds to envName if the resource is new.
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
			Resources: []DeliveryResource{{}},
		})
	} else if env, ok := environments[envIx].(map[string]interface{}); ok {
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

// ResourceExists returns true if the provided resource is found currently in the delivery config.
func (p *DeliveryConfigProcessor) ResourceExists(search *ExportableResource) bool {
	for eix := range p.deliveryConfig.Environments {
		rix := p.findResourceIndex(search, eix)
		if rix >= 0 {
			return true
		}
	}
	return false
}

// InsertArtifact will add an artifact to the delivery config if it is not already present.
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

// Publish will post the delivery config to the Spinnaker API so that Spinnaker
// will update the Managed state for the application.
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

// Diff will compare the delivery config file on disk with the currently deployed state of
// the Spinnaker application and report any changes.  This can also be used to validate
// a delivery config (errors will be returned when an invalid delivery config is submitted).
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

// Delete will stop the delivery config from being managed, and will cause Spinnaker
// to remove all historical state about managing this configuration.
func (p *DeliveryConfigProcessor) Delete(cli *Client) error {
	if p.rawDeliveryConfig == nil {
		err := p.Load()
		if err != nil {
			return stacktrace.Propagate(err, "Failed to load delivery config")
		}
	}
	_, err := commonRequest(cli, "DELETE", fmt.Sprintf("/managed/delivery-configs/%s", p.deliveryConfig.Name), nil)
	return err
}
