package mdlib

import "time"

// Actuation plan types
type ActuationPlan struct {
	Application      string            `json:"application" yaml:"application"`
	UpdatedAt        time.Time         `json:"updatedAt" yaml:"updatedAt"`
	EnvironmentPlans []EnvironmentPlan `json:"environmentPlans" yaml:"environmentPlans"`
	Errors           []string          `json:"errors" yaml:"errors"`
}

type EnvironmentPlan struct {
	Environment   string         `json:"environment" yaml:"environment"`
	ResourcePlans []ResourcePlan `json:"resourcePlans" yaml:"resourcePlans"`
}

type ResourceAction string // TODO: make this an enum?

type ResourcePlan struct {
	Environment         string                        `json:"environment" yaml:"environment"`
	ResourceId          string                        `json:"resourceId" yaml:"resourceId"`
	ResourceDisplayName string                        `json:"resourceDisplayName" yaml:"resourceDisplayName"`
	IsManaged           bool                          `json:"isManaged" yaml:"isManaged"`
	IsPaused            bool                          `json:"isPaused" yaml:"isPaused"`
	Action              ResourceAction                `json:"action" yaml:"action"`
	Diff                map[string]SingleResourceDiff `json:"diff" yaml:"diff"`
}

type SingleResourceDiff struct {
	Type    string `json:"type" yaml:"type"`
	Desired string `json:"desired" yaml:"desired"`
	Current string `json:"current" yaml:"current"`
}
