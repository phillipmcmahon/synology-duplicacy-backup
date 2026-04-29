package command

import (
	"sort"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

type CommandFamily string

const (
	CommandFamilyRuntime       CommandFamily = "runtime"
	CommandFamilyConfig        CommandFamily = "config"
	CommandFamilyDiagnostics   CommandFamily = "diagnostics"
	CommandFamilyHealth        CommandFamily = "health"
	CommandFamilyNotify        CommandFamily = "notify"
	CommandFamilyRestore       CommandFamily = "restore"
	CommandFamilyManagedUpdate CommandFamily = "managed-update"
)

type ProfilePolicy struct {
	UsesProfile     bool
	RequiresSecrets bool
}

type CommandSpec struct {
	Name        string
	Family      CommandFamily
	Subcommands []string
	RequiresDSM bool

	ProfilePolicy ProfilePolicy

	parse     func(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error)
	usage     func(meta workflow.Metadata, rt workflow.Env) string
	fullUsage func(meta workflow.Metadata, rt workflow.Env) string
}

func (s CommandSpec) UsageText(meta workflow.Metadata, rt workflow.Env) string {
	if s.usage == nil {
		return ""
	}
	return s.usage(meta, rt)
}

func (s CommandSpec) FullUsageText(meta workflow.Metadata, rt workflow.Env) string {
	if s.fullUsage == nil {
		return ""
	}
	return s.fullUsage(meta, rt)
}

func (s CommandSpec) HasParser() bool {
	return s.parse != nil
}

func (s CommandSpec) HasHelp() bool {
	return s.usage != nil && s.fullUsage != nil
}

var commandRegistry = map[string]CommandSpec{
	"backup": {
		Name:          "backup",
		Family:        CommandFamilyRuntime,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse: func(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
			return parseRuntimeCommandRequest("backup", args, meta, rt)
		},
		usage:     UsageText,
		fullUsage: FullUsageText,
	},
	"cleanup-storage": {
		Name:          "cleanup-storage",
		Family:        CommandFamilyRuntime,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse: func(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
			return parseRuntimeCommandRequest("cleanup-storage", args, meta, rt)
		},
		usage:     UsageText,
		fullUsage: FullUsageText,
	},
	"config": {
		Name:          "config",
		Family:        CommandFamilyConfig,
		Subcommands:   []string{"validate", "explain", "paths"},
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse:         parseConfigRequest,
		usage:         ConfigUsageText,
		fullUsage:     FullConfigUsageText,
	},
	"diagnostics": {
		Name:          "diagnostics",
		Family:        CommandFamilyDiagnostics,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse:         parseDiagnosticsRequest,
		usage:         DiagnosticsUsageText,
		fullUsage:     FullDiagnosticsUsageText,
	},
	"health": {
		Name:          "health",
		Family:        CommandFamilyHealth,
		Subcommands:   []string{"status", "doctor", "verify"},
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse:         parseHealthRequest,
		usage:         UsageText,
		fullUsage:     FullUsageText,
	},
	"notify": {
		Name:          "notify",
		Family:        CommandFamilyNotify,
		Subcommands:   []string{"test", "test update"},
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse:         parseNotifyRequest,
		usage:         NotifyUsageText,
		fullUsage:     FullNotifyUsageText,
	},
	"prune": {
		Name:          "prune",
		Family:        CommandFamilyRuntime,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse: func(args []string, meta workflow.Metadata, rt workflow.Env) (*ParseResult, error) {
			return parseRuntimeCommandRequest("prune", args, meta, rt)
		},
		usage:     UsageText,
		fullUsage: FullUsageText,
	},
	"restore": {
		Name:          "restore",
		Family:        CommandFamilyRestore,
		Subcommands:   []string{"plan", "list-revisions", "run", "select"},
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true, RequiresSecrets: true},
		parse:         parseRestoreRequest,
		usage:         RestoreUsageText,
		fullUsage:     FullRestoreUsageText,
	},
	"rollback": {
		Name:          "rollback",
		Family:        CommandFamilyManagedUpdate,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{},
		parse:         parseRollbackRequest,
		usage:         RollbackUsageText,
		fullUsage:     FullRollbackUsageText,
	},
	"update": {
		Name:          "update",
		Family:        CommandFamilyManagedUpdate,
		RequiresDSM:   true,
		ProfilePolicy: ProfilePolicy{UsesProfile: true},
		parse:         parseUpdateRequest,
		usage:         UpdateUsageText,
		fullUsage:     FullUpdateUsageText,
	},
}

func PublicCommandSpecs() []CommandSpec {
	specs := make([]CommandSpec, 0, len(commandRegistry))
	for _, spec := range commandRegistry {
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func commandSpec(name string) (CommandSpec, bool) {
	spec, ok := commandRegistry[name]
	return spec, ok
}

func ProfilePolicyForRequest(req *workflow.Request) (string, ProfilePolicy) {
	displayName, spec, ok := commandSpecForRequest(req)
	if !ok {
		return displayName, ProfilePolicy{}
	}
	return displayName, spec.ProfilePolicy
}

func RequiresDSMForRequest(req *workflow.Request) bool {
	_, spec, ok := commandSpecForRequest(req)
	if !ok {
		return true
	}
	return spec.RequiresDSM
}

func commandSpecForRequest(req *workflow.Request) (string, CommandSpec, bool) {
	if req == nil {
		return "", CommandSpec{}, false
	}
	if req.ConfigCommand != "" {
		return requestSpec("config", "config "+req.ConfigCommand)
	}
	if req.DiagnosticsCommand != "" {
		return requestSpec("diagnostics", "diagnostics")
	}
	if req.HealthCommand != "" {
		return requestSpec("health", "health "+req.HealthCommand)
	}
	if req.NotifyCommand != "" {
		command := "notify " + req.NotifyCommand
		if req.NotifyScope == "update" {
			command += " update"
		}
		return requestSpec("notify", command)
	}
	if req.RestoreCommand != "" {
		return requestSpec("restore", "restore "+req.RestoreCommand)
	}
	if req.RollbackCommand != "" {
		return requestSpec("rollback", "rollback")
	}
	if req.UpdateCommand != "" {
		return requestSpec("update", "update")
	}
	if req.DoBackup {
		return requestSpec("backup", "backup")
	}
	if req.DoPrune {
		return requestSpec("prune", "prune")
	}
	if req.DoCleanupStore {
		return requestSpec("cleanup-storage", "cleanup-storage")
	}
	return "", CommandSpec{}, false
}

func requestSpec(commandName string, displayName string) (string, CommandSpec, bool) {
	spec, ok := commandSpec(commandName)
	if !ok {
		return displayName, CommandSpec{}, false
	}
	return displayName, spec, true
}
