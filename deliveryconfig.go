package mdlib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/coryb/walky"
	"golang.org/x/xerrors"
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
	Artifacts    []*DeliveryArtifact
	Environments []*DeliveryEnvironment
}

// DeliveryEnvironment contains the resources per environment.
type DeliveryEnvironment struct {
	Name      string
	Locations DeliveryResourceLocations
	Resources []*DeliveryResource
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
	From struct {
		Branch struct {
			Name       string `json:"name,omitempty" yaml:"name,omitempty"`
			StartsWith string `json:"startsWith,omitempty" yaml:"startsWith,omitempty"`
			Regex      string `json:"regex,omitempty" yaml:"regex,omitempty"`
		} `json:"branch,omitempty" yaml:"branch,omitempty"`
		PullRequestOnly bool `json:"pullRequestOnly,omitempty" yaml:"pullRequestOnly,omitempty"`
	} `json:"from,omitempty" yaml:"from,omitempty"`
}

// RefName returns the Reference value for comparisons.  it will use the
// Reference value if defined, otherwise default to the Name value.
func (a *DeliveryArtifact) RefName() string {
	if a.Reference != "" {
		return a.Reference
	}
	return a.Name
}

// Equal compares the artifact TagVersionStrategy and VMOptions
func (a *DeliveryArtifact) Equal(b *DeliveryArtifact) bool {
	// note we ignore the `Name` property when comparing equality
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

// CloudProvider returns the cloud provider for a resource.  Currently it
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

// Match will return true if the ExportableResource matches the
// Kind, CloudProvider, Account and Name properties
func (r *DeliveryResource) Match(e *ExportableResource) bool {
	if e.HasKind(r.Kind) &&
		r.CloudProvider() == e.CloudProvider &&
		r.Account() == e.Account &&
		r.Name() == e.Name {
		return true
	}
	return false
}

// ResourceType will return the inferred type from the
// Kind property, `ec2/cluster@v1` will return `cluster`
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
	Regions []DeliveryResourceLocationRegion
}

// Empty will return true if the DeliveryResourceLocations has no values set
func (l DeliveryResourceLocations) Empty() bool {
	return l.Account == "" && len(l.Regions) == 0
}

// DeliveryResourceLocationRegion contains the region name
type DeliveryResourceLocationRegion struct {
	Name string
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
	fileName              string
	dirName               string
	rawDeliveryConfig     *yaml.Node
	deliveryConfig        DeliveryConfig
	content               []byte
	yamlMarshal           func(interface{}) ([]byte, error)
	yamlUnmarshal         func([]byte, interface{}) error
	constraintsProvider   func(envName string, current DeliveryConfig) []interface{}
	notificationsProvider func(envName string, current DeliveryConfig) []interface{}
	verifyWithProvider    func(envName string, current DeliveryConfig) []interface{}
	postDeployProvider    func(envName string, current DeliveryConfig) []interface{}
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
		verifyWithProvider: func(_ string, current DeliveryConfig) []interface{} {
			return []interface{}{}
		},
		postDeployProvider: func(_ string, current DeliveryConfig) []interface{} {
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

// WithVerifyProvider is a ProcessorOption to allow customizing how a
// environment verification is generated
func WithVerifyProvider(vp func(envName string, current DeliveryConfig) []interface{}) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		if vp != nil {
			p.verifyWithProvider = vp
		}
	}
}

// WithPostDeployProvider is a ProcessorOption to allow customizing how a
// post deploy action is generated
func WithPostDeployProvider(vp func(envName string, current DeliveryConfig) []interface{}) ProcessorOption {
	return func(p *DeliveryConfigProcessor) {
		if vp != nil {
			p.postDeployProvider = vp
		}
	}
}

