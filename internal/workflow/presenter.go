package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// Presenter owns operator-facing runtime output for the workflow execution
// path. Keeping rendering here lets Executor focus on sequencing and policy.
type Presenter struct {
	meta    Metadata
	rt      Runtime
	log     *logger.Logger
	verbose bool
}

func NewPresenter(meta Metadata, rt Runtime, log *logger.Logger, verbose bool) *Presenter {
	return &Presenter{meta: meta, rt: rt, log: log, verbose: verbose}
}

var backupRevisionPattern = regexp.MustCompile(`(?i)revision\s+(\d+)\s+completed`)
var backupDurationPattern = regexp.MustCompile(`(?i)^Total running time:\s*(.+)$`)
var backupFilesPattern = regexp.MustCompile(`(?i)^Files:\s*(.+)$`)

func (p *Presenter) PrintHeader(plan *Plan, startedAt time.Time, _ string) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run started - %s", startedAt.Format("2006-01-02 15:04:05")))
	p.log.PrintLine("Operation", plan.OperationMode)
	p.log.PrintLine("Label", plan.BackupLabel)
	p.log.PrintLine("Target", plan.TargetName())
	if p.verbose && plan.DefaultNotice != "" {
		p.log.PrintLine("Notice", plan.DefaultNotice)
	}
}

func (p *Presenter) PrintSummary(plan *Plan) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run Summary:"))
	for _, line := range plan.Summary {
		p.log.PrintLine(line.Label, line.Value)
	}
}

func (p *Presenter) PrintPreRunFailureContext(req *Request) {
	if req == nil {
		return
	}
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run could not start"))
	if op := OperationMode(req); op != "" {
		p.log.PrintLine("Operation", op)
	}
	if req.Source != "" {
		p.log.PrintLine("Label", req.Source)
	}
	if req.Target() != "" {
		p.log.PrintLine("Target", req.Target())
	}
}

func (p *Presenter) PrintPreRunFailurePlan(plan *Plan) {
	if plan == nil {
		return
	}
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run could not start"))
	if plan.OperationMode != "" {
		p.log.PrintLine("Operation", plan.OperationMode)
	}
	if plan.BackupLabel != "" {
		p.log.PrintLine("Label", plan.BackupLabel)
	}
	if plan.TargetName() != "" {
		p.log.PrintLine("Target", plan.TargetName())
	}
}

func (p *Presenter) PrintPhase(name string) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Phase: %s", name))
}

func (p *Presenter) PrintStatus(status string) {
	p.log.PrintLine("Status", status)
}

func (p *Presenter) StartStatusActivity(status string) func() {
	if p.log.Interactive() {
		return p.log.StartActivity(status)
	}
	p.PrintStatus(status)
	return func() {}
}

func (p *Presenter) PrintDuration(start time.Time) {
	duration := p.rt.Now().Sub(start)
	if duration < 0 {
		duration = 0
	}
	seconds := int(duration.Truncate(time.Second) / time.Second)
	if duration > 0 && seconds == 0 {
		seconds = 1
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	p.log.PrintLine("Duration", fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs))
}

func (p *Presenter) PrintCommandOutput(stdout, stderr string, force bool) {
	if !p.verbose && !force {
		return
	}
	if stdout != "" {
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line != "" {
				if p.verbose && !force {
					p.log.PrintLine("Output", line)
				} else {
					p.log.Info("%s", line)
				}
			}
		}
	}
	if stderr != "" {
		for _, line := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
			if line != "" {
				p.log.Warn("%s", line)
			}
		}
	}
}

func (p *Presenter) PrintBackupResult(stdout, stderr string, force bool) {
	if force {
		p.PrintCommandOutput(stdout, stderr, true)
		return
	}
	if p.verbose {
		p.PrintCommandOutput(stdout, stderr, false)
		return
	}

	var revision string
	var files string
	var duration string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if match := backupRevisionPattern.FindStringSubmatch(line); len(match) > 1 {
			revision = match[1]
			continue
		}
		if match := backupFilesPattern.FindStringSubmatch(line); len(match) > 1 {
			files = match[1]
			continue
		}
		if match := backupDurationPattern.FindStringSubmatch(line); len(match) > 1 {
			duration = match[1]
		}
	}

	if revision != "" {
		p.log.PrintLine("Revision", revision)
	}
	if files != "" {
		p.log.PrintLine("Files", files)
	}
	if duration != "" {
		p.log.PrintLine("Duration", duration)
	}
}

func (p *Presenter) PrintPrunePreview(preview *duplicacy.PrunePreview, minTotalForPercent int) {
	p.log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
	p.log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
	if preview.PercentEnforced {
		p.log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
		return
	}
	p.log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", minTotalForPercent))
}

func (p *Presenter) PrintCompletion(exitCode int, start time.Time) {
	status := "Success"
	if exitCode != 0 {
		status = "Failed"
	}
	p.log.PrintSeparator()
	p.log.PrintLine("Result", p.log.FormatResult(status))
	p.log.PrintLine("Code", fmt.Sprintf("%d", exitCode))
	p.PrintDuration(start)
	p.log.Info("%s", statusLinef("Run completed - %s", p.rt.Now().Format("2006-01-02 15:04:05")))
	p.log.PrintSeparator()
}
