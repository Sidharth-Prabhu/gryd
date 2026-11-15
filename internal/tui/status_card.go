package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/runner"
	"github.com/loom-cli/gryd/internal/state"
)

// RenderStatusCard formats a single project's status in a modern UI card.
func RenderStatusCard(projectName string, cfg *config.Config, st *state.ProjectState, termWidth int) string {
	if termWidth <= 0 {
		termWidth = 80
	}

	actualName := projectName
	if cfg != nil {
		actualName = cfg.Name
	} else if st != nil {
		actualName = st.ProjectName
	}

	// 1. Header & Status Pill
	titlePill := BadgeProject.Render(" " + actualName + " ")

	aliveCount := 0
	if st != nil {
		for _, p := range st.Processes {
			if p.PID > 0 && state.IsProcessAlive(p.PID) {
				aliveCount++
			}
		}
	}

	var statusPill string
	if st == nil || len(st.Processes) == 0 {
		statusPill = BadgeMuted.Render(" NOT RUNNING ")
	} else if aliveCount == len(st.Processes) {
		statusPill = BadgeSuccess.Render(fmt.Sprintf(" ● ALL %d RUNNING ", aliveCount))
	} else if aliveCount > 0 {
		statusPill = BadgeWarning.Render(fmt.Sprintf(" ! %d/%d RUNNING ", aliveCount, len(st.Processes)))
	} else {
		statusPill = BadgeDanger.Render(" ✖ TERMINATED ")
	}

	metaLeft := lipgloss.JoinHorizontal(lipgloss.Center, titlePill)
	topSpacer := termWidth - lipgloss.Width(metaLeft) - lipgloss.Width(statusPill) - 6
	if topSpacer < 1 {
		topSpacer = 1
	}
	topBar := lipgloss.JoinHorizontal(lipgloss.Center, metaLeft, strings.Repeat(" ", topSpacer), statusPill)

	// 2. Metadata Lines
	var metaLines []string
	if cfg != nil && cfg.WorkingDirectory != "" {
		metaLines = append(metaLines, fmt.Sprintf("\033[2mWorking Dir:\033[0m  %s", cfg.WorkingDirectory))
	}
	if cfg != nil && cfg.ConfigPath != "" {
		metaLines = append(metaLines, fmt.Sprintf("\033[2mConfig File:\033[0m  %s", cfg.ConfigPath))
	}
	if st != nil && !st.StartedAt.IsZero() && aliveCount > 0 {
		uptime := time.Since(st.StartedAt).Round(time.Second)
		metaLines = append(metaLines, fmt.Sprintf("\033[2mUptime:\033[0m       %s (started %s)", uptime, st.StartedAt.Format("15:04:05")))
	}
	if st != nil && st.LogFilePath != "" {
		metaLines = append(metaLines, fmt.Sprintf("\033[2mLog File:\033[0m     %s", st.LogFilePath))
	}
	if cfg != nil {
		for svcName, svc := range cfg.Services {
			if svc.Domain != "" {
				metaLines = append(metaLines, fmt.Sprintf("\033[2mDomain (%s):\033[0m \033[1;36mhttp://%s:8080\033[0m \033[2m(or http://%s)\033[0m", svcName, svc.Domain, svc.Domain))
			}
		}
	}

	// 3. Service Table
	colNameW := 18
	colStatusW := 14
	colPidW := 10
	colPortW := 10
	colCmdW := termWidth - colNameW - colStatusW - colPidW - colPortW - 14
	if colCmdW < 18 {
		colCmdW = 18
	}

	tableHeader := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(
		fmt.Sprintf("  %-*s %-*s %-*s %-*s %s", colNameW, "SERVICE", colStatusW, "STATUS", colPidW, "PID", colPortW, "PORT", "COMMAND"),
	)
	var rows []string
	rows = append(rows, tableHeader)
	rows = append(rows, "  "+strings.Repeat("─", termWidth-10))

	// Gather service names
	serviceNames := make(map[string]bool)
	if cfg != nil {
		for n := range cfg.Services {
			serviceNames[n] = true
		}
	}
	if st != nil {
		for n := range st.Processes {
			serviceNames[n] = true
		}
	}

	for name := range serviceNames {
		cmdStr := "-"
		portStr := "-"
		if cfg != nil {
			if svc, ok := cfg.Services[name]; ok {
				cmdStr = svc.Command
				if svc.Port > 0 {
					portStr = fmt.Sprintf(":%d", svc.Port)
				}
			}
		}

		pidStr := "-"
		statusBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render("○ Inactive")

		if st != nil {
			if proc, ok := st.Processes[name]; ok {
				if proc.Command != "" && cmdStr == "-" {
					cmdStr = proc.Command
				}
				if proc.PID > 0 {
					if state.IsProcessAlive(proc.PID) {
						statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render("● Running")
						pidStr = fmt.Sprintf("%d", proc.PID)
					} else {
						statusBadge = lipgloss.NewStyle().Foreground(ColorDanger).Render("✖ Exited")
						pidStr = fmt.Sprintf("%d", proc.PID)
					}
				}
			}
		}

		if len(cmdStr) > colCmdW {
			cmdStr = cmdStr[:colCmdW-3] + "..."
		}

		row := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
			colNameW, name,
			colStatusW+lipgloss.Width(statusBadge)-len(lipgloss.NewStyle().Render(statusBadge)), statusBadge,
			colPidW, pidStr,
			colPortW, portStr,
			cmdStr,
		)
		rows = append(rows, row)
	}

	// List dependencies / sidecars if defined
	if cfg != nil && len(cfg.Dependencies) > 0 {
		rows = append(rows, "", lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render("  SIDECARS / DEPENDENCIES"))
		for depName, dep := range cfg.Dependencies {
			portStr := "-"
			statusBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render("○ Inactive")
			if len(dep.Ports) > 0 {
				hostPort := strings.Split(dep.Ports[0], ":")[0]
				portStr = ":" + hostPort
				if p, err := strconv.Atoi(hostPort); err == nil && runner.IsPortInUse(p) {
					statusBadge = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render("● Running")
				}
			}
			row := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
				colNameW, depName,
				colStatusW+lipgloss.Width(statusBadge)-len(lipgloss.NewStyle().Render(statusBadge)), statusBadge,
				colPidW, "(container)",
				colPortW, portStr,
				dep.Image,
			)
			rows = append(rows, row)
		}
	}

	cardContent := lipgloss.JoinVertical(lipgloss.Left,
		topBar,
		"",
		strings.Join(metaLines, "\n"),
		"",
		strings.Join(rows, "\n"),
	)

	return BoxCard.Width(termWidth - 2).Render(cardContent)
}
