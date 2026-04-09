package workflow

import (
	"path/filepath"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/secrets"
)

type Plan struct {
	Request *Request
	Config  *config.Config
	Secrets *secrets.Secrets

	DoBackup      bool
	DoPrune       bool
	DeepPruneMode bool
	FixPerms      bool
	FixPermsOnly  bool
	ForcePrune    bool
	RemoteMode    bool
	DryRun        bool

	NeedsDuplicacySetup bool
	NeedsSnapshot       bool

	DefaultNotice string
	ModeDisplay   string

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

func (p *Plan) IsRemote() bool {
	return p.RemoteMode
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
