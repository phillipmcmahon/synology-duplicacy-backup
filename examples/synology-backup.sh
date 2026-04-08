#!/usr/bin/env bash
#
# synology-backup.sh — Example deployment & usage script for duplicacy-backup
#
# This script demonstrates how to:
#   1. Deploy the duplicacy-backup binary to a Synology NAS
#   2. Create a valid configuration file (with MANDATORY fields)
#   3. Set up secrets for remote backups
#   4. Run common backup operations
#
# IMPORTANT: LOCAL_OWNER and LOCAL_GROUP are REQUIRED fields in the config.
#            The binary will refuse to start if they are missing.
#            These must be set to a non-root user/group for security.
#
# Usage:
#   chmod +x synology-backup.sh
#   sudo ./synology-backup.sh
#
# Adjust the variables below to match your environment.
# ---------------------------------------------------------------------------

set -euo pipefail

# ========================== CONFIGURATION ==================================

# Where the binary lives on the NAS
BINARY_DIR="/usr/local/bin"
BINARY_NAME="duplicacy-backup"
BINARY_PATH="${BINARY_DIR}/${BINARY_NAME}"

# Config & secrets directories
CONFIG_DIR="${BINARY_DIR}/.config"
SECRETS_DIR="/root/.secrets"

# Backup label — matches the config filename: <LABEL>-backup.conf
LABEL="homes"

# --------------------------------------------------------------------------
# MANDATORY: LOCAL_OWNER and LOCAL_GROUP
#
# The duplicacy-backup binary runs as root (required for btrfs snapshots),
# but backup repository files should be owned by a regular (non-root) user
# for security.  These two fields are REQUIRED — the binary will exit with
# an error if they are missing or set to "root".
# --------------------------------------------------------------------------
LOCAL_OWNER="myuser"       # REQUIRED — change to the Synology user account
LOCAL_GROUP="users"        # REQUIRED — change to the appropriate group

# Backup destinations
LOCAL_DESTINATION="/volume2/backups"
REMOTE_DESTINATION="s3://gateway.storjshare.io/my-backup-bucket"

# Thread counts (must be power of 2, max 16)
LOCAL_THREADS=4
REMOTE_THREADS=8

# Prune retention policy
PRUNE_POLICY="-keep 1:728 -keep 91:364 -keep 28:182 -keep 7:28"

# Filter pattern (Duplicacy regex syntax)
FILTER='e:^(.*/)?(@eaDir|#recycle|tmp|exclude)/$|^(.*/)?(\..DS_Store|\._.*|Thumbs\.db)$'

# Safe-prune thresholds
SAFE_PRUNE_MAX_DELETE_PERCENT=10
SAFE_PRUNE_MAX_DELETE_COUNT=25
SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT=20

LOG_RETENTION_DAYS=30

# ========================== HELPER FUNCTIONS ================================

info()  { echo "[INFO]  $*"; }
warn()  { echo "[WARN]  $*" >&2; }
error() { echo "[ERROR] $*" >&2; exit 1; }

require_root() {
  [[ "$(id -u)" -eq 0 ]] || error "This script must be run as root (sudo)."
}

# ========================== SETUP ==========================================

setup_config() {
  info "Creating config directory: ${CONFIG_DIR}"
  mkdir -p "${CONFIG_DIR}"

  local conf="${CONFIG_DIR}/${LABEL}-backup.conf"
  info "Writing config file: ${conf}"

  cat > "${conf}" <<EOF
# Auto-generated config for duplicacy-backup
# Source label: ${LABEL}
#
# IMPORTANT: LOCAL_OWNER and LOCAL_GROUP are MANDATORY.
#            The binary will refuse to start without them.

[common]
PRUNE=${PRUNE_POLICY}
FILTER=${FILTER}

# REQUIRED — local file ownership (must NOT be root)
LOCAL_OWNER=${LOCAL_OWNER}
LOCAL_GROUP=${LOCAL_GROUP}

LOG_RETENTION_DAYS=${LOG_RETENTION_DAYS}

# Safe prune thresholds
SAFE_PRUNE_MAX_DELETE_PERCENT=${SAFE_PRUNE_MAX_DELETE_PERCENT}
SAFE_PRUNE_MAX_DELETE_COUNT=${SAFE_PRUNE_MAX_DELETE_COUNT}
SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT=${SAFE_PRUNE_MIN_TOTAL_FOR_PERCENT}

[local]
DESTINATION=${LOCAL_DESTINATION}
THREADS=${LOCAL_THREADS}

[remote]
DESTINATION=${REMOTE_DESTINATION}
THREADS=${REMOTE_THREADS}
EOF

  chmod 644 "${conf}"
  info "Config written successfully."
}

