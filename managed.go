package mdlib

import "fmt"

// PauseManagement will cause Spinnaker to pause managing the state of the application.  Management history will be reserved and can be resumed later.
func PauseManagement(cli *Client, appName string) error {
	_, err := commonRequest(cli, "POST", fmt.Sprintf("/managed/application/%s/pause", appName), nil)
	return err
}

// ResumeManagement will cause Spinnaker to resume managing the state of the application, assuming it had been previously paused.
func ResumeManagement(cli *Client, appName string) error {
	_, err := commonRequest(cli, "DELETE", fmt.Sprintf("/managed/application/%s/pause", appName), nil)
	return err
}
