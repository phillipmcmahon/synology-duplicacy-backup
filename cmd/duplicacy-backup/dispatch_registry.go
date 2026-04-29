package main

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"

type dispatchSpec struct {
	name   string
	handle func(*workflow.Request, workflow.Metadata, workflow.Env) int
}

var dispatchRegistry = map[string]dispatchSpec{
	"backup":          {name: "runtime", handle: runRuntimeRequest},
	"cleanup-storage": {name: "runtime", handle: runRuntimeRequest},
	"config":          {name: "config", handle: runConfigRequest},
	"diagnostics":     {name: "diagnostics", handle: runDiagnosticsRequest},
	"health":          {name: "health", handle: runHealthRequest},
	"notify":          {name: "notify", handle: runNotifyRequest},
	"prune":           {name: "runtime", handle: runRuntimeRequest},
	"restore":         {name: "restore", handle: runRestoreRequest},
	"rollback":        {name: "rollback", handle: runRollbackRequest},
	"update":          {name: "update", handle: runUpdateRequest},
}

func dispatchSpecForRequest(req *workflow.Request) (dispatchSpec, bool) {
	if req == nil || req.Command == "" {
		return dispatchSpec{}, false
	}
	spec, ok := dispatchRegistry[req.Command]
	return spec, ok
}
