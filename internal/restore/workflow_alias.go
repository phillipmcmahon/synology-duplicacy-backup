package restore

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/operator"
	workflow "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

// These aliases are the deliberately small bridge left after extracting
// restore from workflow. Neutral request, plan, environment, and state
// primitives now come from workflowcore; only orchestration helpers remain on
// the workflow side of this bridge.
type ConfigPlanRequest = workflowcore.ConfigPlanRequest
type Metadata = workflowcore.Metadata
type Plan = workflowcore.Plan
type PlanConfig = workflowcore.PlanConfig
type PlanPaths = workflowcore.PlanPaths
type Request = workflowcore.Request
type Env = workflowcore.Env
type RunState = workflowcore.RunState
type SummaryLine = workflowcore.SummaryLine

var ErrRestoreCancelled = workflowcore.ErrRestoreCancelled
var ErrRestoreInterrupted = workflowcore.ErrRestoreInterrupted

var NewConfigPlanner = workflow.NewConfigPlanner
var NewRequestError = workflowcore.NewRequestError
var DefaultEnv = workflowcore.DefaultEnv
var DefaultMetadataForEnv = workflowcore.DefaultMetadataForEnv
var LoadRunState = workflowcore.LoadRunState
var MetadataForLogDir = workflowcore.MetadataForLogDir
var NewMessageError = operator.NewMessageError
var OperatorMessage = operator.Message
var SignalSet = workflowcore.SignalSet
var StateFilePath = workflowcore.StateFilePath
