package command

import (
	"reflect"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflowcore"
)

func TestPublicCommandSpecsCoverCommandSurface(t *testing.T) {
	specs := PublicCommandSpecs()
	got := make([]string, 0, len(specs))
	for _, spec := range specs {
		got = append(got, spec.Name)
		if !spec.HasParser() {
			t.Fatalf("%s has no parser", spec.Name)
		}
		if !spec.HasHelp() {
			t.Fatalf("%s has incomplete help coverage", spec.Name)
		}
		if !spec.RequiresDSM {
			t.Fatalf("%s must declare the DSM runtime requirement", spec.Name)
		}
		switch spec.Family {
		case CommandFamilyRuntime, CommandFamilyDiagnostics, CommandFamilyManagedUpdate:
			if len(spec.Subcommands) != 0 {
				t.Fatalf("%s unexpectedly declares subcommands: %v", spec.Name, spec.Subcommands)
			}
		case CommandFamilyConfig, CommandFamilyHealth, CommandFamilyNotify, CommandFamilyRestore:
			if len(spec.Subcommands) == 0 {
				t.Fatalf("%s must declare subcommand coverage", spec.Name)
			}
		default:
			t.Fatalf("%s has unknown family %q", spec.Name, spec.Family)
		}
	}

	want := []string{
		"backup",
		"cleanup-storage",
		"config",
		"diagnostics",
		"health",
		"notify",
		"prune",
		"restore",
		"rollback",
		"update",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public commands = %v, want %v", got, want)
	}
}

func TestProfilePolicyForRequestCoversCommandSurface(t *testing.T) {
	type policyCase struct {
		name           string
		req            *workflowcore.Request
		command        string
		usesProfile    bool
		requiresSecret bool
	}
	cases := []policyCase{
		{name: "nil request", req: nil},
		{name: "empty request", req: &workflowcore.Request{}},
		{name: "backup", req: &workflowcore.Request{Command: "backup", DoBackup: true}, command: "backup", usesProfile: true, requiresSecret: true},
		{name: "prune", req: &workflowcore.Request{Command: "prune", DoPrune: true}, command: "prune", usesProfile: true, requiresSecret: true},
		{name: "cleanup-storage", req: &workflowcore.Request{Command: "cleanup-storage", DoCleanupStore: true}, command: "cleanup-storage", usesProfile: true, requiresSecret: true},
		{name: "diagnostics", req: &workflowcore.Request{Command: "diagnostics", DiagnosticsCommand: "diagnostics"}, command: "diagnostics", usesProfile: true, requiresSecret: true},
		{name: "update", req: &workflowcore.Request{Command: "update", UpdateCommand: "update"}, command: "update", usesProfile: true},
		{name: "rollback", req: &workflowcore.Request{Command: "rollback", RollbackCommand: "rollback"}, command: "rollback"},
	}

	for _, subcommand := range []string{"validate", "explain", "paths"} {
		cases = append(cases, policyCase{
			name:           "config " + subcommand,
			req:            &workflowcore.Request{Command: "config", ConfigCommand: subcommand},
			command:        "config " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, subcommand := range []string{"status", "doctor", "verify"} {
		cases = append(cases, policyCase{
			name:           "health " + subcommand,
			req:            &workflowcore.Request{Command: "health", HealthCommand: subcommand},
			command:        "health " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, subcommand := range []string{"plan", "list-revisions", "run", "select"} {
		cases = append(cases, policyCase{
			name:           "restore " + subcommand,
			req:            &workflowcore.Request{Command: "restore", RestoreCommand: subcommand},
			command:        "restore " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	cases = append(cases,
		policyCase{
			name:           "notify test label",
			req:            &workflowcore.Request{Command: "notify", NotifyCommand: "test"},
			command:        "notify test",
			usesProfile:    true,
			requiresSecret: true,
		},
		policyCase{
			name:           "notify test update",
			req:            &workflowcore.Request{Command: "notify", NotifyCommand: "test", NotifyScope: "update"},
			command:        "notify test update",
			usesProfile:    true,
			requiresSecret: true,
		},
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			commandName, policy := ProfilePolicyForRequest(tc.req)
			if commandName != tc.command || policy.UsesProfile != tc.usesProfile || policy.RequiresSecrets != tc.requiresSecret {
				t.Fatalf("policy = %q %+v, want command=%q usesProfile=%v requiresSecrets=%v", commandName, policy, tc.command, tc.usesProfile, tc.requiresSecret)
			}
			if tc.command != "" && !RequiresDSMForRequest(tc.req) {
				t.Fatalf("RequiresDSMForRequest(%s) = false, want true", tc.name)
			}
		})
	}
}

func TestRegistryParsersSetCommandDiscriminator(t *testing.T) {
	meta := workflow.MetadataForLogDir("duplicacy-backup", "1.0.0", "now", t.TempDir())
	rt := workflow.DefaultEnv()
	argsByCommand := map[string][]string{
		"backup":          {"--target", "onsite-usb", "homes"},
		"cleanup-storage": {"--target", "onsite-usb", "homes"},
		"config":          {"validate", "--target", "onsite-usb", "homes"},
		"diagnostics":     {"--target", "onsite-usb", "homes"},
		"health":          {"status", "--target", "onsite-usb", "homes"},
		"notify":          {"test", "--target", "onsite-usb", "homes"},
		"prune":           {"--target", "onsite-usb", "homes"},
		"restore":         {"plan", "--target", "onsite-usb", "homes"},
		"rollback":        {"--check-only"},
		"update":          {"--check-only"},
	}

	for _, spec := range PublicCommandSpecs() {
		t.Run(spec.Name, func(t *testing.T) {
			result, err := spec.parse(argsByCommand[spec.Name], meta, rt)
			if err != nil {
				t.Fatalf("parse(%s) error = %v", spec.Name, err)
			}
			if result == nil || result.Request == nil {
				t.Fatalf("parse(%s) did not return a request: %+v", spec.Name, result)
			}
			if result.Request.Command != spec.Name {
				t.Fatalf("request command = %q, want %q", result.Request.Command, spec.Name)
			}
		})
	}
}
