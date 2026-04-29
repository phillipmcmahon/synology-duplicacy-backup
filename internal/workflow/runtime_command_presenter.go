package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RuntimeCommandPresenter renders dry-run command text from the data-only Plan.
// It keeps operator-facing command strings out of planning so the planner only
// derives state and the runtime presentation layer owns how that state is shown.
type RuntimeCommandPresenter struct {
	plan *Plan
}

func NewRuntimeCommandPresenter(plan *Plan) RuntimeCommandPresenter {
	return RuntimeCommandPresenter{plan: plan}
}

func (p RuntimeCommandPresenter) SnapshotCreate() string {
	return fmt.Sprintf("btrfs subvolume snapshot -r %s %s", p.plan.Paths.SnapshotSource, p.plan.Paths.SnapshotTarget)
}

func (p RuntimeCommandPresenter) SnapshotDelete() string {
	return fmt.Sprintf("btrfs subvolume delete %s", p.plan.Paths.SnapshotTarget)
}

func (p RuntimeCommandPresenter) WorkDirCreate() string {
	return fmt.Sprintf("mkdir -p %s", filepath.Join(p.plan.Paths.DuplicacyRoot, ".duplicacy"))
}

func (p RuntimeCommandPresenter) PreferencesWrite() string {
	return fmt.Sprintf("write JSON preferences to %s", filepath.Join(p.plan.Paths.DuplicacyRoot, ".duplicacy", "preferences"))
}

func (p RuntimeCommandPresenter) FiltersWrite() string {
	return fmt.Sprintf("write filters to %s", filepath.Join(p.plan.Paths.DuplicacyRoot, ".duplicacy", "filters"))
}

func (p RuntimeCommandPresenter) WorkDirDirPerms() string {
	return fmt.Sprintf("find %s -type d -exec chmod 770 {} +", p.plan.Paths.DuplicacyRoot)
}

func (p RuntimeCommandPresenter) WorkDirFilePerms() string {
	return fmt.Sprintf("find %s -type f -exec chmod 660 {} +", p.plan.Paths.DuplicacyRoot)
}

func (p RuntimeCommandPresenter) Backup() string {
	return fmt.Sprintf("duplicacy backup -stats -threads %d", p.plan.Config.Threads)
}

func (p RuntimeCommandPresenter) ValidateRepo() string {
	return "duplicacy list -files"
}

func (p RuntimeCommandPresenter) PrunePreview() string {
	args := strings.TrimSpace(p.plan.Config.PruneArgsDisplay)
	if args == "" {
		return "duplicacy prune -dry-run"
	}
	return fmt.Sprintf("duplicacy prune %s -dry-run", args)
}

func (p RuntimeCommandPresenter) PolicyPrune() string {
	args := strings.TrimSpace(p.plan.Config.PruneArgsDisplay)
	if args == "" {
		return "duplicacy prune"
	}
	return fmt.Sprintf("duplicacy prune %s", args)
}

func (p RuntimeCommandPresenter) CleanupStorage() string {
	return "duplicacy prune -exhaustive -exclusive"
}

func (p RuntimeCommandPresenter) WorkDirRemove(workRoot string) string {
	if workRoot == "" {
		workRoot = p.plan.Paths.WorkRoot
	}
	return fmt.Sprintf("rm -rf %s", workRoot)
}
