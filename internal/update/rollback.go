package update

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type rollbackCandidate struct {
	Version string
	Name    string
	Active  bool
}

type rollbackPlan struct {
	CurrentVersion string
	TargetVersion  string
	TargetName     string
	InstallRoot    string
	BinDir         string
	CheckOnly      bool
	Explicit       bool
	Candidates     []rollbackCandidate
}

func (u *Updater) RollbackResult(options RollbackOptions) (RollbackResult, error) {
	planned, err := u.buildRollbackPlan(options)
	if err != nil {
		return RollbackResult{}, err
	}
	if planned.CheckOnly {
		return RollbackResult{Output: renderRollbackReport(planned, "Ready to rollback", "")}, nil
	}
	if err := u.confirmRollback(planned, options); err != nil {
		return RollbackResult{}, err
	}
	activation, err := activateRollback(planned)
	if err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{Output: renderRollbackReport(planned, "Rolled back", activation)}, nil
}

func (u *Updater) buildRollbackPlan(options RollbackOptions) (*rollbackPlan, error) {
	layout, err := u.detectManagedLayout()
	if err != nil {
		return nil, err
	}
	candidates, err := installedRollbackCandidates(layout.InstallRoot, layout.ResolvedPath)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("rollback found no retained versioned binaries in %s", layout.InstallRoot)
	}
	target, explicit, err := selectRollbackTarget(candidates, options.RequestedVersion)
	if err != nil {
		return nil, err
	}
	return &rollbackPlan{
		CurrentVersion: installedVersionFromPath(layout.ResolvedPath),
		TargetVersion:  target.Version,
		TargetName:     target.Name,
		InstallRoot:    layout.InstallRoot,
		BinDir:         layout.BinDir,
		CheckOnly:      options.CheckOnly,
		Explicit:       explicit,
		Candidates:     candidates,
	}, nil
}

func installedRollbackCandidates(installRoot, activePath string) ([]rollbackCandidate, error) {
	entries, err := os.ReadDir(installRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect install root %s: %w", installRoot, err)
	}
	var candidates []rollbackCandidate
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := versionedBinaryPattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}
		path := filepath.Join(installRoot, entry.Name())
		resolvedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			resolvedPath = path
		}
		candidates = append(candidates, rollbackCandidate{
			Version: matches[1],
			Name:    entry.Name(),
			Active:  resolvedPath == activePath,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return compareVersions(candidates[i].Version, candidates[j].Version) > 0
	})
	return candidates, nil
}

func selectRollbackTarget(candidates []rollbackCandidate, requestedVersion string) (rollbackCandidate, bool, error) {
	requestedVersion = strings.TrimPrefix(strings.TrimSpace(requestedVersion), "v")
	if requestedVersion != "" {
		for _, candidate := range candidates {
			if candidate.Version == requestedVersion {
				if candidate.Active {
					return rollbackCandidate{}, true, fmt.Errorf("rollback target %s is already active", ensureTagPrefix(requestedVersion))
				}
				return candidate, true, nil
			}
		}
		return rollbackCandidate{}, true, fmt.Errorf("rollback target %s is not retained in the managed install root", ensureTagPrefix(requestedVersion))
	}
	for _, candidate := range candidates {
		if !candidate.Active {
			return candidate, false, nil
		}
	}
	return rollbackCandidate{}, false, errors.New("rollback found no previous retained version; update retention may have kept only the active binary")
}

func activateRollback(planned *rollbackPlan) (string, error) {
	currentLink := filepath.Join(planned.InstallRoot, "current")
	tempLink := filepath.Join(planned.InstallRoot, ".current.rollback")
	_ = os.Remove(tempLink)
	if err := os.Symlink(planned.TargetName, tempLink); err != nil {
		return "", fmt.Errorf("failed to create rollback symlink %s: %w", tempLink, err)
	}
	if err := os.Rename(tempLink, currentLink); err != nil {
		_ = os.Remove(tempLink)
		return "", fmt.Errorf("failed to activate rollback target %s: %w", planned.TargetName, err)
	}
	return fmt.Sprintf("Activated: %s -> %s\nStable command path: %s", currentLink, planned.TargetName, filepath.Join(planned.BinDir, "duplicacy-backup")), nil
}

func (u *Updater) confirmRollback(planned *rollbackPlan, options RollbackOptions) error {
	if options.Yes {
		return nil
	}
	if !u.Runtime.StdinIsTTY() {
		return errors.New("rollback activation requires --yes when not attached to a terminal; use --check-only to inspect the rollback plan first")
	}
	fmt.Print(renderRollbackReport(planned, "Ready to rollback", ""))
	fmt.Print("Proceed with rollback? [y/N]: ")
	reader := bufio.NewReader(u.Runtime.Stdin())
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to read rollback confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return errors.New("rollback cancelled at the interactive confirmation prompt")
	}
	return nil
}

func renderRollbackReport(planned *rollbackPlan, result string, activation string) string {
	var b strings.Builder
	b.WriteString("Rollback\n")
	fmt.Fprintf(&b, "  Current Version      : %s\n", formatVersion(planned.CurrentVersion))
	fmt.Fprintf(&b, "  Target Version       : %s\n", formatVersion(planned.TargetVersion))
	fmt.Fprintf(&b, "  Install Root         : %s\n", planned.InstallRoot)
	fmt.Fprintf(&b, "  Bin Dir              : %s\n", planned.BinDir)
	fmt.Fprintf(&b, "  Check Only           : %t\n", planned.CheckOnly)
	fmt.Fprintf(&b, "  Explicit Version     : %t\n", planned.Explicit)
	fmt.Fprintf(&b, "  Result               : %s\n", result)
	if len(planned.Candidates) > 0 {
		b.WriteString("  Section: Retained Versions\n")
		for _, candidate := range planned.Candidates {
			marker := "available"
			if candidate.Active {
				marker = "current"
			}
			fmt.Fprintf(&b, "    %-18s : %s\n", formatVersion(candidate.Version), marker)
		}
	}
	if strings.TrimSpace(activation) != "" {
		b.WriteString("  Section: Activation\n")
		scanner := bufio.NewScanner(strings.NewReader(activation))
		for scanner.Scan() {
			fmt.Fprintf(&b, "    %s\n", scanner.Text())
		}
	}
	return b.String()
}

func installedVersionFromPath(path string) string {
	matches := versionedBinaryPattern.FindStringSubmatch(filepath.Base(path))
	if len(matches) == 3 {
		return matches[1]
	}
	return ""
}

func compareVersions(left, right string) int {
	leftParts := versionParts(left)
	rightParts := versionParts(right)
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for i := 0; i < max; i++ {
		var l, r int
		if i < len(leftParts) {
			l = leftParts[i]
		}
		if i < len(rightParts) {
			r = rightParts[i]
		}
		if l != r {
			if l > r {
				return 1
			}
			return -1
		}
	}
	return strings.Compare(left, right)
}

func versionParts(version string) []int {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	fields := strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		value, err := strconv.Atoi(field)
		if err != nil {
			break
		}
		parts = append(parts, value)
	}
	return parts
}
