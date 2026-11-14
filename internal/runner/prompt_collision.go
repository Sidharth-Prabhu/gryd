package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/loom-cli/gryd/internal/config"
	"golang.org/x/term"
)

// InteractiveResolvePortConflicts inspects configured services for port collisions.
// If collisions are detected:
//   - If running interactively, prompts the user:
//     "Port <port> is occupied by PID <pids> (<cmd>). [K]ill process, [A]ssign port <newPort>, or [C]ancel?"
//   - If non-interactive, terminates the lingering process (standard automated dev server behavior).
//
// Returns a map of service name to assigned port, and an error if cancelled.
func InteractiveResolvePortConflicts(cfg *config.Config, in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	servicesToCheck := make(map[string]int)
	for name, svc := range cfg.Services {
		if svc.Port > 0 {
			servicesToCheck[name] = svc.Port
		}
	}

	conflicts := CheckPortConflicts(servicesToCheck, nil)
	if len(conflicts) == 0 {
		return nil
	}

	isInteractive := true
	if f, ok := in.(*os.File); ok {
		isInteractive = term.IsTerminal(int(f.Fd()))
	}

	reader := bufio.NewReader(in)

	for _, c := range conflicts {
		newPort := FindAvailablePort(c.Port)
		cmdSummary := strings.Join(c.ProcessCmds, ", ")
		if cmdSummary == "" {
			cmdSummary = "unknown"
		}

		pidSummary := ""
		for i, pid := range c.PIDs {
			if i > 0 {
				pidSummary += ", "
			}
			pidSummary += fmt.Sprintf("%d", pid)
		}

		if !isInteractive {
			// In non-interactive environments, default to freeing the port
			_, _ = FreePort(c.Port)
			continue
		}

		for {
			fmt.Fprintf(out, "\n\033[1;33m[gryd] Port collision detected for service '%s'!\033[0m\n", c.ServiceName)
			fmt.Fprintf(out, "Port \033[1;31m%d\033[0m is occupied by PID \033[1m%s\033[0m (\033[2m%s\033[0m).\n", c.Port, pidSummary, cmdSummary)
			fmt.Fprintf(out, "\033[1;36m[K]ill process, [A]ssign port %d, or [C]ancel?\033[0m [K/a/c]: ", newPort)

			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("port resolution cancelled: %w", err)
			}
			choice := strings.ToLower(strings.TrimSpace(input))

			if choice == "" || choice == "k" || choice == "kill" {
				fmt.Fprintf(out, "\033[1;32mTerminating process(es) holding port %d...\033[0m\n", c.Port)
				_, _ = FreePort(c.Port)
				break
			} else if choice == "a" || choice == "assign" {
				fmt.Fprintf(out, "\033[1;32mReassigned service '%s' to port %d\033[0m\n", c.ServiceName, newPort)
				svc := cfg.Services[c.ServiceName]
				svc.Port = newPort
				if svc.Env == nil {
					svc.Env = make(map[string]string)
				}
				svc.Env["PORT"] = fmt.Sprintf("%d", newPort)
				cfg.Services[c.ServiceName] = svc
				break
			} else if choice == "c" || choice == "cancel" {
				return fmt.Errorf("execution cancelled due to port collision on port %d", c.Port)
			} else {
				fmt.Fprintln(out, "Please choose 'k' (kill), 'a' (assign new port), or 'c' (cancel).")
			}
		}
	}

	return nil
}
