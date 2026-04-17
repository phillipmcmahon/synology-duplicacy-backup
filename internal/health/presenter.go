package health

import (
	"fmt"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type Presenter struct {
	log *logger.Logger
	now func() time.Time
}

func NewPresenter(log *logger.Logger, now func() time.Time) *Presenter {
	return &Presenter{log: log, now: now}
}

func (p *Presenter) PrintHeader(report *Report) {
	if p == nil || p.log == nil || report == nil {
		return
	}
	p.log.PrintSeparator()
	p.log.Info("%s", statusLinef("Health check started - %s", report.StartedAt.Format("2006-01-02 15:04:05")))
	p.log.PrintLine("Check", presentation.Title(report.CheckType))
	p.log.PrintLine("Label", report.Label)
	p.log.PrintLine("Target", report.Target)
	if report.StorageType != "" {
		p.log.PrintLine("Type", report.StorageType)
	}
	if report.Location != "" {
		p.log.PrintLine("Location", report.Location)
	}
}

func (p *Presenter) PrintReport(report *Report) {
	if p == nil || p.log == nil || report == nil {
		return
	}
	report.CompletedAt = p.now()
	currentSection := ""
	for _, check := range report.Checks {
		section := SectionForCheck(check.Name)
		if section != currentSection {
			p.log.PrintSeparator()
			p.log.Info("%s", statusLinef("Section: %s", section))
			currentSection = section
		}
		p.printCheck(check)
	}
	p.log.PrintSeparator()
	p.log.Info("  %s : %s", p.log.FormatLabel("Result"), p.log.FormatResult(presentation.Title(report.Status)))
	p.log.PrintLine("Code", fmt.Sprintf("%d", ExitCode(report.Status)))
	p.log.PrintLine("Duration", FormatClockDuration(report.CompletedAt.Sub(report.StartedAt)))
	p.log.Info("%s", statusLinef("Health check completed - %s", report.CompletedAt.Format("2006-01-02 15:04:05")))
	p.log.PrintSeparator()
}

func (p *Presenter) StartStatusActivity(status string) func() {
	if p == nil || p.log == nil {
		return func() {}
	}
	if p.log.Interactive() {
		return p.log.StartActivity(status)
	}
	p.log.PrintLine("Status", status)
	return func() {}
}

func (p *Presenter) printCheck(check Check) {
	label := LabelForCheck(check.Name)
	switch check.Result {
	case "warn":
		p.log.Warn("  %s : %s", p.log.FormatLabel(label), check.Message)
	case "fail":
		p.log.Error("  %s : %s", p.log.FormatLabel(label), check.Message)
	default:
		p.log.PrintLine(label, check.Message)
	}
}

func statusLinef(format string, args ...interface{}) string {
	return fmt.Sprintf("  %s", fmt.Sprintf(format, args...))
}
