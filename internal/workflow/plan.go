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
	LockPath    string

	OperationMode string
}

func (p *Plan) IsRemote() bool {
	return p.Request != nil && p.Request.RemoteMode
}

func (p *Plan) ModeLabel() string {
	if p.IsRemote() {
		return "REMOTE"
	}
	return "LOCAL"
}

func (p *Plan) WorkDir() string {
	if p.DuplicacyRoot != "" {
		return p.DuplicacyRoot
	}
	return filepath.Join(p.WorkRoot, "duplicacy")
}
