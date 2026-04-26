package workflow

import (
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Plan struct {
	Secrets *secrets.Secrets

	DoBackup       bool
	DoPrune        bool
	DoCleanupStore bool
	ForcePrune     bool
	DryRun         bool
	Verbose        bool
	JSONSummary    bool

	NeedsDuplicacySetup bool
	NeedsSnapshot       bool

	DefaultNotice string
	ModeDisplay   string
	Target        string
	Location      string
	Notify        config.HealthNotifyConfig

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

	SnapshotCreateCommand   string
	SnapshotDeleteCommand   string
	WorkDirCreateCommand    string
	PreferencesWriteCommand string
	FiltersWriteCommand     string
	WorkDirDirPermsCommand  string
	WorkDirFilePermsCommand string
	BackupCommand           string
	ValidateRepoCommand     string
	PrunePreviewCommand     string
	PolicyPruneCommand      string
	CleanupStorageCommand   string
	WorkDirRemoveCommand    string
}

type PlanRequest struct {
	DoBackup            bool
	DoPrune             bool
	DoCleanupStore      bool
	ForcePrune          bool
	DryRun              bool
	Verbose             bool
	JSONSummary         bool
	NeedsDuplicacySetup bool
	NeedsSnapshot       bool
	DefaultNotice       string
	OperationMode       string
}

type PlanConfig struct {
	Target                      string
	Location                    string
	Notify                      config.HealthNotifyConfig
	BackupLabel                 string
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
}

type PlanPaths struct {
	RunTimestamp   string
	SnapshotSource string
	SnapshotTarget string
	RepositoryPath string
	WorkRoot       string
	DuplicacyRoot  string
	BackupTarget   string
	ConfigDir      string
	ConfigFile     string
	SecretsDir     string
	SecretsFile    string
}

type PlanDisplay struct {
	ModeDisplay             string
	SnapshotCreateCommand   string
	SnapshotDeleteCommand   string
	WorkDirCreateCommand    string
	PreferencesWriteCommand string
	FiltersWriteCommand     string
	WorkDirDirPermsCommand  string
	WorkDirFilePermsCommand string
	BackupCommand           string
	ValidateRepoCommand     string
	PrunePreviewCommand     string
	PolicyPruneCommand      string
	CleanupStorageCommand   string
	WorkDirRemoveCommand    string
}

type PlanSections struct {
	Request PlanRequest
	Config  PlanConfig
	Paths   PlanPaths
	Display PlanDisplay
}

func (p *Plan) Sections() PlanSections {
	if p == nil {
		return PlanSections{}
	}
	return PlanSections{
		Request: PlanRequest{
			DoBackup:            p.DoBackup,
			DoPrune:             p.DoPrune,
			DoCleanupStore:      p.DoCleanupStore,
			ForcePrune:          p.ForcePrune,
			DryRun:              p.DryRun,
			Verbose:             p.Verbose,
			JSONSummary:         p.JSONSummary,
			NeedsDuplicacySetup: p.NeedsDuplicacySetup,
			NeedsSnapshot:       p.NeedsSnapshot,
			DefaultNotice:       p.DefaultNotice,
			OperationMode:       p.OperationMode,
		},
		Config: PlanConfig{
			Target:                      p.Target,
			Location:                    p.Location,
			Notify:                      p.Notify,
			BackupLabel:                 p.BackupLabel,
			Threads:                     p.Threads,
			Filter:                      p.Filter,
			FilterLines:                 append([]string(nil), p.FilterLines...),
			OwnerGroup:                  p.OwnerGroup,
			PruneOptions:                p.PruneOptions,
			PruneArgs:                   append([]string(nil), p.PruneArgs...),
			PruneArgsDisplay:            p.PruneArgsDisplay,
			LocalOwner:                  p.LocalOwner,
			LocalGroup:                  p.LocalGroup,
			LogRetentionDays:            p.LogRetentionDays,
			SafePruneMaxDeletePercent:   p.SafePruneMaxDeletePercent,
			SafePruneMaxDeleteCount:     p.SafePruneMaxDeleteCount,
			SafePruneMinTotalForPercent: p.SafePruneMinTotalForPercent,
		},
		Paths: PlanPaths{
			RunTimestamp:   p.RunTimestamp,
			SnapshotSource: p.SnapshotSource,
			SnapshotTarget: p.SnapshotTarget,
			RepositoryPath: p.RepositoryPath,
			WorkRoot:       p.WorkRoot,
			DuplicacyRoot:  p.DuplicacyRoot,
			BackupTarget:   p.BackupTarget,
			ConfigDir:      p.ConfigDir,
			ConfigFile:     p.ConfigFile,
			SecretsDir:     p.SecretsDir,
			SecretsFile:    p.SecretsFile,
		},
		Display: PlanDisplay{
			ModeDisplay:             p.ModeDisplay,
			SnapshotCreateCommand:   p.SnapshotCreateCommand,
			SnapshotDeleteCommand:   p.SnapshotDeleteCommand,
			WorkDirCreateCommand:    p.WorkDirCreateCommand,
			PreferencesWriteCommand: p.PreferencesWriteCommand,
			FiltersWriteCommand:     p.FiltersWriteCommand,
			WorkDirDirPermsCommand:  p.WorkDirDirPermsCommand,
			WorkDirFilePermsCommand: p.WorkDirFilePermsCommand,
			BackupCommand:           p.BackupCommand,
			ValidateRepoCommand:     p.ValidateRepoCommand,
			PrunePreviewCommand:     p.PrunePreviewCommand,
			PolicyPruneCommand:      p.PolicyPruneCommand,
			CleanupStorageCommand:   p.CleanupStorageCommand,
			WorkDirRemoveCommand:    p.WorkDirRemoveCommand,
		},
	}
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
