package mdlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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
	VMOptions          struct {
		BaseLabel string   `json:"baseLabel,omitempty" yaml:"baseLabel,omitempty"`
		BaseOS    string   `json:"baseOs,omitempty" yaml:"baseOs,omitempty"`
		Regions   []string `json:"regions,omitempty" yaml:"regions,omitempty"`
		StoreType string   `json:"storeType,omitempty" yaml:"storeType,omitempty"`
	} `json:"vmOptions,omitempty" yaml:"vmOptions,omitempty"`
}

// RefName returns the Reference value for comparisons.  it will use the
// Reference value if defined, otherwise default to the Name value.
func (a *DeliveryArtifact) RefName() string {
	if a.Reference != "" {
		return a.Reference
	}
	return a.Name
}

func (a *DeliveryArtifact) Equal(b *DeliveryArtifact) bool {
	// note we ignore the `Name` property when comparing equality
	if a.RefName() != b.RefName() {
		return false
	}
	if a.TagVersionStrategy != b.TagVersionStrategy {
		return false
	}
	if !reflect.DeepEqual(a.VMOptions, b.VMOptions) {
		return false
	}
	return true

}

// DeliveryResource contains the necessary configuration for a managed delivery resource
type DeliveryResource struct {
	Kind string
	Spec DeliveryResourceSpec
}

// Name returns the name for the type of delivery resource
func (r DeliveryResource) Name() string {
	return r.Spec.Moniker.String()
}

// Account return the account for the delivery resource
func (r DeliveryResource) Account() string {
	return r.Spec.Locations.Account
}

// CloudProvider retuurns the cloud provider for a resource.  Currently it
// only return titus or aws
func (r DeliveryResource) CloudProvider() string {
	// Kind is like ec2/cluster@v1 or titus/cluster@v1
	// but CloudProvider needs to be "aws" for "ec2"
	// so make that mapping here
	parts := strings.SplitN(r.Kind, "/", 2)
	if len(parts) == 0 {
		return "unknown-cloud-provider"
	}
	if parts[0] == "ec2" {
		return "aws"
	}
	return parts[0]
}

func (r *DeliveryResource) Match(e *ExportableResource) bool {
	if e.HasKind(r.Kind) &&
		r.CloudProvider() == e.CloudProvider &&
		r.Account() == e.Account &&
		r.Name() == e.Name {
		return true
	}
	return false
}

func (r *DeliveryResource) ResourceType() string {
	left := strings.Index(r.Kind, "/")
	right := strings.LastIndex(r.Kind, "@")
	return r.Kind[left+1 : right]
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
	appName               string
	serviceAccount        string
	fileName              string
	dirName               string
	rawDeliveryConfig     map[string]interface{}
	deliveryConfig        DeliveryConfig
	content               []byte
	yamlMarshal           func(interface{}) ([]byte, error)
	yamlUnmarshal         func([]byte, interface{}) error
	constraintsProvider   func(envName string, current DeliveryConfig) []interface{}
	notificationsProvider func(envName string, current DeliveryConfig) []interface{}
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
		constraintsProvider: func(_ string, current DeliveryConfig) []interface{} {
			return []interface{}{DefaultEnvironmentConstraint}
		},
		notificationsProvider: func(_ string, current DeliveryConfig) []interface{} {
			return []interface{}{}
		},
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

// WithConstraintsProvider is a ProcessorOption to allow customizing how a default
// environment constraint is generated for newly created environments.
func WithConstraintsProvider(cp func(envName string, current DeliveryConfig) []interface{}) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		if cp != nil {
			p.constraintsProvider = cp
		}
	}
}

