package core

// View represents the active TUI screen.
type View int

const (
	ViewDashboard View = iota
	ViewDetail
	ViewLaunch
	ViewHistory
	ViewCreate
	ViewEditWorkflow
)
