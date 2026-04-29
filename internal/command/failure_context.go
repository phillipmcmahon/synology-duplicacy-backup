package command

import "github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"

type FailureRequestKind string

const (
	FailureRequestNone   FailureRequestKind = ""
	FailureRequestHealth FailureRequestKind = "health"
	FailureRequestNotify FailureRequestKind = "notify"
)

type FailureContext struct {
	JSONSummary bool
	Kind        FailureRequestKind
	Request     *workflowcore.Request
}

func ParseFailureContext(args []string) FailureContext {
	ctx := FailureContext{
		JSONSummary: WantsJSONSummary(args),
		Request:     &workflowcore.Request{},
	}
	if len(args) == 0 {
		return ctx
	}

	switch args[0] {
	case "health":
		ctx.Kind = FailureRequestHealth
		ctx.Request = parseHealthFailureRequest(args[1:])
	case "notify":
		ctx.Kind = FailureRequestNotify
		ctx.Request = parseNotifyFailureRequest(args[1:])
	}
	return ctx
}

func WantsJSONSummary(args []string) bool {
	for _, arg := range args {
		if arg == "--json-summary" {
			return true
		}
	}
	return false
}

func parseHealthFailureRequest(args []string) *workflowcore.Request {
	req := &workflowcore.Request{Command: "health"}
	if len(args) > 0 && !isOption(args[0]) {
		req.HealthCommand = args[0]
		args = args[1:]
	}

	var positional []string
	for i := 0; i < len(args); i++ {
		handled := consumeSharedFlagForFailure(args, &i, req, sharedFlagOptions{
			target:      true,
			verbose:     true,
			jsonSummary: true,
			configDir:   true,
			secretsDir:  true,
		})
		if handled {
			continue
		}
		if !isOption(args[i]) {
			positional = append(positional, args[i])
		}
	}
	req.Source = firstPositional(positional)
	return req
}

func parseNotifyFailureRequest(args []string) *workflowcore.Request {
	req := &workflowcore.Request{Command: "notify", NotifyProvider: "all", NotifySeverity: "warning"}
	if len(args) > 0 && !isOption(args[0]) {
		req.NotifyCommand = args[0]
		args = args[1:]
	}

	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			req.NotifyProvider = consumeOptionalValue(args, &i, req.NotifyProvider)
		case "--severity":
			req.NotifySeverity = consumeOptionalValue(args, &i, req.NotifySeverity)
		case "--summary":
			req.NotifySummary = consumeOptionalValue(args, &i, req.NotifySummary)
		case "--message":
			req.NotifyMessage = consumeOptionalValue(args, &i, req.NotifyMessage)
		case "--event":
			req.NotifyEvent = consumeOptionalValue(args, &i, req.NotifyEvent)
		default:
			handled := consumeSharedFlagForFailure(args, &i, req, sharedFlagOptions{
				target:      true,
				dryRun:      true,
				jsonSummary: true,
				configDir:   true,
				secretsDir:  true,
			})
			if handled {
				continue
			}
			if !isOption(args[i]) {
				positional = append(positional, args[i])
			}
		}
	}

	req.Source = firstPositional(positional)
	if req.Source == "update" {
		req.Source = ""
		req.NotifyScope = "update"
	}
	return req
}

func consumeSharedFlagForFailure(args []string, index *int, req *workflowcore.Request, opts sharedFlagOptions) bool {
	handled, err := consumeSharedFlag(args, index, req, opts)
	if err == nil {
		return handled
	}
	return isSharedFlag(args[*index], opts)
}

func isSharedFlag(arg string, opts sharedFlagOptions) bool {
	switch arg {
	case "--target":
		return opts.target
	case "--dry-run":
		return opts.dryRun
	case "--verbose":
		return opts.verbose
	case "--json-summary":
		return opts.jsonSummary
	case "--config-dir":
		return opts.configDir
	case "--secrets-dir":
		return opts.secretsDir
	default:
		return false
	}
}

func consumeOptionalValue(args []string, index *int, fallback string) string {
	if *index+1 >= len(args) {
		return fallback
	}
	*index++
	return args[*index]
}

func firstPositional(positional []string) string {
	if len(positional) == 0 {
		return ""
	}
	return positional[0]
}
