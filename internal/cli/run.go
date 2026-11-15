package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/doctor"
	"github.com/loom-cli/gryd/internal/logger"
	"github.com/loom-cli/gryd/internal/proxy"
	"github.com/loom-cli/gryd/internal/runner"
	"github.com/loom-cli/gryd/internal/state"
	"github.com/loom-cli/gryd/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var runCmd = &cobra.Command{
	Use:   "run [project]",
	Short: "Start all services for a project environment",
	Long: `Start all services defined in a gryd.yaml configuration concurrently.
Launches the interactive dashboard UI by default with per-service logs,
hot reload ('r'), background detach ('c'), and clean shutdown ('q').

If [project] is provided, Gryd searches for the project configuration in standard
directories or uses the local directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		}

		configPath, err := config.FindConfigFile(configFile, projectName)
		if err != nil {
			return err
		}

		cfg, err := config.Load(configPath)
		if err != nil {
			return fmt.Errorf("configuration error: %w", err)
		}

		if err := cfg.ApplyProfile(profileFlag); err != nil {
			return err
		}

		// Pre-flight check: verify required system tools and runtime dependencies
		report := doctor.CheckProject(cfg)
		if !report.AllPassed {
			missing := report.MissingTools()
			fmt.Println()
			fmt.Println(doctor.RenderMissingToolsAlert(missing))
			fmt.Println()
			if term.IsTerminal(int(os.Stdout.Fd())) {
				fmt.Print("? Would you like to proceed anyway? (y/N) [N]: ")
				reader := bufio.NewReader(os.Stdin)
				ans, _ := reader.ReadString('\n')
				ans = strings.ToLower(strings.TrimSpace(ans))
				if ans != "y" && ans != "yes" {
					return fmt.Errorf("pre-flight tool check failed: missing %d required tool(s)", len(missing))
				}
			} else {
				return fmt.Errorf("pre-flight tool check failed: missing %d required tool(s)", len(missing))
			}
		}

		// Check for and resolve any port collisions before starting services
		if err := runner.InteractiveResolvePortConflicts(cfg, os.Stdin, os.Stdout); err != nil {
			return err
		}

		// If running in a non-interactive terminal or raw redirect, fallback to plain streaming runner
		if !term.IsTerminal(int(os.Stdout.Fd())) {
			zapLog, err := logger.NewZapLogger(debug)
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			defer func() {
				_ = zapLog.Sync()
			}()

			appRunner, err := runner.New(cfg, zapLog)
			if err != nil {
				return fmt.Errorf("failed to initialize runner: %w", err)
			}
			return appRunner.Run(context.Background())
		}

		// Check if project is currently running; start services if not
		projectState, err := state.LoadState(cfg.Name)
		if err != nil || len(projectState.Processes) == 0 {
			newSt, err := runner.ReloadBackgroundProject(cfg.Name)
			if err != nil {
				return fmt.Errorf("failed to start project services: %w", err)
			}
			projectState = newSt
		}

		// Initialize interactive TUI dashboard
		model, err := tui.NewUIModel(cfg, projectState)
		if err != nil {
			return err
		}

		// Start local domain reverse proxy during TUI session only if explicit domains are configured
		if proxy.HasConfiguredDomains(cfg) {
			proxyServer := proxy.NewReverseProxyServer(cfg, nil)
			if err := proxyServer.Start(); err == nil {
				defer proxyServer.Close()
				_ = proxy.SyncHostsFile(proxyServer.GetDomains())
				if proxyServer.Port() != 80 {
					_ = proxy.SetupPortForwarding(proxyServer.Port())
				}
			}
		}

		p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("error running UI: %w", err)
		}

		// If user pressed 'q' / Ctrl+C to stop (and didn't detach with 'c')
		if m, ok := finalModel.(*tui.UIModel); ok && !m.IsDetached() {
			fmt.Printf("\n\033[1;33m[gryd] Stopping project '%s'...\033[0m\n", cfg.Name)
			runner.StopProjectProcesses(projectState)
			_ = state.RemoveState(cfg.Name)
			fmt.Printf("\033[1;32m✓ All services stopped cleanly.\033[0m\n\n")
		}

		return nil
	},
}
