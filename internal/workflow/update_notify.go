package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/config"
	"github.com/phillipmcmahon/synology-duplicacy-backup/internal/notify"
)

const updateNotifyScope = "update notification config"

func UpdateNotifyConfigPath(req *Request, rt Runtime) string {
	configDirFlag := ""
	if req != nil {
		configDirFlag = req.ConfigDir
	}
	configDir := ResolveDir(rt, configDirFlag, "DUPLICACY_BACKUP_CONFIG_DIR", ExecutableConfigDir(rt))
	return filepath.Join(configDir, config.DefaultAppConfigFile)
}

func LoadUpdateNotifyConfig(req *Request, rt Runtime) (config.HealthNotifyConfig, string, bool, error) {
	path := UpdateNotifyConfigPath(req, rt)
	appCfg, err := config.LoadAppConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.HealthNotifyConfig{}, path, false, nil
		}
		return config.HealthNotifyConfig{}, path, false, err
	}
	return appCfg.Update.Notify, path, true, nil
}

func MaybeSendUpdateFailureNotification(req *Request, meta Metadata, rt Runtime, updateStatus UpdateStatus, updateErr error) error {
	if updateErr == nil {
		return nil
	}
	status := updateStatus
	if status == UpdateStatusUnknown {
		status = UpdateStatusFailed
	}
	return maybeSendUpdateNotification(req, rt, string(status), classifyUpdateFailureEvent(updateErr), "warning", updateFailureSummary(updateErr), map[string]any{
		"message":         OperatorMessage(updateErr),
		"current_version": meta.Version,
		"check_only":      updateCheckOnly(req),
		"force":           updateForce(req),
	})
}

func MaybeSendUpdateSuccessNotification(req *Request, meta Metadata, rt Runtime, updateStatus UpdateStatus) error {
	status, event, summary := classifyUpdateSuccessStatus(updateStatus)
	if status == "" {
		return nil
	}
	return maybeSendUpdateNotification(req, rt, status, event, "info", summary, map[string]any{
		"message":         summary,
		"current_version": meta.Version,
		"check_only":      updateCheckOnly(req),
		"force":           updateForce(req),
	})
}

func BuildUpdateTestNotificationPayload(req *Request, meta Metadata, rt Runtime) *notify.Payload {
	event := strings.TrimSpace(req.NotifyEvent)
	if event == "" {
		event = string(notify.EventUpdateInstallFailed)
	}
	status := updateStatusForEvent(event)
	summary := strings.TrimSpace(req.NotifySummary)
	if summary == "" {
		summary = updateTestSummary(event)
	}
	message := strings.TrimSpace(req.NotifyMessage)
	if message == "" {
		message = "This is a simulated operator-initiated update notification."
	}
	return notify.NewPayload(rt.Now(), rt.Getpid(), req.NotifySeverity, "maintenance", event, summary,
		"", "", "", "update", "", status,
		map[string]any{
			"message":         message,
			"current_version": meta.Version,
			"simulated":       true,
		},
	)
}

func maybeSendUpdateNotification(req *Request, rt Runtime, status, event, severity, summary string, details map[string]any) error {
	cfg, _, ok, err := LoadUpdateNotifyConfig(req, rt)
	if err != nil || !ok {
		return err
	}
	if !shouldSendUpdateNotification(rt, cfg, status) {
		return nil
	}
	payload := notify.NewPayload(rt.Now(), rt.Getpid(), severity, "maintenance", event, summary,
		"", "", "", "update", "", status, details,
	)
	return notify.SendConfigured(cfg, "", "", payload)
}

func updateCheckOnly(req *Request) bool {
	return req != nil && req.UpdateCheckOnly
}

func updateForce(req *Request) bool {
	return req != nil && req.UpdateForce
}

func shouldSendUpdateNotification(rt Runtime, cfg config.HealthNotifyConfig, status string) bool {
	if !notify.HasDestination(cfg) {
		return false
	}
	if rt.StdinIsTTY() && !cfg.Interactive {
		return false
	}
	return containsString(cfg.NotifyOn, status)
}

func classifyUpdateFailureEvent(err error) string {
	message := strings.ToLower(OperatorMessage(err))
	switch {
	case strings.Contains(message, "checksum"):
		return string(notify.EventUpdateChecksumFailed)
	case strings.Contains(message, "attestation"):
		return string(notify.EventUpdateAttestationFailed)
	case strings.Contains(message, "download"):
		return string(notify.EventUpdateDownloadFailed)
	case strings.Contains(message, "install") || strings.Contains(message, "extract") || strings.Contains(message, "staging"):
		return string(notify.EventUpdateInstallFailed)
	default:
		return string(notify.EventUpdateCheckFailed)
	}
}

func updateFailureSummary(err error) string {
	switch classifyUpdateFailureEvent(err) {
	case string(notify.EventUpdateDownloadFailed):
		return "Duplicacy Backup update download failed"
	case string(notify.EventUpdateChecksumFailed):
		return "Duplicacy Backup update checksum verification failed"
	case string(notify.EventUpdateAttestationFailed):
		return "Duplicacy Backup update attestation verification failed"
	case string(notify.EventUpdateInstallFailed):
		return "Duplicacy Backup update install failed"
	default:
		return "Duplicacy Backup update check failed"
	}
}

func classifyUpdateSuccessStatus(updateStatus UpdateStatus) (string, string, string) {
	switch updateStatus {
	case UpdateStatusInstalled:
		return "succeeded", string(notify.EventUpdateInstallSucceeded), "Duplicacy Backup update installed"
	case UpdateStatusCurrent:
		return "current", string(notify.EventUpdateAlreadyCurrent), "Duplicacy Backup is already up to date"
	case UpdateStatusReinstallRequested:
		return "reinstall-requested", string(notify.EventUpdateReinstallRequested), "Duplicacy Backup update reinstall requested"
	default:
		return "", "", ""
	}
}

func updateStatusForEvent(event string) string {
	switch event {
	case string(notify.EventUpdateInstallSucceeded):
		return "succeeded"
	case string(notify.EventUpdateAlreadyCurrent):
		return "current"
	case string(notify.EventUpdateReinstallRequested):
		return "reinstall-requested"
	default:
		return "failed"
	}
}

func updateTestSummary(event string) string {
	status := updateStatusForEvent(event)
	if status == "failed" {
		return fmt.Sprintf("Duplicacy Backup simulated %s", strings.ReplaceAll(event, "_", " "))
	}
	return "Duplicacy Backup update notification test"
}
