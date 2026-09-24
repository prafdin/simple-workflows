package domain

// Metrics receives workflow lifecycle events for monitoring.
type Metrics interface {
	WorkflowCreated()
	RunStarted()
	RunCompleted(status Status)
}
