package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/doctor"
	"github.com/loom-cli/gryd/internal/state"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var doctorJSONFlag bool

var doctorCmd = &cobra.Command{
	Use:   "doctor [project]",
	Short: "Check system prerequisites, binaries, and tool readiness for your project",
	Long: `Inspect all required tools and runtime dependencies for services defined in gryd.yaml.
Automatically checks system PATH and common developer directories.
If any tool is not found, you can specify its custom path in gryd.yaml under 'tools:':

  tools:
    pnpm: /Users/username/Library/pnpm/pnpm
    node: /opt/homebrew/bin/node
    python: /usr/local/bin/python3`,
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

		report := doctor.CheckProject(cfg)

		if doctorJSONFlag {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			if !report.AllPassed {
				os.Exit(1)
			}
			return nil
		}

		termW := 80
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
			termW = w
		}

		fmt.Println()
		fmt.Println(doctor.RenderCard(report, termW))
		fmt.Println()

		if !report.AllPassed {
			return fmt.Errorf("tool check failed: %d missing tool(s)", len(report.MissingTools()))
		}

		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSONFlag, "json", false, "Output doctor diagnostics as formatted JSON")
	rootCmd.AddCommand(doctorCmd)
}
