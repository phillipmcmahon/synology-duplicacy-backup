package restore

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"

type localStoragePolicy interface {
	UsesRootProtectedLocalRepository() bool
}

func localRepositoryRequiresSudoForStorage(cfg localStoragePolicy, rt Env) bool {
	return cfg != nil && cfg.UsesRootProtectedLocalRepository() && restoreEnvEUID(rt) != 0
}

func restoreStorageRequiresSudo(plan *Plan, storage string) bool {
	return plan != nil && plan.Config.Location == "local" && duplicacy.NewStorageSpec(storage).IsLocalPath()
}

func restoreEnvEUID(rt Env) int {
	if rt.Geteuid != nil {
		return rt.Geteuid()
	}
	return 0
}
