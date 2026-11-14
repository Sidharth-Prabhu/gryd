package runner

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// PortCollisionAction represents user or auto decision for port collisions.
type PortCollisionAction int

const (
	ActionCancel PortCollisionAction = iota
	ActionKill
	ActionAssign
)

// PortConflict represents a detected collision on a service port.
type PortConflict struct {
	ServiceName string
	Port        int
	PIDs        []int
	ProcessCmds []string
}

// FindAvailablePort finds the next available TCP port starting from startPort.
func FindAvailablePort(startPort int) int {
	if startPort <= 0 {
		startPort = 3000
	}
	for p := startPort + 1; p < startPort+500; p++ {
		if !IsPortInUse(p) {
			// Double check by attempting to listen
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if err == nil {
				_ = ln.Close()
				return p
			}
		}
	}
	return startPort + 1
}

// CheckPortConflicts inspects all services in a config to detect ports in use by external processes.
// activePIDs is an optional set of PIDs known to be owned by Gryd (to not falsely treat them as external).
func CheckPortConflicts(services map[string]int, activePIDs map[int]bool) []PortConflict {
	myPid := os.Getpid()
	var conflicts []PortConflict
	for svcName, port := range services {
		if port <= 0 {
			continue
		}
		if IsPortInUse(port) {
			pids := FindPIDsListeningOnPort(port)
			var externalPIDs []int
			var cmds []string

			for _, pid := range pids {
				if pid == myPid {
					continue
				}
				if activePIDs != nil && activePIDs[pid] {
					continue
				}
				externalPIDs = append(externalPIDs, pid)
				cmds = append(cmds, getProcessCommand(pid))
			}

			if len(externalPIDs) > 0 {
				conflicts = append(conflicts, PortConflict{
					ServiceName: svcName,
					Port:        port,
					PIDs:        externalPIDs,
					ProcessCmds: cmds,
				})
			}
		}
	}
	return conflicts
}

// FormatPIDs returns a comma-separated string of PIDs.
func (c *PortConflict) FormatPIDs() string {
	var s []string
	for _, p := range c.PIDs {
		s = append(s, strconv.Itoa(p))
	}
	return fmt.Sprintf("%v", s)
}
