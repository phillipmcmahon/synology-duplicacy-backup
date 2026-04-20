package notify

import "strings"

type EventID string

const (
	EventNotificationTest         EventID = "notification_test"
	EventBackupCouldNotStart      EventID = "backup_could_not_start"
	EventBackupFailed             EventID = "backup_failed"
	EventSafePruneBlocked         EventID = "safe_prune_blocked"
	EventPruneFailed              EventID = "prune_failed"
	EventCleanupFailed            EventID = "cleanup_failed"
	EventVerifyFailedRevisions    EventID = "verify_failed_revisions"
	EventFreshnessFailed          EventID = "freshness_failed"
	EventHealthDegraded           EventID = "health_degraded"
	EventHealthUnhealthy          EventID = "health_unhealthy"
	EventUpdateCheckFailed        EventID = "update_check_failed"
	EventUpdateDownloadFailed     EventID = "update_download_failed"
	EventUpdateChecksumFailed     EventID = "update_checksum_failed"
	EventUpdateAttestationFailed  EventID = "update_attestation_failed"
	EventUpdateInstallFailed      EventID = "update_install_failed"
	EventUpdateInstallSucceeded   EventID = "update_install_succeeded"
	EventUpdateAlreadyCurrent     EventID = "update_already_current"
	EventUpdateReinstallRequested EventID = "update_reinstall_requested"
)

func KnownEvents() []EventID {
	return []EventID{
		EventNotificationTest,
		EventBackupCouldNotStart,
		EventBackupFailed,
		EventSafePruneBlocked,
		EventPruneFailed,
		EventCleanupFailed,
		EventVerifyFailedRevisions,
		EventFreshnessFailed,
		EventHealthDegraded,
		EventHealthUnhealthy,
		EventUpdateCheckFailed,
		EventUpdateDownloadFailed,
		EventUpdateChecksumFailed,
		EventUpdateAttestationFailed,
		EventUpdateInstallFailed,
		EventUpdateInstallSucceeded,
		EventUpdateAlreadyCurrent,
		EventUpdateReinstallRequested,
	}
}

func IsKnownEvent(event string) bool {
	event = strings.TrimSpace(event)
	if event == "" {
		return true
	}
	for _, known := range KnownEvents() {
		if string(known) == event {
			return true
		}
	}
	return false
}
