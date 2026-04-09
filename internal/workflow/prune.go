package workflow

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func (e *Executor) runPrunePhase() error {
	e.log.Info("%s", statusLinef("Starting prune phase."))

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.ValidateRepoCommand)
	} else {
		if err := e.dup.ValidateRepo(); err != nil {
			return err
		}
		e.log.Info("%s", statusLinef("Duplicacy repository validated."))
	}

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.PrunePreviewCommand)
	}
	preview, err := e.dup.SafePrunePreview(e.plan.PruneArgs, e.plan.SafePruneMinTotalForPercent)
	if err != nil {
		return err
	}

	e.logPrunePreviewOutput(preview)
	if err := e.enforcePrunePreview(preview); err != nil {
		return err
	}

	e.log.Info("%s", statusLinef("Starting policy prune."))
	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.PolicyPruneCommand)
	} else {
		stdout, stderr, err := e.dup.RunPrune(e.plan.PruneArgs)
		e.view.PrintCommandOutput(stdout, stderr)
		if err != nil {
			return err
		}
	}
	e.log.Info("%s", statusLinef("Policy prune completed."))

	if e.plan.DeepPruneMode {
		e.log.Warn("%s", statusLinef("Starting deep prune maintenance step: duplicacy prune -exhaustive -exclusive."))
		if e.plan.DryRun {
			e.log.DryRun("%s", e.plan.DeepPruneCommand)
		} else {
			stdout, stderr, err := e.dup.RunDeepPrune()
			e.view.PrintCommandOutput(stdout, stderr)
			if err != nil {
				return err
			}
		}
		e.log.Info("%s", statusLinef("Deep prune completed."))
	}

	e.log.Info("%s", statusLinef("Prune phase completed successfully."))
	return nil
}

func (e *Executor) logPrunePreviewOutput(preview *duplicacy.PrunePreview) {
	if preview.Output != "" {
		for _, line := range strings.Split(preview.Output, "\n") {
			if line != "" {
				e.log.Info("[SAFE-PRUNE-PREVIEW] %s", line)
			}
		}
	}
	if preview.RevisionOutput != "" {
		for _, line := range strings.Split(preview.RevisionOutput, "\n") {
			if line != "" {
				e.log.Info("[REVISION-LIST] %s", line)
			}
		}
	}
}

func (e *Executor) enforcePrunePreview(preview *duplicacy.PrunePreview) error {
	if preview.RevisionCountFailed {
		if e.plan.ForcePrune {
			e.log.Warn("%s", statusLinef("Revision count failed; proceeding because --force-prune was supplied (percentage threshold not enforced)."))
		} else {
			return NewMessageError("Revision count is required for safe prune but failed; use --force-prune to override.")
		}
	}

	e.view.PrintPrunePreview(preview, e.plan.SafePruneMinTotalForPercent)

	blocked := false
	if preview.DeleteCount > e.plan.SafePruneMaxDeleteCount {
		e.log.Error("%s", statusLinef("Safe prune preview exceeds delete count threshold: %d > %d.", preview.DeleteCount, e.plan.SafePruneMaxDeleteCount))
		blocked = true
	}
	if preview.ExceedsPercent(e.plan.SafePruneMaxDeletePercent) {
		e.log.Error("%s", statusLinef("Safe prune preview exceeds delete percentage threshold (%d of %d revisions > %d%%).", preview.DeleteCount, preview.TotalRevisions, e.plan.SafePruneMaxDeletePercent))
		blocked = true
	}
	if blocked {
		if e.plan.ForcePrune {
			e.log.Warn("%s", statusLinef("Proceeding despite safe prune threshold breach because --force-prune was supplied."))
		} else {
			return NewMessageError("Refusing to continue because safe prune thresholds were exceeded.")
		}
	}

	return nil
}
