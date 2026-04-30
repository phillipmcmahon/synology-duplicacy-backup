package workflowcore

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type SummaryLine = presentation.Line

type Plan struct {
	Secrets *secrets.Secrets

	Request PlanRequest
	Config  PlanConfig
	Paths   PlanPaths

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
	StorageName                 string
	Location                    string
	Notify                      config.HealthNotifyConfig
	BackupLabel                 string
	Threads                     int
	Filter                      string
	FilterLines                 []string
	PruneOptions                string
	PruneArgs                   []string
	PruneArgsDisplay            string
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

type PlanSections struct {
	Request PlanRequest
	Config  PlanConfig
	Paths   PlanPaths
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
	}
}

func (p *Plan) IsRemoteLocation() bool {
	return p != nil && p.Config.Location == LocationRemote
}

func (p *Plan) ModeLabel() string {
	if p == nil {
		return ""
	}
	return modeDisplay(p.TargetName())
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
	return p.Config.StorageName
}

func (p *Plan) ApplyConfigIdentity(cfg *config.Config) {
	if p == nil || cfg == nil {
		return
	}
	p.Config.StorageName = cfg.StorageName
	p.Config.Location = cfg.Location
	p.Config.Notify = cfg.Health.Notify
}

// ApplyConfig projects resolved configuration values onto the orchestration
// plan after config validation has completed.
func (p *Plan) ApplyConfig(cfg *config.Config, rt Env) {
	if p == nil || cfg == nil {
		return
	}
	p.ApplyConfigIdentity(cfg)
	p.Paths.SnapshotSource = cfg.SourcePath
	p.Paths.SnapshotTarget = filepath.Join(rootVolumeForSource(cfg.SourcePath), fmt.Sprintf("%s-%s-%s-%d", p.Config.BackupLabel, p.TargetName(), p.Paths.RunTimestamp, rt.Getpid()))
	p.Paths.RepositoryPath = cfg.SourcePath
	if p.Request.DoBackup {
		p.Paths.RepositoryPath = p.Paths.SnapshotTarget
	}
	p.Paths.BackupTarget = cfg.Storage
	p.Config.Threads = cfg.Threads
	p.Config.Filter = cfg.Filter
	p.Config.FilterLines = splitNonEmptyLines(cfg.Filter)
	p.Config.PruneOptions = cfg.Prune
	p.Config.PruneArgs = append([]string(nil), cfg.PruneArgs...)
	p.Config.PruneArgsDisplay = strings.Join(cfg.PruneArgs, " ")
	p.Config.LogRetentionDays = cfg.LogRetentionDays
	p.Config.SafePruneMaxDeletePercent = cfg.SafePruneMaxDeletePercent
	p.Config.SafePruneMaxDeleteCount = cfg.SafePruneMaxDeleteCount
	p.Config.SafePruneMinTotalForPercent = cfg.SafePruneMinTotalForPercent
}

func splitNonEmptyLines(value string) []string {
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func modeDisplay(targetName string) string {
	if targetName != "" {
		return targetName
	}
	return "not supplied"
}

func rootVolumeForSource(sourcePath string) string {
	clean := filepath.Clean(sourcePath)
	if clean == "." || clean == "/" {
		return clean
	}
	if !filepath.IsAbs(clean) {
		return clean
	}
	trimmed := strings.TrimPrefix(clean, string(filepath.Separator))
	parts := strings.Split(trimmed, string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return string(filepath.Separator)
	}
	return string(filepath.Separator) + parts[0]
}
