package workflow

import (
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Plan struct {
	Secrets *secrets.Secrets

	Request PlanRequest
	Config  PlanConfig
	Paths   PlanPaths
	Display PlanDisplay

	Summary []SummaryLine
}

// These section field names are mirrored by
// scripts/check-plan-section-boundary.sh. Adding, renaming, or removing a Plan
// section field requires updating that guard's FIELDS list so the old flat Plan
// shape cannot quietly return.
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
	config := p.Config
	config.FilterLines = append([]string(nil), p.Config.FilterLines...)
	config.PruneArgs = append([]string(nil), p.Config.PruneArgs...)
	return PlanSections{
		Request: p.Request,
		Config:  config,
		Paths:   p.Paths,
		Display: p.Display,
	}
}

func (p *Plan) IsRemoteLocation() bool {
	return p != nil && p.Config.Location == locationRemote
}

func (p *Plan) ModeLabel() string {
	return p.Display.ModeDisplay
}

func (p *Plan) WorkDir() string {
	if p.Paths.DuplicacyRoot != "" {
		return p.Paths.DuplicacyRoot
	}
	return filepath.Join(p.Paths.WorkRoot, "duplicacy")
}

func (p *Plan) TargetName() string {
	if p == nil {
		return ""
	}
	return p.Config.Target
}
