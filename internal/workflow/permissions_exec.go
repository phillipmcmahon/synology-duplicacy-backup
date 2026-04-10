package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/permissions"
)

func (e *Executor) runFixPermsPhase() error {
	start := e.rt.Now()
	e.view.PrintPhase("Fix permissions")
	e.log.PrintLine("Target", e.plan.BackupTarget)
	stopApplying := e.view.StartStatusActivity("Applying ownership and permissions")
	if e.plan.Verbose {
		e.log.PrintLine("Local Owner", e.plan.LocalOwner)
		e.log.PrintLine("Local Group", e.plan.LocalGroup)
	}

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.FixPermsChownCommand)
		e.log.DryRun("%s", e.plan.FixPermsDirPermsCommand)
		e.log.DryRun("%s", e.plan.FixPermsFilePermsCommand)
		stopApplying()
		e.log.Info("%s", statusLinef("Fix permissions phase completed (dry-run)"))
		return nil
	}

	if err := permissions.Fix(e.runner, e.plan.BackupTarget, e.plan.LocalOwner, e.plan.LocalGroup, false); err != nil {
		stopApplying()
		return err
	}
	stopApplying()
	e.view.PrintDuration(start)
	e.log.Info("%s", statusLinef("Fix permissions phase completed successfully"))
	return nil
}
