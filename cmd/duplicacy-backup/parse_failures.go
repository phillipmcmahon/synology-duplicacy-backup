package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/command"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func buildRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*command.ParseResult, int) {
	startedAt := rt.Now()
	result, err := command.ParseRequest(args, meta, rt)
	if err == nil {
		return result, 0
	}

	failureContext := command.ParseFailureContext(args)
	if !failureContext.JSONSummary {
		writeDirectError("%s", workflow.OperatorMessage(err))
		var requestErr *workflow.RequestError
		if errors.As(err, &requestErr) && requestErr.ShowUsage {
			_, _ = os.Stderr.WriteString("\n")
			fmt.Print(command.UsageText(meta, rt))
		}
		return nil, 1
	}

	writeDirectError("%s", workflow.OperatorMessage(err))
	completedAt := rt.Now()
	switch failureContext.Kind {
	case command.FailureRequestHealth:
		req := failureContext.Request
		healthReq := workflow.NewHealthRequest(req)
		_ = workflow.WriteHealthReport(os.Stdout, workflow.NewFailureHealthReport(&healthReq, healthReq.Command, workflow.OperatorMessage(err), completedAt))
	case command.FailureRequestNotify:
		req := failureContext.Request
		commandName := req.NotifyCommand
		if commandName == "" {
			commandName = "test"
		}
		provider := req.NotifyProvider
		if provider == "" {
			provider = notify.ProviderAll
		}
		severity := req.NotifySeverity
		if severity == "" {
			severity = "warning"
		}
		summary := req.NotifySummary
		if summary == "" {
			summary = "Notification test failed"
		}
		message := req.NotifyMessage
		if message == "" {
			message = workflow.OperatorMessage(err)
		}
		_ = notify.WriteTestReport(os.Stdout, notify.NewFailureTestReport(notify.TestReportInput{
			Command:  commandName,
			Scope:    req.NotifyScope,
			Label:    req.Source,
			Target:   req.Target(),
			Provider: provider,
			Severity: severity,
			Summary:  summary,
			Message:  message,
			DryRun:   req.DryRun,
		}))
	default:
		emitJSONFailureSummary(os.Stdout, nil, nil, startedAt, completedAt, workflow.OperatorMessage(err))
	}
	return nil, 1
}

func emitJSONFailureSummary(w io.Writer, req *workflow.RuntimeRequest, plan *workflow.Plan, startedAt, completedAt time.Time, message string) {
	if w == nil {
		return
	}
	if err := workflow.WriteRunReport(w, workflow.NewFailureRunReport(req, plan, startedAt, completedAt, exitCodeGeneralFailure, message)); err != nil {
		writeJSONSummaryFailure(err)
	}
}
