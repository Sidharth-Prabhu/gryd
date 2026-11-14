package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/tui"
	"gopkg.in/yaml.v3"
)

// readLineSafely reads a line from the reader, returning empty string and EOF error on stream exhaustion.
func readLineSafely(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && len(line) > 0 {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// runInitWizard walks the user through an interactive step-by-step setup to generate gryd.yaml.
func runInitWizard(defaultName string) error {
	reader := bufio.NewReader(os.Stdin)

	currDir, _ := os.Getwd()
	defaultProjectName := filepath.Base(currDir)
	if defaultName != "" && defaultName != "MyProject" {
		defaultProjectName = defaultName
	}

	termW := 80
	titlePill := tui.BadgeProject.Render(" GRYD INIT WIZARD ")
	desc := lipgloss.NewStyle().Foreground(tui.ColorTextMuted).Render(" Interactive Multi-Service Project Setup")
	topHeader := lipgloss.JoinHorizontal(lipgloss.Center, titlePill, desc)

	box := tui.BoxCard.Width(termW - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			topHeader,
			"",
			"Welcome to the Gryd configuration wizard!\nAnswer the following prompts to scaffold a fully configured \033[1mgryd.yaml\033[0m.",
		),
	)

	fmt.Println()
	fmt.Println(box)
	fmt.Println()

	// 1. Project Name
	fmt.Printf("\033[1;36m? Project name\033[0m \033[2m[%s]\033[0m: ", defaultProjectName)
	projectName, err := readLineSafely(reader)
	if err != nil && projectName == "" {
		return err
	}
	if projectName == "" {
		projectName = defaultProjectName
	}

	// 2. Working Directory
	fmt.Printf("\033[1;36m? Project working directory\033[0m \033[2m[.]\033[0m: ")
	workingDir, _ := readLineSafely(reader)
	if workingDir == "" {
		workingDir = "."
	}

	// 3. Global Settings
	fmt.Println()
	fmt.Println("\033[1mProject Settings:\033[0m")
	fmt.Printf("  \033[1;36m? Enable health checks on port listeners?\033[0m \033[2m(Y/n) [Y]\033[0m: ")
	healthCheckInput, _ := readLineSafely(reader)
	healthCheck := true
	if strings.ToLower(healthCheckInput) == "n" || strings.ToLower(healthCheckInput) == "no" {
		healthCheck = false
	}

	fmt.Printf("  \033[1;36m? Restart services automatically on unexpected failure?\033[0m \033[2m(y/N) [N]\033[0m: ")
	restartInput, _ := readLineSafely(reader)
	restartOnFail := false
	if strings.ToLower(restartInput) == "y" || strings.ToLower(restartInput) == "yes" {
		restartOnFail = true
	}

	// 4. Services Loop
	services := make(map[string]config.Service)
	var configuredServiceNames []string

	fmt.Println()
	bannerServices := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorPrimary).Render("── Configuring Services ──")
	fmt.Println(bannerServices)
	fmt.Println("\033[2mAdd the microservices, frontends, APIs, workers, or mock servers for this project.\033[0m")
	fmt.Println()

	serviceIndex := 1
	for {
		header := fmt.Sprintf("\033[1;33m[Service #%d]\033[0m", serviceIndex)
		fmt.Println(header)

		// Service Name
		var svcName string
		for {
			fmt.Printf("  \033[1;36m? Service identifier name\033[0m (e.g. backend, frontend, worker): ")
			nameIn, err := readLineSafely(reader)
			if err != nil {
				return err
			}
			svcName = nameIn
			if svcName == "" {
				fmt.Println("    \033[31mService name cannot be empty.\033[0m")
				continue
			}
			if _, exists := services[svcName]; exists {
				fmt.Printf("    \033[31mService '%s' is already configured. Choose another name.\033[0m\n", svcName)
				continue
			}
			break
		}

		// Service Directory
		defaultSvcDir := "."
		fmt.Printf("  \033[1;36m? Service subdirectory\033[0m \033[2m(relative to working dir, default '.')\033[0m: ")
		dirIn, _ := readLineSafely(reader)
		svcDir := dirIn
		if svcDir == "" {
			svcDir = defaultSvcDir
		}

		// Command
		var svcCmd string
		for {
			fmt.Printf("  \033[1;36m? Command to execute\033[0m (e.g. 'npm run dev', 'uv run api', 'go run .'): ")
			cmdIn, err := readLineSafely(reader)
			if err != nil {
				return err
			}
			svcCmd = cmdIn
			if svcCmd == "" {
				fmt.Println("    \033[31mCommand cannot be empty.\033[0m")
				continue
			}
			break
		}

		// Port
		var port int
		fmt.Printf("  \033[1;36m? Port\033[0m \033[2m(optional, leave empty if background worker/queue)\033[0m: ")
		portStr, _ := readLineSafely(reader)
		if portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				port = p
			} else {
				fmt.Printf("    \033[33mInvalid port '%s', proceeding with port: 0\033[0m\n", portStr)
			}
		}

		// Dependencies
		var dependsOn config.DependsOnMap
		if len(configuredServiceNames) > 0 {
			fmt.Printf("  \033[1;36m? Depends on other services?\033[0m \033[2mAvailable: [%s] (comma-separated, or Enter to skip)\033[0m: ",
				strings.Join(configuredServiceNames, ", "))
			depStr, _ := readLineSafely(reader)
			if depStr != "" {
				dependsOn = make(config.DependsOnMap)
				for _, part := range strings.Split(depStr, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						dependsOn[part] = config.DepCondition{}
					}
				}
			}
		}

		// Environment Variables
		var envVars map[string]string
		fmt.Printf("  \033[1;36m? Custom environment variables?\033[0m \033[2m(e.g. PORT=8080,ENV=dev - comma-separated, or Enter to skip)\033[0m: ")
		envStr, _ := readLineSafely(reader)
		if envStr != "" {
			envVars = make(map[string]string)
			for _, pair := range strings.Split(envStr, ",") {
				pair = strings.TrimSpace(pair)
				if kv := strings.SplitN(pair, "=", 2); len(kv) == 2 {
					k := strings.TrimSpace(kv[0])
					v := strings.TrimSpace(kv[1])
					if k != "" {
						envVars[k] = v
					}
				}
			}
		}

		// Store service
		services[svcName] = config.Service{
			Directory: svcDir,
			Command:   svcCmd,
			Port:      port,
			DependsOn: dependsOn,
			Env:       envVars,
		}
		configuredServiceNames = append(configuredServiceNames, svcName)
		fmt.Printf("  \033[1;32m✓ Service '%s' added!\033[0m\n\n", svcName)

		// Ask whether to add another service
		fmt.Printf("\033[1;36m? Would you like to configure another service?\033[0m \033[2m(Y/n) [Y]\033[0m: ")
		anotherIn, err := readLineSafely(reader)
		if err != nil {
			break
		}
		trimmed := strings.ToLower(anotherIn)
		if trimmed == "n" || trimmed == "no" {
			break
		}
		serviceIndex++
		fmt.Println()
	}

	// 5. Construct Config object
	cfg := &config.Config{
		Name:             projectName,
		WorkingDirectory: workingDir,
		Services:         services,
		Settings: config.Settings{
			HealthCheck:      healthCheck,
			RestartOnFailure: restartOnFail,
		},
	}

	// Check if gryd.yaml already exists
	targetFile := "gryd.yaml"
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Printf("\n\033[1;33mWarning: '%s' already exists in this directory.\033[0m\n", targetFile)
		fmt.Print("  \033[1;36m? Overwrite existing file?\033[0m \033[2m(y/N) [N]\033[0m: ")
		owIn, _ := readLineSafely(reader)
		if strings.ToLower(owIn) != "y" && strings.ToLower(owIn) != "yes" {
			fmt.Println("\nConfiguration wizard cancelled. No changes written.")
			return nil
		}
	}

	// Marshal to formatted YAML
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	if err := os.WriteFile(targetFile, yamlBytes, 0644); err != nil {
		return fmt.Errorf("failed to write '%s': %w", targetFile, err)
	}

	// 6. Success Output
	fmt.Println()
	successPill := tui.BadgeSuccess.Render(" ✓ GRYD CONFIGURATION GENERATED ")
	fmt.Println(successPill)
	fmt.Println()

	var previewLines []string
	previewLines = append(previewLines, fmt.Sprintf("\033[1mProject:\033[0m        %s", cfg.Name))
	previewLines = append(previewLines, fmt.Sprintf("\033[1mConfig File:\033[0m    %s", targetFile))
	previewLines = append(previewLines, fmt.Sprintf("\033[1mServices:\033[0m       %d defined", len(cfg.Services)))
	for sName, s := range cfg.Services {
		portInfo := ""
		if s.Port > 0 {
			portInfo = fmt.Sprintf(" (port %d)", s.Port)
		}
		previewLines = append(previewLines, fmt.Sprintf("  • \033[1;36m%s\033[0m%s: %s", sName, portInfo, s.Command))
	}

	previewCard := tui.BoxCard.Width(termW - 2).Render(strings.Join(previewLines, "\n"))
	fmt.Println(previewCard)
	fmt.Println()
	fmt.Println("🚀 Next steps:")
	fmt.Println("  • Run \033[1mgryd test\033[0m to verify all services build and listen successfully")
	fmt.Println("  • Run \033[1mgryd run\033[0m to launch the interactive TUI development dashboard")
	fmt.Println()

	return nil
}