setup_secrets() {
  info "Creating secrets directory: ${SECRETS_DIR}"
  mkdir -p "${SECRETS_DIR}"
  chmod 700 "${SECRETS_DIR}"

  local env_file="${SECRETS_DIR}/duplicacy-${LABEL}.env"

  if [[ -f "${env_file}" ]]; then
    info "Secrets file already exists: ${env_file} — skipping."
    return
  fi

  info "Creating placeholder secrets file: ${env_file}"
  cat > "${env_file}" <<'EOF'
# Storj S3 credentials for remote backup
# Replace with real values before running remote backups.
STORJ_S3_ID=replace-with-your-access-key-id
STORJ_S3_SECRET=replace-with-your-secret-access-key
EOF

  chown root:root "${env_file}"
  chmod 600 "${env_file}"
  warn "Secrets file created with placeholders — edit before using remote mode!"
}

# ========================== VALIDATE =======================================

validate_env() {
  require_root

  [[ -x "${BINARY_PATH}" ]] || error "Binary not found or not executable: ${BINARY_PATH}"

  # Validate mandatory fields are not empty
  [[ -n "${LOCAL_OWNER}" ]] || error "LOCAL_OWNER is REQUIRED but not set."
  [[ -n "${LOCAL_GROUP}" ]] || error "LOCAL_GROUP is REQUIRED but not set."

  # Validate mandatory fields are not root
  [[ "${LOCAL_OWNER}" != "root" ]] || error "LOCAL_OWNER must NOT be root."

  info "Environment validated."
}

# ========================== BACKUP OPERATIONS ==============================

run_local_backup() {
  info "Running local backup for label: ${LABEL}"
  "${BINARY_PATH}" "${LABEL}"
}

run_remote_backup() {
  info "Running remote backup for label: ${LABEL}"
  "${BINARY_PATH}" --remote "${LABEL}"
}

run_prune() {
  info "Running safe prune for label: ${LABEL}"
  "${BINARY_PATH}" --prune "${LABEL}"
}

run_dry_run() {
  info "Running dry-run backup for label: ${LABEL}"
  "${BINARY_PATH}" --dry-run "${LABEL}"
}

run_full_cycle() {
  info "Running full backup cycle: remote backup → local prune"
  "${BINARY_PATH}" --remote "${LABEL}" && "${BINARY_PATH}" --prune "${LABEL}"
}

# ========================== MAIN ===========================================

usage() {
  cat <<EOF
Usage: $(basename "$0") <command>

Commands:
  setup        Create config and secrets files
  backup       Run local backup
  remote       Run remote backup
  prune        Run safe prune
  dry-run      Simulate backup (no changes)
  full-cycle   Remote backup + local prune
  help         Show this message

IMPORTANT: LOCAL_OWNER and LOCAL_GROUP are MANDATORY config fields.
           The binary will refuse to start without them.
EOF
}

main() {
  local cmd="${1:-help}"

  case "${cmd}" in
    setup)
      require_root
      setup_config
      setup_secrets
      info "Setup complete. Edit secrets before using remote mode."
      ;;
    backup)
      validate_env
      run_local_backup
      ;;
    remote)
      validate_env
      run_remote_backup
      ;;
    prune)
      validate_env
      run_prune
      ;;
    dry-run)
      validate_env
      run_dry_run
      ;;
    full-cycle)
      validate_env
      run_full_cycle
      ;;
    help|--help|-h)
      usage
      ;;
    *)
      error "Unknown command: ${cmd}. Run '$(basename "$0") help' for usage."
      ;;
  esac
}

main "$@"
