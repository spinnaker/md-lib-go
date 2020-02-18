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

// GetSecurityGroups populates the security groups result structure for spinnaker account provided..
// Unless a custom result type is required, *SecurityGroups is recommended.
func GetSecurityGroups(cli *Client, account string, result interface{}) error {
	return commonParsedGet(cli, fmt.Sprintf("/securityGroups/%s", account), result)
}
