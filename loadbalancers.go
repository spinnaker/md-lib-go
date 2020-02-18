package mdlib

import (
	"fmt"
)

// LoadBalancerTargetGroup contains the name of the target group for an ALB
type LoadBalancerTargetGroup struct {
	Name string `json:"name"`
}

// LoadBalancerServerGroup contains the server groups for an ELB
type LoadBalancerServerGroup struct {
	Name string `json:"name"`
}

// LoadBalancer contains details about a load balancer, TargetGroups
// will be populated if it is an ALB.  ServerGroups will be populated
// if it is an ELB.
type LoadBalancer struct {
	Name           string                    `json:"name"`
	Account        string                    `json:"account"`
	Region         string                    `json:"region"`
	Type           string                    `json:"type"`
	SecurityGroups []string                  `json:"securityGroups"`
	ServerGroups   []LoadBalancerServerGroup `json:"serverGroups"`
	TargetGroups   []LoadBalancerTargetGroup `json:"targetGroups"`
}

// GetLoadBalancers populates the load balancers result structure for spinnaker application appName.
// Unless a custom result type is required, *[]LoadBalancer is recommended.
func GetLoadBalancers(cli *Client, appName string, result interface{}) error {
	return commonParsedGet(cli, fmt.Sprintf("/applications/%s/loadBalancers", appName), result)
}
