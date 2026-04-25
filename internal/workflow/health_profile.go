package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (h *HealthRunner) addRootProfileConfigWarning(report *HealthReport, req *HealthRequest) {
	if report == nil || req == nil {
		return
	}
	message := rootProfileConfigWarning(req, h.rt, rootProfileCandidateConfigFiles(req.Label, h.rt))
	if message == "" {
		return
	}
	report.AddCheck("Root config profile", "warn", message)
}

func rootProfileConfigWarning(req *HealthRequest, rt Runtime, candidateFiles []string) string {
	if req == nil || req.Command != "doctor" || req.Label == "" {
		return ""
	}
	if rt.Geteuid == nil || rt.Geteuid() != 0 {
		return ""
	}
	configDir := strings.TrimSpace(req.ConfigDir)
	if configDir == "" {
		configDir = EffectiveConfigDir(rt)
	}
	rootConfigDir := filepath.Clean("/root/.config/duplicacy-backup")
	if filepath.Clean(configDir) != rootConfigDir {
		return ""
	}
	for _, candidate := range candidateFiles {
		candidate = filepath.Clean(candidate)
		if candidate == "" || strings.HasPrefix(candidate, rootConfigDir+string(os.PathSeparator)) {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return fmt.Sprintf("Running health doctor as root is using %s, but %s exists; rerun as the operator user or pass --config-dir %s", rootConfigDir, candidate, filepath.Dir(candidate))
		}
	}
	return ""
}

func rootProfileCandidateConfigFiles(label string, rt Runtime) []string {
	filename := label + "-backup.toml"
	candidates := make([]string, 0, 8)
	if sudoUser := strings.TrimSpace(rt.Getenv("SUDO_USER")); sudoUser != "" && sudoUser != "root" {
		for _, home := range []string{
			filepath.Join("/volume1/homes", sudoUser),
			filepath.Join("/home", sudoUser),
			filepath.Join("/Users", sudoUser),
		} {
			candidates = append(candidates, filepath.Join(home, ".config", "duplicacy-backup", filename))
		}
	}
	for _, pattern := range []string{
		filepath.Join("/volume1/homes", "*", ".config", "duplicacy-backup", filename),
		filepath.Join("/home", "*", ".config", "duplicacy-backup", filename),
		filepath.Join("/Users", "*", ".config", "duplicacy-backup", filename),
	} {
		if matches, err := filepath.Glob(pattern); err == nil {
			candidates = append(candidates, matches...)
		}
	}
	return candidates
}