// WithNotificationsProvider is a ProcessorOption to allow customizing how a default
// environment notification is generated for newly created environments.
func WithNotificationsProvider(np func(envName string, current DeliveryConfig) []interface{}) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		if np != nil {
			p.notificationsProvider = np
		}
	}
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
	var err error
	p.content, err = ioutil.ReadFile(deliveryFile)
	if err != nil {
		return stacktrace.Propagate(err, "failed to read %s", deliveryFile)
	}

	err = yaml.Unmarshal(p.content, &p.rawDeliveryConfig)
	if err != nil {
		return stacktrace.Propagate(
			ErrorInvalidContent{Content: p.content, ParseError: err},
			"Failed to parse contents of %s as yaml", deliveryFile,
		)
	}

	err = yaml.Unmarshal(p.content, &p.deliveryConfig)
	if err != nil {
		return stacktrace.Propagate(
			ErrorInvalidContent{Content: p.content, ParseError: err},
			"Failed to parse contents of %s as yaml", deliveryFile,
		)
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
	p.content = output

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
	for eix := range p.deliveryConfig.Environments {
		rix := p.findResourceIndex(resource, eix)
		if rix >= 0 {
			return p.deliveryConfig.Environments[eix].Name
		}
	}
	return ""
}

// UpsertResource will update (if exists) or insert (if new) a resource into the delivery config.  The resource will
// be added to the environment that corresponds to envName if the resource is new.
func (p *DeliveryConfigProcessor) UpsertResource(resource *ExportableResource, envName string, content []byte) (added bool, err error) {
	data, err := p.bytesToData(content)
	if err != nil {
		return false, stacktrace.Propagate(err, "failed to parse content")
	}
	// fixup imageProvider.reference [ec2/cluster] or container.reference [titus/cluster] to a match artifacts
	if dataMap, ok := data.(map[string]interface{}); ok {
		if spec, ok := dataMap["spec"].(map[string]interface{}); ok {
			if imageProvider, ok := spec["imageProvider"].(map[string]interface{}); ok {
				// this is an ec2/cluster, check for reference
				if _, ok := imageProvider["reference"]; !ok {
					// need to add a reference
					for _, current := range p.deliveryConfig.Artifacts {
						if current.Name == imageProvider["name"].(string) && current.Type == DebianArtifactType {
							spec["imageProvider"] = map[string]interface{}{
								"reference": current.RefName(),
							}
							break
						}
					}
				}
			} else if container, ok := spec["container"].(map[string]interface{}); ok {
				// this is an titus/cluster, check for reference
				if _, ok := container["reference"]; !ok {
					// need to add a reference
					name := fmt.Sprintf("%s/%s", container["organization"], container["image"])
					for _, current := range p.deliveryConfig.Artifacts {
						if current.Name == name && current.Type == DockerArtifactType {
							spec["container"] = map[string]interface{}{
								"reference": current.RefName(),
							}
							break
						}
					}
				}
			}
			dataMap["spec"] = spec
		}
		data = dataMap
	}

	envIx := p.findEnvIndex(envName)
	if environments, ok := p.rawDeliveryConfig["environments"].([]interface{}); !ok || envIx < 0 {
		// new environment
		environments = append(environments, map[string]interface{}{
			"name":          envName,
			"constraints":   p.constraintsProvider(envName, p.deliveryConfig),
			"notifications": p.notificationsProvider(envName, p.deliveryConfig),
			"resources":     []interface{}{data},
		})
		p.rawDeliveryConfig["environments"] = environments
		// update in memory struct in case we look for this environment again later
		p.deliveryConfig.Environments = append(p.deliveryConfig.Environments, DeliveryEnvironment{
			Name:      envName,
			Resources: []DeliveryResource{{}},
		})
		added = true
	} else if env, ok := environments[envIx].(map[string]interface{}); ok {
		if _, ok := env["constraints"].([]interface{}); !ok {
			env["constraints"] = p.constraintsProvider(envName, p.deliveryConfig)
		}
		if _, ok := env["notifications"].([]interface{}); !ok {
			env["notifications"] = p.notificationsProvider(envName, p.deliveryConfig)
		}
		if resources, ok := env["resources"].([]interface{}); ok {
			resourceIx := p.findResourceIndex(resource, envIx)
			if resourceIx < 0 {
				resources = append(resources, data)
				added = true
			} else {
				resources[resourceIx] = data
			}
			env["resources"] = resources
			environments[envIx] = env
			p.rawDeliveryConfig["environments"] = environments
		}
	}
	return added, nil
}

