package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// writeWithElevatedPrivileges attempts cross-platform administrator elevation to write target file.
// On macOS: uses osascript with administrator privileges prompt.
// On Linux: tries pkexec or sudo tee.
// On Windows: invokes PowerShell Start-Process -Verb RunAs.
func writeWithElevatedPrivileges(filePath string, content string) error {
	// First create a temporary file with the content to safely move or tee
	tmpFile, err := os.CreateTemp("", "gryd-hosts-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return err
	}
	_ = tmpFile.Close()

	switch runtime.GOOS {
	case "darwin":
		// macOS AppleScript system dialog: "Gryd wants to update /etc/hosts for local domain routing"
		script := fmt.Sprintf(`do shell script "cp '%s' '%s' && chmod 644 '%s'" with administrator privileges with prompt "Gryd needs administrator permission to update local development domains in /etc/hosts."`, tmpPath, filePath, filePath)
		cmd := exec.Command("osascript", "-e", script)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}

		// Fallback to interactive sudo in terminal if osascript was dismissed or not available
		sudoCmd := exec.Command("sudo", "cp", tmpPath, filePath)
		sudoCmd.Stdin = os.Stdin
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		return sudoCmd.Run()

	case "linux":
		// Check for pkexec (GUI/polkit) or sudo
		if _, err := exec.LookPath("pkexec"); err == nil {
			cmd := exec.Command("pkexec", "cp", tmpPath, filePath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				return nil
			}
		}

		// Interactive sudo fallback
		sudoCmd := exec.Command("sudo", "cp", tmpPath, filePath)
		sudoCmd.Stdin = os.Stdin
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		return sudoCmd.Run()

	case "windows":
		// Windows PowerShell Start-Process with Verb RunAs (UAC prompt)
		psScript := fmt.Sprintf(`Start-Process -FilePath "cmd.exe" -ArgumentList "/c copy /y ""%s"" ""%s""" -Verb RunAs -Wait`,
			strings.ReplaceAll(tmpPath, `'`, `''`),
			strings.ReplaceAll(filePath, `'`, `''`),
		)
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()

	default:
		sudoCmd := exec.Command("sudo", "cp", tmpPath, filePath)
		sudoCmd.Stdin = os.Stdin
		sudoCmd.Stdout = os.Stdout
		sudoCmd.Stderr = os.Stderr
		return sudoCmd.Run()
	}
}
