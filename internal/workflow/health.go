package workflow

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/duplicacy"
	execpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/exec"
	healthpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/health"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/lock"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/logger"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

type HealthIssue = healthpkg.Issue
type HealthCheck = healthpkg.Check
type HealthRevisionResult = healthpkg.RevisionResult
type HealthReport = healthpkg.Report

type HealthRunner struct {
	meta      Metadata
	rt        Env
	log       *logger.Logger
	runner    execpkg.Runner
	presenter *healthpkg.Presenter
}

const (
	verifyFailureNoRevisionsFound = healthpkg.VerifyFailureNoRevisionsFound
	verifyFailureIntegrityFailed  = healthpkg.VerifyFailureIntegrityFailed
	verifyFailureResultMissing    = healthpkg.VerifyFailureResultMissing
	verifyFailureAccessFailed     = healthpkg.VerifyFailureAccessFailed
	verifyFailureListingFailed    = healthpkg.VerifyFailureListingFailed
	verifyActionRunBackup         = "run_backup"
)

func NewHealthRunner(meta Metadata, rt Env, log *logger.Logger, runner execpkg.Runner) *HealthRunner {
	return &HealthRunner{
		meta:      meta,
		rt:        rt,
		log:       log,
		runner:    runner,
		presenter: healthpkg.NewPresenter(log, rt.Now),
	}
}

func NewFailureHealthReport(req *HealthRequest, checkType, message string, checkedAt time.Time) *HealthReport {
	mode := ""
	label := ""
	target := ""
	if req != nil {
		label = req.Label
		target = req.Target()
		mode = req.Target()
		if checkType == "" {
			checkType = req.Command
		}
	}
	return healthpkg.NewFailureReport(checkType, label, target, mode, message, checkedAt)
}

func WriteHealthReport(w io.Writer, report *HealthReport) error {
	return healthpkg.WriteReport(w, report)
}

func (h *HealthRunner) Run(req *Request) (*HealthReport, int) {
	healthReq := NewHealthRequest(req)
	return h.run(&healthReq)
}

func (h *HealthRunner) run(req *HealthRequest) (*HealthReport, int) {
	checkedAt := h.rt.Now()
	report := &HealthReport{
		Status:    "healthy",
		CheckType: req.Command,
		Label:     req.Label,
		Target:    req.Target(),
		Mode:      req.Target(),
		CheckedAt: formatReportTime(checkedAt),
		StartedAt: checkedAt,
	}

	restoreDebugCommands := h.suppressCommandDebug()
	defer restoreDebugCommands()

	h.addRootProfileConfigWarning(report, req)

	cfg, plan, sec, err := h.prepare(req)
	if err != nil {
		h.presenter.PrintHeader(report)
		report.AddCheck("Environment", "fail", OperatorMessage(err))
		report.Finalize()
		h.maybeSendEarlyNotification(req, report)
		h.presenter.PrintReport(report)
		return report, healthpkg.ExitCode(report.Status)
	}
	report.Location = plan.Config.Location
	h.presenter.PrintHeader(report)

	state, stateErr := loadRunState(h.meta, req.Label, req.Target())
	if stateErr != nil {
		if os.IsNotExist(stateErr) {
			report.AddCheck("Backup state", "warn", fmt.Sprintf("No prior backup state found at %s", stateFilePath(h.meta, req.Label, req.Target())))
		} else {
			report.AddCheck("Backup state", "warn", fmt.Sprintf("Could not read backup state: %v", stateErr))
		}
	} else {
		report.AddCheck("Backup state", "pass", "Available")
		report.LastSuccessAt = chooseLocalSuccessTime(state)
	}

	lockStatus, lockErr := lock.InspectTarget(h.meta.LockParent, req.Label, req.Target())
	if lockErr != nil {
		report.AddCheck("Lock", "warn", OperatorMessage(lockErr))
	} else {
		switch {
		case lockStatus.Active:
			report.AddCheck("Lock", "warn", fmt.Sprintf("Active lock detected (PID %d)", lockStatus.PID))
		case lockStatus.Stale:
			report.AddCheck("Lock", "warn", "Stale lock detected")
		default:
			report.AddCheck("Lock", "pass", "No active lock")
		}
	}

	dup, dupErr := h.prepareDuplicacySetup(plan, sec)
	if dupErr != nil {
		report.AddCheck("Duplicacy setup", "fail", OperatorMessage(dupErr))
		report.Finalize()
		h.maybeSendEarlyNotification(req, report)
		h.presenter.PrintReport(report)
		return report, healthpkg.ExitCode(report.Status)
	}
	defer dup.Cleanup()

	var visibleRevisions []duplicacy.RevisionInfo
	if localRepositoryRequiresSudo(cfg, h.rt) {
		h.runLocalRepositorySudoStatusChecks(report, req, plan)
	} else {
		visibleRevisions = h.runStatusChecks(report, req, cfg, plan, state, dup)
	}
	if req.Command == "doctor" || req.Command == "verify" {
		h.runDoctorChecks(report, req, cfg, plan, dup)
	}
	if req.Command == "verify" {
		h.runVerifyChecks(report, cfg, dup, visibleRevisions)
	}

	if err := updateHealthCheckState(h.meta, req.Label, req.Target(), req.Command, checkedAt); err != nil {
		report.AddCheck("Health state", "warn", err.Error())
	}

	report.Finalize()
	if h.shouldSendNotification(req, cfg.Health, report.Status) {
		if payload := buildHealthNotificationPayload(h.rt, report); payload != nil {
			if err := notify.SendConfigured(cfg.Health.Notify, plan.Paths.SecretsFile, report.Target, payload); err != nil {
				report.AddCheck("Notification", "warn", OperatorMessage(err))
			} else {
				report.NotificationSent = true
				report.AddCheck("Notification", "pass", "Delivered")
			}
		}
	}
	report.Finalize()
	h.presenter.PrintReport(report)
	return report, healthpkg.ExitCode(report.Status)
}

func (h *HealthRunner) suppressCommandDebug() func() {
	type commandDebugController interface {
		SetDebugCommands(bool)
		DebugCommands() bool
	}

	controller, ok := h.runner.(commandDebugController)
	if !ok {
		return func() {}
	}

	previous := controller.DebugCommands()
	controller.SetDebugCommands(false)
	return func() {
		controller.SetDebugCommands(previous)
	}
}

func healthCheckSection(name string) string { return healthpkg.SectionForCheck(name) }
func healthCheckLabel(name string) string   { return healthpkg.LabelForCheck(name) }
func humanAge(d time.Duration) string       { return healthpkg.HumanAge(d) }
func humanAgo(d time.Duration) string       { return healthpkg.HumanAgo(d) }

func (h *HealthRunner) addVerifyFailureCode(report *HealthReport, code string) {
	report.AddVerifyFailureCode(code)
}

func (h *HealthRunner) hasVerifyFailureCode(report *HealthReport, code string) bool {
	return report.HasVerifyFailureCode(code)
}
