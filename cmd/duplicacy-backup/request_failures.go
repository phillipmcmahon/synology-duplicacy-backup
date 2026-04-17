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

func buildRequest(args []string, meta workflow.Metadata, rt workflow.Runtime) (*command.ParseResult, int) {
	startedAt := rt.Now()
	result, err := command.ParseRequest(args, meta, rt)
	if err == nil {
		return result, 0
	}

	if !wantsJSONSummary(args) {
		fmt.Fprintf(os.Stderr, "[ERRO] %s\n", workflow.OperatorMessage(err))
		var requestErr *workflow.RequestError
		if errors.As(err, &requestErr) && requestErr.ShowUsage {
			fmt.Fprintln(os.Stderr)
			fmt.Print(command.UsageText(meta, rt))
		}
		return nil, 1
	}

	fmt.Fprintf(os.Stderr, "[ERRO] %s\n", workflow.OperatorMessage(err))
	completedAt := rt.Now()
	if looksLikeHealthCommand(args) {
		req := inferHealthFailureRequest(args)
		_ = workflow.WriteHealthReport(os.Stdout, workflow.NewFailureHealthReport(req, req.HealthCommand, workflow.OperatorMessage(err), completedAt))
	} else if looksLikeNotifyCommand(args) {
		req := inferNotifyFailureRequest(args)
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
	} else {
		emitJSONFailureSummary(os.Stdout, nil, nil, startedAt, completedAt, workflow.OperatorMessage(err))
	}
	return nil, 1
}

func wantsJSONSummary(args []string) bool {
	for _, arg := range args {
		if arg == "--json-summary" {
			return true
		}
	}
	return false
}

func looksLikeHealthCommand(args []string) bool {
	return len(args) > 0 && args[0] == "health"
}

func looksLikeNotifyCommand(args []string) bool {
	return len(args) > 0 && args[0] == "notify"
}

func inferHealthFailureRequest(args []string) *workflow.Request {
	req := &workflow.Request{}
	if len(args) == 0 || args[0] != "health" {
		return req
	}
	if len(args) > 1 && args[1] != "" && args[1][0] != '-' {
		req.HealthCommand = args[1]
	}
	var positional []string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 < len(args) {
				i++
				req.RequestedTarget = args[i]
			}
		case "--json-summary":
			req.JSONSummary = true
		case "--verbose":
			req.Verbose = true
		case "--config-dir", "--secrets-dir":
			i++
		default:
			if !isFlag(args[i]) {
				positional = append(positional, args[i])
			}
		}
	}
	if len(positional) > 0 {
		req.Source = positional[0]
	}
	return req
}

func inferNotifyFailureRequest(args []string) *workflow.Request {
	req := &workflow.Request{NotifyProvider: "all", NotifySeverity: "warning"}
	if len(args) == 0 || args[0] != "notify" {
		return req
	}
	if len(args) > 1 && args[1] != "" && args[1][0] != '-' {
		req.NotifyCommand = args[1]
	}
	var positional []string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 < len(args) {
				i++
				req.RequestedTarget = args[i]
			}
		case "--provider":
			if i+1 < len(args) {
				i++
				req.NotifyProvider = args[i]
			}
		case "--severity":
			if i+1 < len(args) {
				i++
				req.NotifySeverity = args[i]
			}
		case "--summary":
			if i+1 < len(args) {
				i++
				req.NotifySummary = args[i]
			}
		case "--message":
			if i+1 < len(args) {
				i++
				req.NotifyMessage = args[i]
			}
		case "--event":
			if i+1 < len(args) {
				i++
				req.NotifyEvent = args[i]
			}
		case "--dry-run":
			req.DryRun = true
		case "--json-summary":
			req.JSONSummary = true
		case "--config-dir", "--secrets-dir":
			i++
		default:
			if !isFlag(args[i]) {
				positional = append(positional, args[i])
			}
		}
	}
	if len(positional) > 0 {
		req.Source = positional[0]
		if req.Source == "update" {
			req.Source = ""
			req.NotifyScope = "update"
		}
	}
	return req
}

func emitJSONFailureSummary(w io.Writer, req *workflow.Request, plan *workflow.Plan, startedAt, completedAt time.Time, message string) {
	if w == nil {
		return
	}
	if err := workflow.WriteRunReport(w, workflow.NewFailureRunReport(req, plan, startedAt, completedAt, exitCodeGeneralFailure, message)); err != nil {
		writeJSONSummaryFailure(err)
	}
}

func isFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}
