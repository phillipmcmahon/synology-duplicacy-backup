package workflow

import (
	"fmt"
	"time"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	healthpkg "github.com/phillipmcmahon/synology-duplicacy-backup/internal/health"
)

func (h *HealthRunner) evaluateFreshness(report *HealthReport, cfg config.HealthConfig, last time.Time, checkName string) {
	age := h.rt.Now().Sub(last)
	warnAfter := time.Duration(cfg.FreshnessWarnHours) * time.Hour
	failAfter := time.Duration(cfg.FreshnessFailHours) * time.Hour
	message := fmt.Sprintf("%s old", healthpkg.HumanAge(age))
	switch {
	case failAfter > 0 && age > failAfter:
		report.AddCheck(checkName, "fail", message)
	case warnAfter > 0 && age > warnAfter:
		report.AddCheck(checkName, "warn", message)
	default:
		report.AddCheck(checkName, "pass", message)
	}
}

func (h *HealthRunner) evaluateHealthRecency(report *HealthReport, cfg config.HealthConfig, kind, name string) {
	_, at, ok := h.loadHealthRecencyTime(report, kind)
	if !ok {
		report.AddCheck(name, "warn", "No prior health state is available")
		return
	}
	var thresholdHours int
	switch kind {
	case "doctor":
		thresholdHours = cfg.DoctorWarnAfter
	case "verify":
		thresholdHours = cfg.VerifyWarnAfter
	default:
		return
	}
	age := h.rt.Now().Sub(at)
	if thresholdHours > 0 && age > time.Duration(thresholdHours)*time.Hour {
		report.AddCheck(name, "warn", healthpkg.HumanAgo(age))
		return
	}
	report.AddCheck(name, "pass", healthpkg.HumanAgo(age))
}

func (h *HealthRunner) populateHealthRecencyTimestamp(report *HealthReport, kind string) bool {
	_, _, ok := h.loadHealthRecencyTime(report, kind)
	return ok
}

func (h *HealthRunner) loadHealthRecencyTime(report *HealthReport, kind string) (*RunState, time.Time, bool) {
	state, err := loadRunState(h.meta, report.Label, report.Target)
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

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
