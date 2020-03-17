package mdlib

import "fmt"

// SecurityGroup contains the relevant detail for mapping a SG id to a SG name.
type SecurityGroup struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Region is alias to string to make SecurityGroups map more clear
type Region = string

// SecurityGroups is a map from region to security groups
type SecurityGroups map[Region][]SecurityGroup

// GetSecurityGroups populates the security groups result structure for spinnaker account provided.
// Unless a custom result type is required, *SecurityGroups is recommended.
func GetSecurityGroups(cli *Client, account string, result interface{}) error {
	return commonParsedGet(cli, fmt.Sprintf("/securityGroups/%s", account), result)
}

// Credential contains account status
type Credential struct {
	PrimaryAccount bool   `json:"primaryAccount"`
	CloudProvider  string `json:"cloudProvider"`
	AWSAccount     string `json:"awsAccount"`
}

// GetCredential populates the credential result nstructure for the spinnaker accound provided.
// Unless a custom result type is required, *Credential is recommended
func GetCredential(cli *Client, account string, result interface{}) error {
	return commonParsedGet(cli, fmt.Sprintf("/credentials/%s", account), result)
}
