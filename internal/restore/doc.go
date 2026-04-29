// Package restore owns restore planning, revision listing, workspace
// preparation, guided selection, execution, and restore-specific reports.
//
// Restore remains command-facing rather than a low-level domain package: it
// coordinates workflow planning data, Duplicacy repository reads, workspace
// safety rules, and the interactive tree picker.
package restore