func (p *DeliveryConfigProcessor) bytesToData(content []byte) (data interface{}, err error) {
	err = p.yamlUnmarshal(content, &data)
	if err != nil {
		return nil, stacktrace.Propagate(ErrorInvalidContent{Content: content, ParseError: err}, "")
	}
	return data, nil
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
		if resource.Match(search) {
			return ix
		}
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
func (p *DeliveryConfigProcessor) InsertArtifact(artifact *DeliveryArtifact) (added bool) {
	// TODO change this to detect changes in artifacts, not simple equality.  If
	// two artifacts have same refname but different values, then this is likely
	// an update operation
	for _, current := range p.deliveryConfig.Artifacts {
		if current.Equal(artifact) {
			return false
		}
	}
	p.deliveryConfig.Artifacts = append(p.deliveryConfig.Artifacts, *artifact)
	p.rawDeliveryConfig["artifacts"] = p.deliveryConfig.Artifacts
	return true
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

	_, err = commonRequest(cli, "POST", "/managed/delivery-configs", requestBody{
		Content:     bytes.NewReader(encoded),
		ContentType: "application/json",
	})

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
	if len(p.content) == 0 {
		err := p.Load()
		if err != nil {
			return nil, stacktrace.Propagate(err, "Failed to load delivery config")
		}
	}

	content, err := commonRequest(cli, "POST", "/managed/delivery-configs/diff", requestBody{
		Content:     bytes.NewReader(p.content),
		ContentType: "application/x-yaml",
	})

	if err != nil {
		return nil, stacktrace.Propagate(err, "Failed to diff delivery config with spinnaker")
	}

	data := []struct {
		ResourceDiffs []*ManagedResourceDiff `json:"resourceDiffs" yaml:"resourceDiffs"`
	}{}

	err = json.Unmarshal(content, &data)
	if err != nil {
		return nil, stacktrace.Propagate(
			ErrorInvalidContent{Content: content, ParseError: err},
			"failed to parse response from diff api",
		)
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
	_, err := commonRequest(cli, "DELETE", fmt.Sprintf("/managed/delivery-configs/%s", p.deliveryConfig.Name), requestBody{})
	return err
}

// ValidationErrorDetail is the structure of the document from /managed/delivery-configs/validate API
type ValidationErrorDetail struct {
	Error    string `json:"error"`
	Location struct {
		Column int `json:"column"`
		Line   int `json:"line"`
	} `json:"location"`
	Message        string `json:"message"`
	PathExpression string `json:"pathExpression"`
}

// Validate posts the delivery config to the validation api and returns nil on success,
// or a ValidationErrorDetail
func (p *DeliveryConfigProcessor) Validate(cli *Client) (*ValidationErrorDetail, error) {
	if len(p.content) == 0 {
		err := p.Load()
		if err != nil {
			return nil, stacktrace.Propagate(err, "Failed to load delivery config")
		}
	}

	_, err := commonRequest(cli, "POST", "/managed/delivery-configs/validate", requestBody{
		Content:     bytes.NewReader(p.content),
		ContentType: "application/x-yaml",
	})

	if err != nil {
		if errResp, ok := stacktrace.RootCause(err).(ErrorUnexpectedResponse); ok {
			if errResp.StatusCode == http.StatusBadRequest {
				validation := struct {
					Details ValidationErrorDetail `json:"details"`
				}{}
				return &validation.Details, stacktrace.Propagate(
					errResp.Parse(&validation),
					"Failed to parse response from /managed/delivery-configs/validate",
				)
			}
		}
		return nil, stacktrace.Propagate(err, "Failed to validate delivery config to spinnaker")
	}

	return nil, nil
}
