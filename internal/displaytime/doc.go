// Package displaytime resolves the local timezone for human-readable operator
// output.
//
// Synology DSM can run this program in contexts where Go's time.Local does not
// reflect the device timezone, especially across user and sudo/root execution.
// Operator-facing timestamps should use this package instead of time.Local so
// output follows the NAS system timezone recorded by /etc/localtime.
package displaytime
