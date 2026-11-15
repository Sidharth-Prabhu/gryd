package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// SetupPortForwarding forwards incoming loopback port 80 to proxyPort
// so browsers typing http://domain.test (without :8080) connect cleanly.
// This is done without running Gryd itself as root.
func SetupPortForwarding(targetPort int) error {
	switch runtime.GOOS {
	case "darwin":
		// On macOS, configure a pf anchor to redirect port 80 -> targetPort.
		// Matching on lo0, en0, and general proto catches both loopback and local outbound requests.
		rule := fmt.Sprintf("rdr pass on lo0 inet proto tcp from any to any port 80 -> 127.0.0.1 port %d\n"+
			"rdr pass on en0 inet proto tcp from any to any port 80 -> 127.0.0.1 port %d\n"+
			"rdr pass proto tcp from any to any port 80 -> 127.0.0.1 port %d\n", targetPort, targetPort, targetPort)
		tmpRuleFile, err := os.CreateTemp("", "gryd-pf-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmpRuleFile.Name())
		_, _ = tmpRuleFile.WriteString(rule)
		_ = tmpRuleFile.Close()

		if os.Geteuid() == 0 {
			_ = exec.Command("pfctl", "-a", "com.apple/gryd", "-f", tmpRuleFile.Name()).Run()
			_ = exec.Command("pfctl", "-E").Run()
		} else {
			script := fmt.Sprintf(`do shell script "pfctl -a 'com.apple/gryd' -f '%s' && pfctl -E 2>/dev/null || true" with administrator privileges with prompt "Gryd needs permission to forward local port 80 for domain routing."`, tmpRuleFile.Name())
			cmd := exec.Command("osascript", "-e", script)
			_ = cmd.Run()
		}
		return nil

	case "linux":
		// On Linux, use iptables PREROUTING redirect
		rule := fmt.Sprintf("iptables -t nat -A OUTPUT -p tcp -d 127.0.0.1 --dport 80 -j REDIRECT --to-ports %d", targetPort)
		cmd := exec.Command("sudo", "sh", "-c", rule)
		_ = cmd.Run()
		return nil

	case "windows":
		// On Windows, use netsh portproxy
		rule := fmt.Sprintf("netsh interface portproxy add v4tov4 listenaddress=127.0.0.1 listenport=80 connectaddress=127.0.0.1 connectport=%d", targetPort)
		cmd := exec.Command("powershell", "-Command", fmt.Sprintf("Start-Process cmd -ArgumentList '/c %s' -Verb RunAs", rule))
		_ = cmd.Run()
		return nil
	}
	return nil
}

// RemovePortForwarding tears down the port forwarding rule.
func RemovePortForwarding() {
	switch runtime.GOOS {
	case "darwin":
		script := `do shell script "pfctl -a 'com.apple/gryd' -F all 2>/dev/null || true" with administrator privileges`
		_ = exec.Command("osascript", "-e", script).Run()
	case "linux":
		_ = exec.Command("sudo", "sh", "-c", "iptables -t nat -D OUTPUT -p tcp -d 127.0.0.1 --dport 80 -j REDIRECT 2>/dev/null || true").Run()
	case "windows":
		_ = exec.Command("powershell", "-Command", "Start-Process cmd -ArgumentList '/c netsh interface portproxy delete v4tov4 listenaddress=127.0.0.1 listenport=80' -Verb RunAs").Run()
	}
}