// Load will load the delivery config files from disk.
func (p *DeliveryConfigProcessor) Load() error {
	p.deliveryConfig = DeliveryConfig{}

	deliveryFile := filepath.Join(p.dirName, p.fileName)
	if _, err := os.Stat(deliveryFile); err != nil && os.IsNotExist(err) {
		// file does not exist, skip
		p.rawDeliveryConfig = walky.NewDocumentNode()
		p.rawDeliveryConfig.Content = append(
			p.rawDeliveryConfig.Content,
			walky.NewMappingNode(),
		)
		return nil
	} else if err != nil {
		return xerrors.Errorf("failed to stat %s: %w", deliveryFile, err)
	}
	var err error
	p.content, err = ioutil.ReadFile(deliveryFile)
	if err != nil {
		return xerrors.Errorf("failed to read %s: %w", deliveryFile, err)
	}

	p.rawDeliveryConfig = &yaml.Node{}
	err = yaml.Unmarshal(p.content, p.rawDeliveryConfig)
	if err != nil {
		return xerrors.Errorf(
			"Failed to parse contents of %s as yaml: %w", deliveryFile,
			ErrorInvalidContent{Content: p.content, ParseError: err},
		)
	}

	err = yaml.Unmarshal(p.content, &p.deliveryConfig)
	if err != nil {
		return xerrors.Errorf(
			"Failed to parse contents of %s as yaml: %w", deliveryFile,
			ErrorInvalidContent{Content: p.content, ParseError: err},
		)
	}
	return nil
}

// Save will serialize the delivery config to disk.
func (p *DeliveryConfigProcessor) Save() error {
	log.Printf("Saving")
	if ok := walky.HasKey(p.rawDeliveryConfig, "application"); !ok && p.appName != "" {
		keyNode, _ := walky.ToNode("application")
		appNode, _ := walky.ToNode(p.appName)
		walky.AssignMapNode(p.rawDeliveryConfig, keyNode, appNode)
	}
	// ensure if no artifacts are present then we set it to an empty list, it is
	// required by the API
	if !walky.HasKey(p.rawDeliveryConfig, "artifacts") {
		keyNode, _ := walky.ToNode("artifacts")
		valNode := walky.NewSequenceNode()
		walky.AssignMapNode(p.rawDeliveryConfig, keyNode, valNode)
	}

	environmentsNode := walky.GetKey(p.rawDeliveryConfig, "environments")
	for envIx, envNode := range environmentsNode.Content {
		resourcesNode := walky.GetKey(envNode, "resources")
		if resourcesNode != nil {
			for ix, resourceNode := range resourcesNode.Content {
				kindNode := walky.GetKey(resourceNode, "kind")
				if kindNode != nil {
					resource := p.deliveryConfig.Environments[envIx].Resources[ix]
					kindNode.LineComment = fmt.Sprintf("%s/%s", resource.Spec.Moniker.String(), resource.Spec.Locations.Account)
				}
			}
		}
	}

	output, err := p.yamlMarshal(p.rawDeliveryConfig)
	if err != nil {
		return xerrors.Errorf("unmarshal delivery config YAML: %w", err)
	}

	// convert rawDeliveryConfig to yaml.Node so we can do custom sorting
	root := yaml.Node{}
	err = p.yamlUnmarshal(output, &root)
	if err != nil {
		return xerrors.Errorf("convert delivery config to yaml.Node: %w", err)
	}
	// then walk yaml.Node and sort fields to have high-priority fields on top
	configKeySort(&root)

	output, err = p.yamlMarshal(&root)
	if err != nil {
		return xerrors.Errorf("marshal sorted delivery config: %w", err)
	}

	p.content = output

	err = os.MkdirAll(p.dirName, 0755)
	if err != nil {
		return xerrors.Errorf("failed to create directory %s: %w", p.dirName, err)
	}

	deliveryFile := filepath.Join(p.dirName, p.fileName)

	log.Printf("Writing to %s", deliveryFile)
	err = ioutil.WriteFile(deliveryFile, output, 0644)
	if err != nil {
		return xerrors.Errorf("write delivery file: %w", err)
	}
	return nil
}

// ConfigKeySortPriority is used to sort the maps in the delivery config yaml
// document so "important" fields appear near the top
var ConfigKeySortPriority = []string{
	"kind",
	"name",
	"type",
	"moniker",
	"artifactReference",
	"container",
	"locations",
	"application",
	"artifacts",
	"environments",
}

