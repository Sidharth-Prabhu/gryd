package proxy

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const hostsMarkerStart = "# GRYD LOCAL DOMAINS (DO NOT EDIT)"
const hostsMarkerEnd = "# END GRYD LOCAL DOMAINS"

func getHostsPath() string {
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		return fmt.Sprintf(`%s\System32\drivers\etc\hosts`, systemRoot)
	}
	return "/etc/hosts"
}

// SyncHostsFile adds or updates the specified domains in the system hosts file pointing to 127.0.0.1.
// If permission is denied, it returns a non-fatal error with guidance.
func SyncHostsFile(domains []string) error {
	if len(domains) == 0 {
		return nil
	}

	hostsPath := getHostsPath()
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", hostsPath, err)
	}

	content := string(data)

	// Clean out existing gryd managed block if present
	lines := strings.Split(content, "\n")
	var newLines []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == hostsMarkerStart {
			inBlock = true
			continue
		}
		if trimmed == hostsMarkerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	// Filter domains: only non-.localhost domains need /etc/hosts entries (since *.localhost resolves natively)
	var needEntry []string
	for _, d := range domains {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			continue
		}
		if strings.HasSuffix(d, ".localhost") {
			// RFC 6761: natively resolved to loopback by OS/browser
			continue
		}
		needEntry = append(needEntry, d)
	}

	if len(needEntry) == 0 {
		// No extra entries required, just clean up old blocks if any
		cleanContent := strings.TrimRight(strings.Join(newLines, "\n"), "\n") + "\n"
		if cleanContent != content {
			_ = os.WriteFile(hostsPath, []byte(cleanContent), 0644)
		}
		return nil
	}

	// Append gryd block
	cleanContent := strings.TrimRight(strings.Join(newLines, "\n"), "\n")
	grydBlock := fmt.Sprintf("\n\n%s\n127.0.0.1 %s\n::1 %s\n%s\n",
		hostsMarkerStart,
		strings.Join(needEntry, " "),
		strings.Join(needEntry, " "),
		hostsMarkerEnd,
	)

	newContent := cleanContent + grydBlock
	err = os.WriteFile(hostsPath, []byte(newContent), 0644)
	if err != nil {
		// Attempt elevated writing (macOS Touch ID/dialog, Linux pkexec/sudo, Windows UAC)
		elevErr := writeWithElevatedPrivileges(hostsPath, newContent)
		if elevErr != nil {
			return fmt.Errorf("permission denied writing to %s. Run 'sudo gryd hosts sync' or execute as Administrator", hostsPath)
		}
	}

	return nil
}

// RemoveHostsEntries removes the gryd managed block from the system hosts file.
func RemoveHostsEntries() error {
	hostsPath := getHostsPath()
	data, err := os.ReadFile(hostsPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var newLines []string
	inBlock := false
	changed := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == hostsMarkerStart {
			inBlock = true
			changed = true
			continue
		}
		if trimmed == hostsMarkerEnd {
			inBlock = false
			changed = true
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	if changed {
		cleanContent := strings.TrimRight(strings.Join(newLines, "\n"), "\n") + "\n"
		err := os.WriteFile(hostsPath, []byte(cleanContent), 0644)
		if err != nil {
			return writeWithElevatedPrivileges(hostsPath, cleanContent)
		}
		return nil
	}

	return nil
}

// CheckHostsResolved verifies whether each domain resolves to loopback in /etc/hosts or DNS.
func CheckHostsResolved(domain string) bool {
	if strings.HasSuffix(domain, ".localhost") {
		return true
	}

	hostsPath := getHostsPath()
	f, err := os.Open(hostsPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ip := fields[0]
			if ip == "127.0.0.1" || ip == "::1" {
				for _, host := range fields[1:] {
					if strings.EqualFold(host, domain) {
						return true
					}
				}
			}
		}
	}
	return false
}
