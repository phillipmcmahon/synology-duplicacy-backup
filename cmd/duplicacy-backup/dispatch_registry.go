package main

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"

type dispatchSpec struct {
	name    string
	matches func(*workflow.Request) bool
	handle  func(*workflow.Request, workflow.Metadata, workflow.Runtime) int
}

var dispatchRegistry = []dispatchSpec{
	{
		name:    "config",
		matches: func(req *workflow.Request) bool { return req != nil && req.ConfigCommand != "" },
		handle:  runConfigRequest,
	},
	{
		name:    "diagnostics",
		matches: func(req *workflow.Request) bool { return req != nil && req.DiagnosticsCommand != "" },
		handle:  runDiagnosticsRequest,
	},
	{
		name:    "notify",
		matches: func(req *workflow.Request) bool { return req != nil && req.NotifyCommand != "" },
		handle:  runNotifyRequest,
	},
	{
		name:    "restore",
		matches: func(req *workflow.Request) bool { return req != nil && req.RestoreCommand != "" },
		handle:  runRestoreRequest,
	},
	{
		name:    "rollback",
		matches: func(req *workflow.Request) bool { return req != nil && req.RollbackCommand != "" },
		handle:  runRollbackRequest,
	},
	{
		name:    "update",
		matches: func(req *workflow.Request) bool { return req != nil && req.UpdateCommand != "" },
		handle:  runUpdateRequest,
	},
	{
		name:    "health",
		matches: func(req *workflow.Request) bool { return req != nil && req.HealthCommand != "" },
		handle:  runHealthRequest,
	},
	{
		name:    "runtime",
		matches: func(*workflow.Request) bool { return true },
		handle:  runRuntimeRequest,
	},
}

func dispatchSpecForRequest(req *workflow.Request) dispatchSpec {
	for _, spec := range dispatchRegistry {
		if spec.matches(req) {
			return spec
		}
	}
	return dispatchSpec{name: "runtime", handle: runRuntimeRequest}
}
