package workflow

import (
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Plan struct {
	Secrets *secrets.Secrets

	DoBackup       bool
	DoPrune        bool
	DoCleanupStore bool
	FixPerms       bool
	FixPermsOnly   bool
	ForcePrune     bool
	DryRun         bool
	Verbose        bool
	JSONSummary    bool

	NeedsDuplicacySetup bool
	NeedsSnapshot       bool

	DefaultNotice string
	ModeDisplay   string
	Target        string
	StorageType   string
	Location      string

	BackupLabel    string
	RunTimestamp   string
	SnapshotSource string
	SnapshotTarget string
	RepositoryPath string
	WorkRoot       string
	DuplicacyRoot  string
	BackupTarget   string

	ConfigDir   string
	ConfigFile  string
	SecretsDir  string
	SecretsFile string

	OperationMode string
	Summary       []SummaryLine

	Threads                     int
	Filter                      string
	FilterLines                 []string
	OwnerGroup                  string
	PruneOptions                string
	PruneArgs                   []string
	PruneArgsDisplay            string
	LocalOwner                  string
	LocalGroup                  string
	LogRetentionDays            int
	SafePruneMaxDeletePercent   int
	SafePruneMaxDeleteCount     int
	SafePruneMinTotalForPercent int

	SnapshotCreateCommand    string
	SnapshotDeleteCommand    string
	WorkDirCreateCommand     string
	PreferencesWriteCommand  string
	FiltersWriteCommand      string
	WorkDirDirPermsCommand   string
	WorkDirFilePermsCommand  string
	BackupCommand            string
	ValidateRepoCommand      string
	PrunePreviewCommand      string
	PolicyPruneCommand       string
	CleanupStorageCommand    string
	FixPermsChownCommand     string
	FixPermsDirPermsCommand  string
	FixPermsFilePermsCommand string
	WorkDirRemoveCommand     string
}

func (p *Plan) UsesFilesystem() bool {
	return p != nil && p.StorageType == storageTypeFilesystem
}

func (p *Plan) UsesObjectStorage() bool {
	return p != nil && p.StorageType == storageTypeObject
}

func (p *Plan) IsRemoteLocation() bool {
	return p != nil && p.Location == locationRemote
}

func (p *Plan) ModeLabel() string {
	return p.ModeDisplay
}

func (p *Plan) WorkDir() string {
	if p.DuplicacyRoot != "" {
		return p.DuplicacyRoot
	}
	return filepath.Join(p.WorkRoot, "duplicacy")
}

func (p *Plan) TargetName() string {
	if p == nil {
		return ""
	}
	return p.Target
}