func configKeySort(node *yaml.Node) {
	switch node.Kind {
	case yaml.MappingNode:
		start := 0
		for _, key := range ConfigKeySortPriority {
			for i := start; i < len(node.Content); i += 2 {
				if node.Content[i].Value == key {
					if i == start {
						start += 2
						break
					}
					node.Content[start], node.Content[start+1], node.Content[i], node.Content[i+1] = node.Content[i], node.Content[i+1], node.Content[start], node.Content[start+1]
					start += 2
					break
				}
			}
		}
		// crude bubble sort for the remaining non-prioritized map keys
		swapped := true
		for swapped {
			swapped = false
			for i := start + 2; i < len(node.Content); i += 2 {
				if node.Content[i-2].Value > node.Content[i].Value {
					node.Content[i-2], node.Content[i-1], node.Content[i], node.Content[i+1] = node.Content[i], node.Content[i+1], node.Content[i-2], node.Content[i-1]
					swapped = true
				}
			}
		}
		for i := 0; i < len(node.Content); i += 2 {
			// recursive sort map values
			configKeySort(node.Content[i+1])
		}
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, node := range node.Content {
			configKeySort(node)
		}
	}

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
		return false, xerrors.Errorf("failed to parse content: %w", err)
	}
	deliveryResource := &DeliveryResource{}
	err = yaml.Unmarshal(content, &deliveryResource)
	if err != nil {
		return false, xerrors.Errorf("unmarshal delivery resource: %w", ErrorInvalidContent{Content: content, ParseError: err})
	}

	envIx := p.findEnvIndex(envName)
	envsNode := walky.GetKey(p.rawDeliveryConfig, "environments")

	if envsNode == nil || envIx < 0 {
		// new environment
		keyNode, _ := walky.ToNode("environments")
		if envsNode == nil {
			envsNode = walky.NewSequenceNode()
		}
		newEnvNode, err := walky.ToNode(map[string]interface{}{
			"name":          envName,
			"constraints":   p.constraintsProvider(envName, p.deliveryConfig),
			"notifications": p.notificationsProvider(envName, p.deliveryConfig),
			"resources":     []interface{}{data},
			"verifyWith":    p.verifyWithProvider(envName, p.deliveryConfig),
			"postDeploy":    p.postDeployProvider(envName, p.deliveryConfig),
		})
		if err != nil {
			return false, xerrors.Errorf("convert to node: %w", err)
		}
		err = walky.AppendNode(envsNode, newEnvNode)
		if err != nil {
			return false, xerrors.Errorf("append node: %w", err)
		}
		err = walky.AssignMapNode(p.rawDeliveryConfig, keyNode, envsNode)
		if err != nil {
			return false, xerrors.Errorf("assign map node: %w", err)
		}
		// update in memory struct in case we look for this environment again later
		p.deliveryConfig.Environments = append(p.deliveryConfig.Environments, &DeliveryEnvironment{
			Name:      envName,
			Resources: []*DeliveryResource{deliveryResource},
		})
		added = true
	} else if len(envsNode.Content) > envIx {
		envNode := envsNode.Content[envIx]
		if !walky.HasKey(envNode, "constraints") {
			keyNode, _ := walky.ToNode("constraints")
			valNode, err := walky.ToNode(p.constraintsProvider(envName, p.deliveryConfig))
			if err != nil {
				return false, xerrors.Errorf("convert to node: %w", err)
			}
			err = walky.AssignMapNode(envNode, keyNode, valNode)
			if err != nil {
				return false, xerrors.Errorf("assign map node: %w", err)
			}
		}
		if !walky.HasKey(envNode, "notifications") {
			keyNode, _ := walky.ToNode("notifications")
			valNode, err := walky.ToNode(p.notificationsProvider(envName, p.deliveryConfig))
			if err != nil {
				return false, xerrors.Errorf("convert to node: %w", err)
			}
			err = walky.AssignMapNode(envNode, keyNode, valNode)
			if err != nil {
				return false, xerrors.Errorf("assign map node: %w", err)
			}
		}
		resourcesNode := walky.GetKey(envNode, "resources")
		if resourcesNode != nil {
			resourceIx := p.findResourceIndex(resource, envIx)
			dataNode, err := walky.ToNode(data)
			if err != nil {
				return false, xerrors.Errorf("convert to node: %w", err)
			}
			if resourceIx < 0 {
				p.deliveryConfig.Environments[envIx].Resources = append(p.deliveryConfig.Environments[envIx].Resources, deliveryResource)
				walky.AppendNode(resourcesNode, dataNode)
				added = true
			} else {
				p.deliveryConfig.Environments[envIx].Resources[resourceIx] = deliveryResource
				resourcesNode.Content[resourceIx] = dataNode
			}
		}
		keyNode, _ := walky.ToNode("verifyWith")
		veryWithNode, err := walky.ToNode(p.verifyWithProvider(envName, p.deliveryConfig))
		if err != nil {
			return false, xerrors.Errorf("convert to node: %w", err)
		}
		walky.AssignMapNode(envNode, keyNode, veryWithNode) // overwrite previous config

		keyNode, _ = walky.ToNode("postDeploy")
		postDeployNode, err := walky.ToNode(p.postDeployProvider(envName, p.deliveryConfig))
		if err != nil {
			return false, xerrors.Errorf("convert to node: %w", err)
		}
		walky.AssignMapNode(envNode, keyNode, postDeployNode) // overwrite previous config
	}
	return added, nil
}

