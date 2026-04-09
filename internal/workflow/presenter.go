package workflow

import (
	"fmt"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

// Presenter owns operator-facing runtime output for the workflow execution
// path. Keeping rendering here lets Executor focus on sequencing and policy.
type Presenter struct {
	meta Metadata
	rt   Runtime
	log  *logger.Logger
}

func NewPresenter(meta Metadata, rt Runtime, log *logger.Logger) *Presenter {
	return &Presenter{meta: meta, rt: rt, log: log}
}

func (p *Presenter) PrintHeader(lockPath string) {
	p.log.PrintSeparator()
	p.log.Info("Backup script started - %s", p.rt.Now().Format("2006-01-02 15:04:05"))
	p.log.PrintLine("Script", p.meta.ScriptName)
	p.log.PrintLine("PID", fmt.Sprintf("%d", p.rt.Getpid()))
	p.log.PrintLine("Lock Path", lockPath)
	p.log.PrintSeparator()
}

func (p *Presenter) PrintSummary(plan *Plan) {
	p.log.Info("Configuration Summary:")
	for _, line := range plan.Summary {
		p.log.PrintLine(line.Label, line.Value)
	}
}

func (p *Presenter) PrintCommandOutput(stdout, stderr string) {
	if stdout != "" {
		for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
			if line != "" {
				p.log.Info("%s", line)
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

func (p *Presenter) PrintPrunePreview(preview *duplicacy.PrunePreview, minTotalForPercent int) {
	p.log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
	p.log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
	if preview.PercentEnforced {
		p.log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
		return
	}
	p.log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", minTotalForPercent))
}

func (p *Presenter) PrintCompletion(exitCode int) {
	status := "SUCCESS"
	if exitCode != 0 {
		status = "FAILED"
	}
	p.log.PrintSeparator()
	p.log.Info("Backup script completed:")
	p.log.PrintLine("Result", p.log.FormatResult(status))
	p.log.PrintLine("Code", fmt.Sprintf("%d", exitCode))
	p.log.PrintLine("Timestamp", p.rt.Now().Format("2006-01-02 15:04:05"))
	p.log.PrintSeparator()
}
