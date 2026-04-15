package presentation

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
)

type RuntimePresenter struct {
	now     func() time.Time
	log     *logger.Logger
	verbose bool
}

type HeaderData struct {
	StartedAt     time.Time
	Operation     string
	Label         string
	Target        string
	StorageType   string
	Location      string
	DefaultNotice string
}

type PreRunFailureData struct {
	Operation   string
	Label       string
	Target      string
	StorageType string
	Location    string
}

func NewRuntimePresenter(now func() time.Time, log *logger.Logger, verbose bool) *RuntimePresenter {
	return &RuntimePresenter{now: now, log: log, verbose: verbose}
}

var backupRevisionPattern = regexp.MustCompile(`(?i)revision\s+(\d+)\s+completed`)
var backupDurationPattern = regexp.MustCompile(`(?i)^Total running time:\s*(.+)$`)
var backupFilesPattern = regexp.MustCompile(`(?i)^Files:\s*(.+)$`)

func (p *RuntimePresenter) PrintHeader(data HeaderData) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run started - %s", data.StartedAt.Format("2006-01-02 15:04:05")))
	p.log.PrintLine("Operation", data.Operation)
	p.log.PrintLine("Label", data.Label)
	p.log.PrintLine("Target", data.Target)
	if data.StorageType != "" {
		p.log.PrintLine("Type", data.StorageType)
	}
	if data.Location != "" {
		p.log.PrintLine("Location", data.Location)
	}
	if p.verbose && data.DefaultNotice != "" {
		p.log.PrintLine("Notice", data.DefaultNotice)
	}
}

func (p *RuntimePresenter) PrintSummary(lines []Line) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run Summary:"))
	for _, line := range lines {
		p.log.PrintLine(line.Label, line.Value)
	}
}

func (p *RuntimePresenter) PrintPreRunFailure(data PreRunFailureData) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Run could not start"))
	if data.Operation != "" {
		p.log.PrintLine("Operation", data.Operation)
	}
	if data.Label != "" {
		p.log.PrintLine("Label", data.Label)
	}
	if data.Target != "" {
		p.log.PrintLine("Target", data.Target)
	}
	if data.StorageType != "" {
		p.log.PrintLine("Type", data.StorageType)
	}
	if data.Location != "" {
		p.log.PrintLine("Location", data.Location)
	}
}

func (p *RuntimePresenter) PrintPhase(name string) {
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Phase: %s", name))
}

func (p *RuntimePresenter) PrintStatus(status string) {
	p.log.PrintLine("Status", status)
}

func (p *RuntimePresenter) StartStatusActivity(status string) func() {
	if p.log.Interactive() {
		return p.log.StartActivity(status)
	}
	p.PrintStatus(status)
	return func() {}
}

func (p *RuntimePresenter) PrintDuration(start time.Time) {
	duration := p.now().Sub(start)
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

func (p *RuntimePresenter) PrintCommandOutput(stdout, stderr string, force bool) {
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

func (p *RuntimePresenter) PrintBackupResult(stdout, stderr string, force bool) {
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

func (p *RuntimePresenter) PrintPrunePreview(preview *duplicacy.PrunePreview, minTotalForPercent int) {
	p.log.PrintLine("Preview Deletes", fmt.Sprintf("%d", preview.DeleteCount))
	p.log.PrintLine("Preview Total Revs", fmt.Sprintf("%d", preview.TotalRevisions))
	if preview.PercentEnforced {
		p.log.PrintLine("Preview Delete %", fmt.Sprintf("%d", preview.DeletePercent))
		return
	}
	p.log.PrintLine("Preview Delete %", fmt.Sprintf("<not enforced; total revisions unavailable or below %d>", minTotalForPercent))
}

func (p *RuntimePresenter) PrintCompletion(exitCode int, start time.Time) {
	status := "Success"
	if exitCode != 0 {
		status = "Failed"
	}
	p.log.PrintSeparator()
	p.log.PrintLine("Result", p.log.FormatResult(status))
	p.log.PrintLine("Code", fmt.Sprintf("%d", exitCode))
	p.PrintDuration(start)
	p.log.Info("%s", statusLinef("Run completed - %s", p.now().Format("2006-01-02 15:04:05")))
	p.log.PrintSeparator()
}

func statusLinef(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}