func (p *DeliveryConfigProcessor) bytesToData(content []byte) (data interface{}, err error) {
	err = p.yamlUnmarshal(content, &data)
	if err != nil {
		return nil, xerrors.Errorf("unmarshal delivery config: %w", ErrorInvalidContent{Content: content, ParseError: err})
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
		// inherit the location from the env if the resource location is empty
		if resource.Spec.Locations.Empty() {
			resource.Spec.Locations = p.deliveryConfig.Environments[envIx].Locations
		}
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
func (p *DeliveryConfigProcessor) InsertArtifact(artifact *DeliveryArtifact) (added bool, updatedRef string) {
	// TODO change this to detect changes in artifacts, not simple equality.  If
	// two artifacts have same refname but different values, then this is likely
	// an update operation
	collision := false
	for _, current := range p.deliveryConfig.Artifacts {
		if current.Equal(artifact) {
			if current.RefName() == artifact.RefName() {
				return false, ""
			}
			artifact.Reference = current.RefName()
			return false, artifact.Reference
		}
		// contents don't match but have the same ref name, so we
		// need to rename the current ref
		if current.RefName() == artifact.RefName() {
			collision = true
		}
	}
	if collision {
		artifact.Reference = fmt.Sprintf("%s-%d", artifact.RefName(), time.Now().UnixNano())
		updatedRef = artifact.Reference
	}
	p.deliveryConfig.Artifacts = append(p.deliveryConfig.Artifacts, artifact)
	artifactsNode := walky.GetKey(p.rawDeliveryConfig, "artifacts")
	if artifactsNode == nil {
		artifactsNode = walky.NewSequenceNode()
		keyNode, _ := walky.ToNode("artifacts")
		walky.AssignMapNode(p.rawDeliveryConfig, keyNode, artifactsNode)
	}
	artifactNode, _ := walky.ToNode(artifact)
	walky.AppendNode(artifactsNode, artifactNode)
	return true, updatedRef
}

// UpdateArtifactReference will update the artifact reference in the delivery config
func (p *DeliveryConfigProcessor) UpdateArtifactReference(content *[]byte, updatedRef string) error {
	if content == nil {
		return xerrors.New("cannot update nil content for artifact reference")
	}
	data := map[string]interface{}{}
	err := yaml.Unmarshal(*content, &data)
	if err != nil {
		return xerrors.Errorf("Failed to parse resource as yaml to updated reference: %w", err)
	}
	if kind, ok := data["kind"].(string); ok {
		switch {
		case strings.HasPrefix(kind, "titus/cluster@v1"):
			// kind: titus/cluster@v1
			// spec:
			//   container:
			//     reference: some/image
			if spec, ok := data["spec"].(map[string]interface{}); ok {
				if container, ok := spec["container"].(map[string]interface{}); ok {
					container["reference"] = updatedRef
					spec["container"] = container
					data["spec"] = spec
				} else {
					return xerrors.New("resource for titus/cluster@v1 missing spec.cluster property")
				}
			} else {
				return xerrors.New("resource for titus/cluster@v1 missing spec property")
			}
		case kind == "ec2/cluster@v1":
			// kind: ec2/cluster@v1
			// spec:
			//   imageProvider:
			//     reference: some-deb
			if spec, ok := data["spec"].(map[string]interface{}); ok {
				if imageProvider, ok := spec["imageProvider"].(map[string]interface{}); ok {
					imageProvider["reference"] = updatedRef
					spec["imageProvider"] = imageProvider
					data["spec"] = spec
				} else {
					return xerrors.New("resource for ec2/cluster@v1 missing spec.imageProvider property")
				}
			} else {
				return xerrors.New("resource for ec2/cluster@v1 missing spec property")
			}
		case kind == "ec2/cluster@v1.1":
			// kind: ec2/cluster@v1.1
			// spec:
			//   artifactReference: some-deb
			if spec, ok := data["spec"].(map[string]interface{}); ok {
				spec["artifactReference"] = updatedRef
			} else {
				return xerrors.New("resource for ec2/cluster@v1 missing spec property")
			}
		default:
			return xerrors.Errorf("cannot update artifact reference for unexpected kind: %q", kind)
		}
	}
	updated, err := yaml.Marshal(&data)
	if err != nil {
		return xerrors.Errorf("Failed to marshal resource for updated artifact reference: %w", err)
	}
	*content = updated
	return nil
}

// Publish will post the delivery config to the Spinnaker API so that Spinnaker
// will update the Managed state for the application.
func (p *DeliveryConfigProcessor) Publish(cli *Client, force bool) error {
	if p.rawDeliveryConfig == nil {
		err := p.Load()
		if err != nil {
			return xerrors.Errorf("Failed to load delivery config: %w", err)
		}
	}

	_, err := commonRequest(cli, "POST", fmt.Sprintf("/managed/delivery-configs?force=%t", force), requestBody{
		Content:     bytes.NewReader(p.content),
		ContentType: "application/x-yaml",
	})

	if err != nil {
		return xerrors.Errorf("Failed to post delivery config to spinnaker: %w", err)
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
			return nil, xerrors.Errorf("Failed to load delivery config: %w", err)
		}
	}

	content, err := commonRequest(cli, "POST", "/managed/delivery-configs/diff", requestBody{
		Content:     bytes.NewReader(p.content),
		ContentType: "application/x-yaml",
	})

	if err != nil {
		return nil, xerrors.Errorf("Failed to diff delivery config with spinnaker: %w", err)
	}

	data := []struct {
		ResourceDiffs []*ManagedResourceDiff `json:"resourceDiffs" yaml:"resourceDiffs"`
	}{}

	err = json.Unmarshal(content, &data)
	if err != nil {
		return nil, xerrors.Errorf(
			"failed to parse response from diff api: %w",
			ErrorInvalidContent{Content: content, ParseError: err},
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
			return xerrors.Errorf("Failed to load delivery config: %w", err)
		}
	}
	_, err := commonRequest(cli, "DELETE", "/managed/delivery-configs/"+p.deliveryConfig.Name, requestBody{})
	return err
}

// ValidationErrorDetail is the structure of the document from /managed/delivery-configs/validate API
type ValidationErrorDetail struct {
	Error   string `json:"error"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// Validate posts the delivery config to the validation api and returns nil on success,
// or a ValidationErrorDetail
func (p *DeliveryConfigProcessor) Validate(cli *Client) (*ValidationErrorDetail, error) {
	if len(p.content) == 0 {
		err := p.Load()
		if err != nil {
			return nil, xerrors.Errorf("Failed to load delivery config: %w", err)
		}
	}

	_, err := commonRequest(cli, "POST", "/managed/delivery-configs/validate", requestBody{
		Content:     bytes.NewReader(p.content),
		ContentType: "application/x-yaml",
	})

	if err != nil {
		var errResp ErrorUnexpectedResponse
		if errors.As(err, &errResp) {
			if errResp.StatusCode == http.StatusBadRequest {
				validation := ValidationErrorDetail{}
				errResp.Parse(&validation)
				return &validation, xerrors.Errorf(
					"Failed to parse response from /managed/delivery-configs/validate: %w",
					errResp,
				)
			}
		}
		return nil, xerrors.Errorf("Failed to validate delivery config to spinnaker: %w", err)
	}

	return nil, nil
}
