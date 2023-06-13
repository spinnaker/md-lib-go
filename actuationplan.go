package mdlib

import "time"

// ActuationPlan describes the action MD will take to reconcile the state of the world with the desired state
type ActuationPlan struct {
	Application      string            `json:"application" yaml:"application"`
	UpdatedAt        time.Time         `json:"updatedAt" yaml:"updatedAt"`
	EnvironmentPlans []EnvironmentPlan `json:"environmentPlans" yaml:"environmentPlans"`
	Errors           []string          `json:"errors" yaml:"errors"`
}

// EnvironmentPlan describes the actions for a given environment
type EnvironmentPlan struct {
	Environment   string         `json:"environment" yaml:"environment"`
	ResourcePlans []ResourcePlan `json:"resourcePlans" yaml:"resourcePlans"`
}

// ResourceAction describes the type of operation - NONE, CREATE or UPDATE
type ResourceAction string

// ResourcePlan describes the actions for a given resource
type ResourcePlan struct {
	Environment         string               `json:"environment" yaml:"environment"`
	ResourceId          string               `json:"resourceId" yaml:"resourceId"`
	ResourceDisplayName string               `json:"resourceDisplayName" yaml:"resourceDisplayName"`
	IsManaged           bool                 `json:"isManaged" yaml:"isManaged"`
	IsPaused            bool                 `json:"isPaused" yaml:"isPaused"`
	Action              ResourceAction       `json:"action" yaml:"action"`
	Diff                []SingleResourceDiff `json:"diff" yaml:"diff"`
}

// SingleResourceDiff describes the difference between the desired and current state of a resource
type SingleResourceDiff struct {
	Field   string `json:"field" yaml:"field"`
	Type    string `json:"type" yaml:"type"`
	Desired string `json:"desired" yaml:"desired"`
	Current string `json:"current" yaml:"current"`
	Message string `json:"message" yaml:"message"`
	Prefix  string `json:"prefix" yaml:"prefix"`
}
