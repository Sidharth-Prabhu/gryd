package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/doctor"
	"github.com/loom-cli/gryd/internal/runner"
	"github.com/loom-cli/gryd/internal/state"
	"github.com/loom-cli/gryd/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	testTimeoutFlag time.Duration
	testJSONFlag    bool
)

var testCmd = &cobra.Command{
	Use:   "test [project]",
	Short: "Run a test boot of the project, verify all services start, and cleanly terminate",
	Long: `Starts all services in the project, monitors them until all configured ports
are active and listening (or processes run stably), and then gracefully terminates
all services and frees ports.

Displays a rich full-screen TUI with real-time log scrolling and service readiness badges.
Returns exit code 0 if all services pass, or 1 if any service fails or times out.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectName string
		if len(args) > 0 {
			projectName = args[0]
		} else {
			detected, err := config.DetectCurrentProject()
			if err == nil {
				projectName = detected
			} else {
				running, _ := state.ListRunningProjects()
				if len(running) == 1 {
					projectName = running[0].ProjectName
				} else if len(running) > 1 {
					return fmt.Errorf("multiple projects found. Please specify which one: gryd test <project>")
				} else {
					return fmt.Errorf("no project specified and no active project in current directory")
				}
			}
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

		// Pre-flight check: verify required system tools before launching test run
		report := doctor.CheckProject(cfg)
		if !report.AllPassed {
			missing := report.MissingTools()
			if testJSONFlag {
				type testJSONOutput struct {
					Project string                     `json:"project"`
					Passed  bool                       `json:"passed"`
					Error   string                     `json:"error,omitempty"`
					Results []runner.ServiceTestResult `json:"results"`
				}
				out := testJSONOutput{
					Project: cfg.Name,
					Passed:  false,
					Error:   fmt.Sprintf("tool check failed: missing %d required tool(s)", len(missing)),
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Println()
			fmt.Println(doctor.RenderMissingToolsAlert(missing))
			fmt.Println()
			return fmt.Errorf("pre-flight tool check failed: missing %d required tool(s)", len(missing))
		}

		// Non-interactive or JSON output mode (e.g. CI / scripts)
		if testJSONFlag || !term.IsTerminal(int(os.Stdout.Fd())) {
			results, err := runner.TestRun(context.Background(), cfg, testTimeoutFlag)
			if testJSONFlag {
				type testJSONOutput struct {
					Project string                     `json:"project"`
					Passed  bool                       `json:"passed"`
					Error   string                     `json:"error,omitempty"`
					Results []runner.ServiceTestResult `json:"results"`
				}
				out := testJSONOutput{
					Project: cfg.Name,
					Passed:  err == nil,
					Results: results,
				}
				if err != nil {
					out.Error = err.Error()
				}
				data, _ := json.MarshalIndent(out, "", "  ")
				fmt.Println(string(data))
				if err != nil {
					return err
				}
				return nil
			}

			if err != nil {
				return err
			}
			printTestResultsSummary(cfg.Name, results)
			return nil
		}

		// Interactive Bubble Tea TUI
		testModel := tui.NewTestUIModel(cfg, testTimeoutFlag)
		p := tea.NewProgram(testModel, tea.WithAltScreen())

		var testResults []runner.ServiceTestResult
		var testErr error

		go func() {
			hooks := &runner.TestHooks{
				OnLog: func(line string) {
					testModel.AddLog(line)
				},
				OnServiceReady: func(name string, d time.Duration) {
					testModel.SetServiceReady(name, d)
				},
			}
			res, err := runner.TestRunWithHooks(context.Background(), cfg, testTimeoutFlag, hooks)
			testResults = res
			testErr = err
			if err != nil {
				testModel.SetFailed(err)
			} else {
				testModel.SetAllDone()
			}
		}()

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error running test UI: %w", err)
		}

		if testErr != nil {
			fmt.Printf("\n\033[1;31m✗ Test run failed for '%s':\033[0m %v\n\n", cfg.Name, testErr)
			return testErr
		}

		printTestResultsSummary(cfg.Name, testResults)
		return nil
	},
}

func printTestResultsSummary(projectName string, results []runner.ServiceTestResult) {
	fmt.Println()
	fmt.Printf("\033[1;32m✓ All %d services for '%s' booted successfully!\033[0m\n", len(results), projectName)
	fmt.Println("All child processes have been cleanly terminated and ports released.")
	fmt.Println()

	fmt.Printf("  \033[1m%-16s %-10s %-8s %-12s %s\033[0m\n", "SERVICE", "STATUS", "PORT", "BOOT TIME", "COMMAND")
	fmt.Printf("  %s\n", strings.Repeat("─", 68))

	for _, r := range results {
		portStr := "-"
		if r.Port > 0 {
			portStr = fmt.Sprintf("%d", r.Port)
		}
		cmdStr := r.Command
		if len(cmdStr) > 28 {
			cmdStr = cmdStr[:25] + "..."
		}
		bootStr := r.Duration.Round(time.Millisecond).String()
		fmt.Printf("  %-16s \033[32mPassed\033[0m     %-8s %-12s %s\n",
			r.Name, portStr, bootStr, cmdStr)
	}

	fmt.Println()
	fmt.Printf("\033[1;32m✓ Project '%s' is verified and ready to run with: gryd run\033[0m\n\n", projectName)
}

func init() {
	testCmd.Flags().DurationVarP(&testTimeoutFlag, "timeout", "t", 25*time.Second, "Maximum time to wait for services to boot and listen")
	testCmd.Flags().BoolVarP(&testJSONFlag, "json", "", false, "Output verification results in machine-readable JSON format")
}
