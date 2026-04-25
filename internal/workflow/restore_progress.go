package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type RestoreProgress interface {
	PrintRunStart(req *RestoreRequest, plan *Plan, inputs restoreRunInputs, startedAt time.Time)
	PrintSelectionStart(req *RestoreRequest, plan *Plan, revision int, workspace string, total int, startedAt time.Time)
	PrintStatus(status string)
	StartActivity(status string) func()
	StartSelectionActivity(current, total int, path string) func()
	PrintInterrupted(info restoreInterruptInfo)
	PrintRunCompletion(success bool, startedAt time.Time)
}

type noopRestoreProgress struct{}

func (noopRestoreProgress) PrintRunStart(*RestoreRequest, *Plan, restoreRunInputs, time.Time) {}
func (noopRestoreProgress) PrintSelectionStart(*RestoreRequest, *Plan, int, string, int, time.Time) {
}
func (noopRestoreProgress) PrintStatus(string)          {}
func (noopRestoreProgress) StartActivity(string) func() { return func() {} }
func (noopRestoreProgress) StartSelectionActivity(int, int, string) func() {
	return func() {}
}
func (noopRestoreProgress) PrintInterrupted(restoreInterruptInfo) {}
func (noopRestoreProgress) PrintRunCompletion(bool, time.Time)    {}

type loggerRestoreProgress struct {
	runtime *presentation.RuntimePresenter
	log     *logger.Logger
}

// Restore run/select are the only restore commands with live progress output.
// Restore plan/list-revisions/inspection are read-only formatter paths, so
// their operation labels are emitted by restore_format.go instead of the
// runtime presenter.
func NewRestoreProgress(meta Metadata, rt Runtime, log *logger.Logger) RestoreProgress {
	if log == nil {
		return noopRestoreProgress{}
	}
	return &loggerRestoreProgress{
		runtime: presentation.NewRuntimePresenter(rt.Now, log, false),
		log:     log,
	}
}

func (p *loggerRestoreProgress) PrintRunStart(req *RestoreRequest, plan *Plan, inputs restoreRunInputs, startedAt time.Time) {
	data := presentation.HeaderData{
		StartedAt: startedAt,
		Operation: "Restore",
		Label:     req.Label,
		Target:    req.Target(),
	}
	if plan != nil {
		data.Location = plan.Location
	}
	p.runtime.PrintHeader(data)
	p.log.PrintLine("Revision", strconv.Itoa(inputs.Revision))
	p.log.PrintLine("Workspace", inputs.Workspace)
	p.log.PrintLine("Path", restoreProgressPath(inputs.RestorePath))
	p.printSafetyWarning()
}

func (p *loggerRestoreProgress) PrintSelectionStart(req *RestoreRequest, plan *Plan, revision int, workspace string, total int, startedAt time.Time) {
	data := presentation.HeaderData{
		StartedAt: startedAt,
		Operation: "Restore selection",
		Label:     req.Label,
		Target:    req.Target(),
	}
	if plan != nil {
		data.Location = plan.Location
	}
	p.runtime.PrintHeader(data)
	p.log.PrintLine("Revision", strconv.Itoa(revision))
	p.log.PrintLine("Workspace", workspace)
	p.log.PrintLine("Restore paths", strconv.Itoa(total))
	p.printSafetyWarning()
}

func (p *loggerRestoreProgress) printSafetyWarning() {
	p.log.Warn("  %s : %s", p.log.FormatLabel("Restore safety"), p.log.FormatValue("workspace only; live source will not be modified; copy-back is manual"))
}

func (p *loggerRestoreProgress) PrintStatus(status string) {
	p.runtime.PrintStatus(status)
}

func (p *loggerRestoreProgress) StartActivity(status string) func() {
	return p.runtime.StartStatusActivity(status)
}

func (p *loggerRestoreProgress) StartSelectionActivity(current, total int, path string) func() {
	status := restoreSelectionProgressActivity(current, total, path)
	if p.log.Interactive() {
		p.log.Record(logger.INFO, "  %s : %s", p.log.FormatLabel("Status"), p.log.FormatValue(status))
		return p.log.StartActivity(status)
	}
	return p.runtime.StartStatusActivity(status)
}

func (p *loggerRestoreProgress) PrintInterrupted(info restoreInterruptInfo) {
	p.log.Warn("  %s : %s", p.log.FormatLabel("Restore interrupted"), p.log.FormatValue("received "+info.Signal+"; cancelling active Duplicacy restore"))
	p.log.Warn("  %s : %s", p.log.FormatLabel("Workspace retained"), p.log.FormatValue(info.Workspace))
	p.log.Warn("  %s : %s", p.log.FormatLabel("Cleanup"), p.log.FormatValue("no restored files were deleted automatically"))
	p.log.Warn("  %s : %s", p.log.FormatLabel("Completed paths"), p.log.FormatValue(restoreInterruptProgress(info)))
	if strings.TrimSpace(info.CurrentPath) != "" {
		p.log.Warn("  %s : %s", p.log.FormatLabel("Current path"), p.log.FormatValue(info.CurrentPath))
	}
	p.log.Warn("  %s : %s", p.log.FormatLabel("Live source"), p.log.FormatValue("not modified"))
}

func (p *loggerRestoreProgress) PrintRunCompletion(success bool, startedAt time.Time) {
	code := 0
	if !success {
		code = 1
	}
	p.runtime.PrintCompletion(code, startedAt)
}

func restoreProgressPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "<full revision>"
	}
	return path
}

func restoreProgressActivity(inputs restoreRunInputs) string {
	if strings.TrimSpace(inputs.RestorePath) == "" {
		return fmt.Sprintf("Restoring revision %d into drill workspace", inputs.Revision)
	}
	return fmt.Sprintf("Restoring selected path from revision %d into drill workspace", inputs.Revision)
}

func restoreSelectionProgressActivity(current, total int, path string) string {
	return fmt.Sprintf("Restoring selection %d of %d: %s", current, total, restoreProgressShortPath(path))
}

func restoreProgressShortPath(path string) string {
	path = restoreProgressPath(path)
	const limit = 90
	if len(path) <= limit {
		return path
	}
	return path[:limit-3] + "..."
}
