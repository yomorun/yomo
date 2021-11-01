package core

// Workflow describes stream function workflows.
type Workflow struct {
	// Seq represents the sequence id when executing workflows.
	Seq int

	// Token represents the name of workflow.
	Name string
}
