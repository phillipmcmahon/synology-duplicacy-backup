package workflow

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"

// Path-based local repositories are root-protected by policy; non-root
// commands should stop before probing them through Duplicacy.
func localRepositoryRequiresSudo(cfg *config.Config, rt Runtime) bool {
	return cfg != nil && cfg.UsesLocalDiskStorage() && runtimeEUID(rt) != 0
}
