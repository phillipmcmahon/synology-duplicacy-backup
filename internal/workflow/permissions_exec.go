package workflow

import (
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/permissions"
)

func (e *Executor) runFixPermsPhase() error {
	e.log.Info("%s", statusLinef("Starting permission normalisation on %s.", e.plan.BackupTarget))
	e.log.PrintLine("Fix Perms Path", e.plan.BackupTarget)
	e.log.PrintLine("Fix Perms Owner", e.plan.LocalOwner)
	e.log.PrintLine("Fix Perms Group", e.plan.LocalGroup)

	if e.plan.DryRun {
		e.log.DryRun("%s", e.plan.FixPermsChownCommand)
		e.log.DryRun("%s", e.plan.FixPermsDirPermsCommand)
		e.log.DryRun("%s", e.plan.FixPermsFilePermsCommand)
		e.log.Info("%s", statusLinef("Permission normalisation completed (dry-run)."))
		return nil
	}

	if err := permissions.Fix(e.runner, e.plan.BackupTarget, e.plan.LocalOwner, e.plan.LocalGroup, false); err != nil {
		return err
	}
	e.log.Info("%s", statusLinef("Permission normalisation completed successfully."))
	return nil
}
