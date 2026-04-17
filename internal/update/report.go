package update

import (
	"bufio"
	"fmt"
	"strings"
)

func renderReport(planned *plan, result string, installerOutput string) string {
	var b strings.Builder
	b.WriteString("Update\n")
	fmt.Fprintf(&b, "  Current Version      : %s\n", formatVersion(planned.CurrentVersion))
	fmt.Fprintf(&b, "  Target Version       : %s\n", planned.ReleaseTag)
	fmt.Fprintf(&b, "  Asset                : %s\n", planned.AssetName)
	fmt.Fprintf(&b, "  Install Root         : %s\n", planned.InstallRoot)
	fmt.Fprintf(&b, "  Bin Dir              : %s\n", planned.BinDir)
	fmt.Fprintf(&b, "  Keep                 : %d\n", planned.Keep)
	fmt.Fprintf(&b, "  Check Only           : %t\n", planned.CheckOnly)
	fmt.Fprintf(&b, "  Force                : %t\n", planned.Force)
	fmt.Fprintf(&b, "  Attestations         : %s\n", planned.Attestations)
	if planned.Attested != "" {
		fmt.Fprintf(&b, "  Attestation Result   : %s\n", planned.Attested)
	}
	fmt.Fprintf(&b, "  Result               : %s\n", result)
	if installerOutput != "" {
		b.WriteString("  Section: Installer\n")
		scanner := bufio.NewScanner(strings.NewReader(installerOutput))
		for scanner.Scan() {
			fmt.Fprintf(&b, "    %s\n", scanner.Text())
		}
	}
	return b.String()
}

func formatVersion(version string) string {
	if version == "" {
		return "<unknown>"
	}
	return ensureTagPrefix(version)
}
