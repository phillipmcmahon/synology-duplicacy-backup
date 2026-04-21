package workflow

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
)

func (e *Executor) runPrunePhase() error {
	start := e.rt.Now()
	e.view.PrintPhase("Prune")
	stopInspecting := e.view.StartStatusActivity("Inspecting repository revisions")

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.ValidateRepoCommand)
	} else {
		if err := e.dup.ValidateRepo(); err != nil {
			stopInspecting()
			return err
		}
		if e.plan.Verbose {
			e.log.PrintLine("Repository", "Validated")
		}
	}

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.PrunePreviewCommand)
	}
	preview, err := e.dup.SafePrunePreview(e.plan.PruneArgs, e.plan.SafePruneMinTotalForPercent)
	stopInspecting()
	if err != nil {
		return err
	}
	e.lastPrunePreview = preview

	e.logPrunePreviewOutput(preview)
	if err := e.enforcePrunePreview(preview); err != nil {
		return err
	}

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.PolicyPruneCommand)
		e.log.Info("%s", statusLinef("Prune phase completed (dry-run)"))
	} else {
		stopApplying := e.view.StartStatusActivity("Applying retention policy")
		stdout, stderr, err := e.dup.RunPrune(e.plan.PruneArgs)
		stopApplying()
		e.view.PrintCommandOutput(stdout, stderr, err != nil)
		if err != nil {
			return err
		}
		e.view.PrintDuration(start)
		e.log.Info("%s", statusLinef("Prune phase completed successfully"))
	}

	return nil
}

func (e *Executor) runCleanupStoragePhase() error {
	start := e.rt.Now()
	e.view.PrintPhase("Storage cleanup")
	stopScanning := e.view.StartStatusActivity("Scanning storage for unreferenced chunks")

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.ValidateRepoCommand)
		e.log.DryRun("%s", e.plan.CleanupStorageCommand)
		stopScanning()
		e.log.Info("%s", statusLinef("Storage cleanup phase completed (dry-run)"))
		return nil
	}

	if err := e.dup.ValidateRepo(); err != nil {
		stopScanning()
		return err
	}
	if e.plan.Verbose {
		e.log.PrintLine("Repository", "Validated")
	}

	stdout, stderr, err := e.dup.RunCleanupStorage()
	stopScanning()
	e.view.PrintCommandOutput(stdout, stderr, err != nil)
	if err != nil {
		return err
	}
	e.view.PrintDuration(start)
	e.log.Info("%s", statusLinef("Storage cleanup phase completed successfully"))
	return nil
}

func (e *Executor) logPrunePreviewOutput(preview *duplicacy.PrunePreview) {
	if !e.plan.Verbose {
		return
	}
	if preview.Output != "" {
		for _, line := range strings.Split(preview.Output, "\n") {
			if line != "" {
				e.log.PrintLine("Preview", line)
			}
		}
	}
}

func (e *Executor) enforcePrunePreview(preview *duplicacy.PrunePreview) error {
	if preview.RevisionCountFailed {
		if e.plan.ForcePrune {
			e.log.Warn("%s", statusLinef("Revision count failed; proceeding because prune --force was supplied (percentage threshold not enforced)"))
		} else {
			return NewMessageError("Revision count is required for safe prune but failed; use prune --force to override")
		}
	}

	e.view.PrintPrunePreview(preview, e.plan.SafePruneMinTotalForPercent)

	blocked := false
	if preview.DeleteCount > e.plan.SafePruneMaxDeleteCount {
		e.log.Error("%s", statusLinef("Safe prune preview exceeds delete count threshold: %d > %d", preview.DeleteCount, e.plan.SafePruneMaxDeleteCount))
		blocked = true
	}
	if preview.ExceedsPercent(e.plan.SafePruneMaxDeletePercent) {
		e.log.Error("%s", statusLinef("Safe prune preview exceeds delete percentage threshold (%d of %d revisions > %d%%)", preview.DeleteCount, preview.TotalRevisions, e.plan.SafePruneMaxDeletePercent))
		blocked = true
	}
	if blocked {
		if e.plan.ForcePrune {
			e.log.Warn("%s", statusLinef("Proceeding despite safe prune threshold breach because prune --force was supplied"))
		} else {
			return NewMessageError("Refusing to continue because safe prune thresholds were exceeded")
		}
	}

	return nil
}
