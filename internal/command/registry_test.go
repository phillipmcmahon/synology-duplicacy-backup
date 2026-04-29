package command

import (
	"reflect"
	"testing"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
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
		req            *workflow.Request
		command        string
		usesProfile    bool
		requiresSecret bool
	}
	cases := []policyCase{
		{name: "nil request", req: nil},
		{name: "empty request", req: &workflow.Request{}},
		{name: "backup", req: &workflow.Request{Command: "backup", DoBackup: true}, command: "backup", usesProfile: true, requiresSecret: true},
		{name: "prune", req: &workflow.Request{Command: "prune", DoPrune: true}, command: "prune", usesProfile: true, requiresSecret: true},
		{name: "cleanup-storage", req: &workflow.Request{Command: "cleanup-storage", DoCleanupStore: true}, command: "cleanup-storage", usesProfile: true, requiresSecret: true},
		{name: "diagnostics", req: &workflow.Request{Command: "diagnostics", DiagnosticsCommand: "diagnostics"}, command: "diagnostics", usesProfile: true, requiresSecret: true},
		{name: "update", req: &workflow.Request{Command: "update", UpdateCommand: "update"}, command: "update", usesProfile: true},
		{name: "rollback", req: &workflow.Request{Command: "rollback", RollbackCommand: "rollback"}, command: "rollback"},
	}

	for _, subcommand := range []string{"validate", "explain", "paths"} {
		cases = append(cases, policyCase{
			name:           "config " + subcommand,
			req:            &workflow.Request{Command: "config", ConfigCommand: subcommand},
			command:        "config " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, subcommand := range []string{"status", "doctor", "verify"} {
		cases = append(cases, policyCase{
			name:           "health " + subcommand,
			req:            &workflow.Request{Command: "health", HealthCommand: subcommand},
			command:        "health " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	for _, subcommand := range []string{"plan", "list-revisions", "run", "select"} {
		cases = append(cases, policyCase{
			name:           "restore " + subcommand,
			req:            &workflow.Request{Command: "restore", RestoreCommand: subcommand},
			command:        "restore " + subcommand,
			usesProfile:    true,
			requiresSecret: true,
		})
	}
	cases = append(cases,
		policyCase{
			name:           "notify test label",
			req:            &workflow.Request{Command: "notify", NotifyCommand: "test"},
			command:        "notify test",
			usesProfile:    true,
			requiresSecret: true,
		},
		policyCase{
			name:           "notify test update",
			req:            &workflow.Request{Command: "notify", NotifyCommand: "test", NotifyScope: "update"},
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
