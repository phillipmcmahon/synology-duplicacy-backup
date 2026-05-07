package health

import (
	"fmt"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/workflow"
)

func (h *HealthRunner) evaluateFreshness(report *Report, cfg config.HealthConfig, last time.Time, checkName string) {
	age := h.rt.Now().Sub(last)
	warnAfter := time.Duration(cfg.FreshnessWarnHours) * time.Hour
	failAfter := time.Duration(cfg.FreshnessFailHours) * time.Hour
	ageText := fmt.Sprintf("%s old", HumanAge(age))
	switch {
	case failAfter > 0 && age > failAfter:
		report.AddCheck(checkName, "fail", fmt.Sprintf("Backup freshness failure: latest successful backup is %s; %s", ageText, healthPolicyWindow("freshness_fail_hours", cfg.FreshnessFailHours)))
	case warnAfter > 0 && age > warnAfter:
		report.AddCheck(checkName, "warn", fmt.Sprintf("Backup freshness warning: latest successful backup is %s; %s", ageText, healthPolicyWindow("freshness_warn_hours", cfg.FreshnessWarnHours)))
	default:
		report.AddCheck(checkName, "pass", ageText)
	}
}

func (h *HealthRunner) evaluateHealthRecency(report *Report, cfg config.HealthConfig, kind, name string) {
	var thresholdHours int
	switch kind {
	case "doctor":
		thresholdHours = cfg.DoctorWarnAfter
	case "verify":
		thresholdHours = cfg.VerifyWarnAfter
	default:
		return
	}

	_, at, ok := h.loadHealthRecencyTime(report, kind)
	if !ok {
		report.AddCheck(name, "warn", fmt.Sprintf("Health check cadence warning: no prior %s run is recorded; %s", kind, healthPolicyWindow(healthRecencyPolicyName(kind), thresholdHours)))
		return
	}
	age := h.rt.Now().Sub(at)
	if thresholdHours > 0 && age > time.Duration(thresholdHours)*time.Hour {
		report.AddCheck(name, "warn", fmt.Sprintf("Health check cadence warning: last %s run was %s; %s", kind, HumanAgo(age), healthPolicyWindow(healthRecencyPolicyName(kind), thresholdHours)))
		return
	}
	report.AddCheck(name, "pass", HumanAgo(age))
}

func healthPolicyWindow(name string, hours int) string {
	if hours <= 0 {
		return fmt.Sprintf("policy %s=%d", name, hours)
	}
	return fmt.Sprintf("policy %s=%d (%s)", name, hours, HumanAge(time.Duration(hours)*time.Hour))
}

func healthRecencyPolicyName(kind string) string {
	switch kind {
	case "doctor":
		return "doctor_warn_after_hours"
	case "verify":
		return "verify_warn_after_hours"
	default:
		return "health_check_warn_after_hours"
	}
}

func (h *HealthRunner) populateHealthRecencyTimestamp(report *Report, kind string) bool {
	_, _, ok := h.loadHealthRecencyTime(report, kind)
	return ok
}

func (h *HealthRunner) loadHealthRecencyTime(report *Report, kind string) (*RunState, time.Time, bool) {
	state, err := workflow.LoadRunState(h.meta, report.Label, report.Target)
	if err != nil || state == nil {
		return nil, time.Time{}, false
	}

	var last string
	switch kind {
	case "doctor":
		last = state.LastDoctorAt
	case "verify":
		last = state.LastVerifyAt
	default:
		return state, time.Time{}, false
	}
	if last == "" {
		return state, time.Time{}, false
	}
	at, parseErr := time.Parse(time.RFC3339, last)
	if parseErr != nil {
		return state, time.Time{}, false
	}
	switch kind {
	case "doctor":
		report.LastDoctorRunAt = formatReportTime(at)
	case "verify":
		report.LastVerifyRunAt = formatReportTime(at)
	}
	return state, at, true
}

func chooseLocalSuccessTime(state *RunState) string {
	if state == nil {
		return ""
	}
	if state.LastSuccessfulBackupAt != "" {
		return state.LastSuccessfulBackupAt
	}
	return state.LastSuccessfulRunAt
}
