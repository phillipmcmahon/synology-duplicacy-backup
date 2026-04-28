package workflow

func shouldPromptForSafety(plan *Plan, stderrInteractive, stdinIsTTY bool) bool {
	if plan == nil {
		return false
	}
	if plan.Request.DryRun {
		return false
	}
	if !stderrInteractive || !stdinIsTTY {
		return false
	}
	return plan.Request.ForcePrune || plan.Request.DoCleanupStore
}

func safetyWarnings(plan *Plan) []string {
	if plan == nil {
		return nil
	}

	var warnings []string
	if plan.Request.ForcePrune {
		warnings = append(warnings, "Forced prune overrides safe prune thresholds and may delete more revisions than a safe prune would allow")
	}
	if plan.Request.DoCleanupStore {
		warnings = append(warnings, "Storage cleanup runs exhaustive exclusive prune and should be used only when no other client is writing to the same storage")
	}
	return warnings
}
