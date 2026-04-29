package command

import (
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func parseNotifyRequest(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
	if len(args) == 0 {
		return &ParseResult{Handled: true, Output: NotifyUsageText(meta, rt)}, nil
	}
	if result := parseHelpRequest(args, NotifyUsageText(meta, rt), FullNotifyUsageText(meta, rt)); result != nil {
		return result, nil
	}

	action := args[0]
	switch action {
	case "test":
	default:
		return nil, workflowcore.NewUsageRequestError("unknown notify command %s", action)
	}

	req, err := parseNotifyFlags(args[1:])
	if err != nil {
		return nil, err
	}
	req.Command = "notify"
	req.NotifyCommand = action
	if action == "test" && req.Source == "update" {
		req.Source = ""
		req.NotifyScope = "update"
		if req.RequestedTarget != "" {
			return nil, workflowcore.NewUsageRequestError("notify test update does not use --target")
		}
		return &ParseResult{Request: req}, nil
	}
	if req.NotifyEvent != "" {
		return nil, workflowcore.NewUsageRequestError("--event is only supported for notify test update")
	}
	if err := validateTargetAndLabel(req); err != nil {
		return nil, err
	}

	return &ParseResult{Request: req}, nil
}

func parseNotifyFlags(args []string) (*workflowcore.Request, error) {
	req := &workflowcore.Request{
		NotifyProvider: "all",
		NotifySeverity: "warning",
	}
	err := parseSourceFlags(args, req, sharedFlagOptions{
		target:      true,
		dryRun:      true,
		jsonSummary: true,
		configDir:   true,
		secretsDir:  true,
	}, func(args []string, index *int, req *workflowcore.Request) (bool, error) {
		switch args[*index] {
		case "--provider":
			value, err := consumeRequiredValue(args, index, "--provider")
			if err != nil {
				return false, err
			}
			req.NotifyProvider = value
			return true, nil
		case "--severity":
			value, err := consumeRequiredValue(args, index, "--severity")
			if err != nil {
				return false, err
			}
			req.NotifySeverity = value
			return true, nil
		case "--summary":
			value, err := consumeRequiredValue(args, index, "--summary")
			if err != nil {
				return false, err
			}
			req.NotifySummary = value
			return true, nil
		case "--message":
			value, err := consumeRequiredValue(args, index, "--message")
			if err != nil {
				return false, err
			}
			req.NotifyMessage = value
			return true, nil
		case "--event":
			value, err := consumeRequiredValue(args, index, "--event")
			if err != nil {
				return false, err
			}
			req.NotifyEvent = value
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	if err := validateNotifyProvider(req.NotifyProvider); err != nil {
		return nil, err
	}
	if err := validateNotifySeverity(req.NotifySeverity); err != nil {
		return nil, err
	}
	if err := validateNotifyEvent(req.NotifyEvent); err != nil {
		return nil, err
	}
	return req, nil
}

func validateNotifyProvider(provider string) error {
	switch strings.TrimSpace(provider) {
	case "", "all", "webhook", "ntfy":
		return nil
	default:
		return workflowcore.NewRequestError("unsupported notify provider %q; expected all, webhook, or ntfy", provider)
	}
}

func validateNotifySeverity(severity string) error {
	switch strings.TrimSpace(severity) {
	case "", "warning", "critical", "info":
		return nil
	default:
		return workflowcore.NewRequestError("unsupported notify severity %q; expected warning, critical, or info", severity)
	}
}

func validateNotifyEvent(event string) error {
	if strings.TrimSpace(event) == "" {
		return nil
	}
	if notify.IsKnownEvent(event) {
		return nil
	}
	return workflowcore.NewUsageRequestError("unsupported notify event %q", event)
}
