package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"

const (
	locationLocal  = workflowcore.LocationLocal
	locationRemote = workflowcore.LocationRemote
)

type Env = workflowcore.Env
type Metadata = workflowcore.Metadata
type UserProfileDirs = workflowcore.UserProfileDirs

var DefaultEnv = workflowcore.DefaultEnv
var MetadataForLogDir = workflowcore.MetadataForLogDir
var DefaultMetadataForEnv = workflowcore.DefaultMetadataForEnv
var ValidateLabel = workflowcore.ValidateLabel
var ValidateTargetName = workflowcore.ValidateTargetName
var ResolveDir = workflowcore.ResolveDir
var DefaultUserProfileDirs = workflowcore.DefaultUserProfileDirs
var HasSudoOperator = workflowcore.HasSudoOperator
var EnvEUID = workflowcore.EnvEUID
var EffectiveConfigDir = workflowcore.EffectiveConfigDir
var EffectiveSecretsDir = workflowcore.EffectiveSecretsDir
var SignalSet = workflowcore.SignalSet
