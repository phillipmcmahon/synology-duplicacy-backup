package workflow

import (
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/presentation"
)

type Presenter struct {
	runtime *presentation.RuntimePresenter
}

func NewPresenter(_ Metadata, rt Runtime, log *logger.Logger, verbose bool) *Presenter {
	return &Presenter{runtime: presentation.NewRuntimePresenter(rt.Now, log, verbose)}
}

func (p *Presenter) PrintHeader(plan *Plan, startedAt time.Time, _ string) {
	p.runtime.PrintHeader(p.headerData(plan, startedAt))
}

func (p *Presenter) PrintSummary(plan *Plan) {
	if plan == nil {
		return
	}
	p.runtime.PrintSummary(plan.Summary)
}

func (p *Presenter) PrintPreRunFailureContext(req *Request) {
	if req == nil {
		return
	}
	p.runtime.PrintPreRunFailure(p.preRunFailureDataFromRequest(req))
}

func (p *Presenter) PrintPreRunFailurePlan(plan *Plan) {
	if plan == nil {
		return
	}
	p.runtime.PrintPreRunFailure(p.preRunFailureDataFromPlan(plan))
}

func (p *Presenter) PrintPhase(name string) {
	p.runtime.PrintPhase(name)
}

func (p *Presenter) PrintStatus(status string) {
	p.runtime.PrintStatus(status)
}

func (p *Presenter) StartStatusActivity(status string) func() {
	return p.runtime.StartStatusActivity(status)
}

func (p *Presenter) PrintDuration(start time.Time) {
	p.runtime.PrintDuration(start)
}

func (p *Presenter) PrintCommandOutput(stdout, stderr string, force bool) {
	p.runtime.PrintCommandOutput(stdout, stderr, force)
}

func (p *Presenter) PrintBackupResult(stdout, stderr string, force bool) {
	p.runtime.PrintBackupResult(stdout, stderr, force)
}

func (p *Presenter) PrintPrunePreview(preview *duplicacy.PrunePreview, minTotalForPercent int) {
	p.runtime.PrintPrunePreview(preview, minTotalForPercent)
}

func (p *Presenter) PrintCompletion(exitCode int, start time.Time) {
	p.runtime.PrintCompletion(exitCode, start)
}

func (p *Presenter) headerData(plan *Plan, startedAt time.Time) presentation.HeaderData {
	if plan == nil {
		return presentation.HeaderData{StartedAt: startedAt}
	}
	return presentation.HeaderData{
		StartedAt:     startedAt,
		Operation:     plan.OperationMode,
		Label:         plan.BackupLabel,
		Target:        plan.TargetName(),
		Location:      plan.Location,
		DefaultNotice: plan.DefaultNotice,
	}
}

func (p *Presenter) preRunFailureDataFromRequest(req *Request) presentation.PreRunFailureData {
	return presentation.PreRunFailureData{
		Operation: OperationMode(req),
		Label:     req.Source,
		Target:    req.Target(),
	}
}

func (p *Presenter) preRunFailureDataFromPlan(plan *Plan) presentation.PreRunFailureData {
	return presentation.PreRunFailureData{
		Operation: plan.OperationMode,
		Label:     plan.BackupLabel,
		Target:    plan.TargetName(),
		Location:  plan.Location,
	}
}
