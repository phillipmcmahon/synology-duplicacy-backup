package restore

import workflow "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"

// These aliases are the deliberately small bridge left after extracting
// restore from workflow. Stage 3 removes more of this coupling by replacing the
// broad command envelope with typed command requests and a command registry.
type ConfigPlanRequest = workflow.ConfigPlanRequest
type Metadata = workflow.Metadata
type Plan = workflow.Plan
type PlanConfig = workflow.PlanConfig
type PlanPaths = workflow.PlanPaths
type Request = workflow.Request
type Runtime = workflow.Runtime
type RunState = workflow.RunState
type SummaryLine = workflow.SummaryLine

var ErrRestoreCancelled = workflow.ErrRestoreCancelled
var ErrRestoreInterrupted = workflow.ErrRestoreInterrupted

var NewConfigPlanner = workflow.NewConfigPlanner
var NewRequestError = workflow.NewRequestError
var DefaultRuntime = workflow.DefaultRuntime
var DefaultMetadataForRuntime = workflow.DefaultMetadataForRuntime
var LoadRunState = workflow.LoadRunState
var MetadataForLogDir = workflow.MetadataForLogDir
var NewMessageError = workflow.NewMessageError
var OperatorMessage = workflow.OperatorMessage
var SignalSet = workflow.SignalSet
var StateFilePath = workflow.StateFilePath
