package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/proxy"
	"github.com/loom-cli/gryd/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	domainRoutingFlag       bool
	removeDomainRoutingFlag bool
	configJSONFlag          bool
	configEditFlag          bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect, edit, or configure gryd.yaml settings",
	Long: `Display or edit the project's gryd.yaml configuration.
  gryd config               Display the current gryd.yaml content
  gryd config --json        Display the configuration formatted as JSON
  gryd config --edit        Open gryd.yaml in a verified terminal text editor (nano or vim)
  gryd config --domain-routing        Configure local subdomains interactively
  gryd config --remove-domain-routing Remove domain routing from all services`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if removeDomainRoutingFlag {
			return runRemoveDomainRouting()
		}
		if domainRoutingFlag {
			return runDomainRoutingConfig()
		}
		if configEditFlag {
			return runEditConfig()
		}
		if configJSONFlag {
			return runShowJSONConfig()
		}
		// Default: show the raw gryd.yaml file
		return runShowConfig()
	},
}

// runShowConfig prints the raw contents of the current gryd.yaml file.
func runShowConfig() error {
	configPath, err := config.FindConfigFile(configFile, "")
	if err != nil {
		return fmt.Errorf("no gryd.yaml found: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file '%s': %w", configPath, err)
	}

	fmt.Printf("\033[2m# %s\033[0m\n", configPath)
	fmt.Print(string(data))
	return nil
}

// runShowJSONConfig parses gryd.yaml and outputs clean, indented JSON.
func runShowJSONConfig() error {
	configPath, err := config.FindConfigFile(configFile, "")
	if err != nil {
		return fmt.Errorf("no gryd.yaml found: %w", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Clean map keys for json marshal
	cleaned := cleanYAMLNode(raw)

	jsonBytes, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	fmt.Println(string(jsonBytes))
	return nil
}

// cleanYAMLNode recursively converts map[interface{}]interface{} to map[string]interface{}
func cleanYAMLNode(in interface{}) interface{} {
	switch x := in.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for k, v := range x {
			m[fmt.Sprintf("%v", k)] = cleanYAMLNode(v)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{})
		for k, v := range x {
			m[k] = cleanYAMLNode(v)
		}
		return m
	case []interface{}:
		for i, v := range x {
			x[i] = cleanYAMLNode(v)
		}
		return x
	default:
		return in
	}
}

// EditorOption describes an editor detected on the system.
type EditorOption struct {
	Name string
	Path string
}

// runEditConfig detects nano and vim installations, asks the user to pick one (or uses default preference),
// optionally saves the choice as default, and launches the editor.
func runEditConfig() error {
	configPath, err := config.FindConfigFile(configFile, "")
	if err != nil {
		return fmt.Errorf("no gryd.yaml found: %w", err)
	}

	// 1. Check if user already set a default editor preference
	savedDefault := state.GetDefaultEditor()
	if savedDefault != "" {
		if path, err := exec.LookPath(savedDefault); err == nil {
			return openEditor(path, configPath)
		}
		// If saved default is no longer valid, clear it
		_ = state.SetDefaultEditor("")
	}

	// 2. Detect installation of nano and vim
	candidates := []string{"nano", "vim"}
	var available []EditorOption

	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			available = append(available, EditorOption{
				Name: name,
				Path: path,
			})
		}
	}

	if len(available) == 0 {
		return fmt.Errorf("neither 'nano' nor 'vim' was found in your PATH. Please install either nano or vim to edit configuration")
	}

	reader := bufio.NewReader(os.Stdin)
	var chosen EditorOption

	// 3. If only one is installed, select it directly
	if len(available) == 1 {
		chosen = available[0]
		fmt.Printf("\033[1;36m[gryd]\033[0m Detected editor: \033[1m%s\033[0m (%s)\n", chosen.Name, chosen.Path)
	} else {
		// Multiple available: prompt user to pick
		fmt.Println("\n\033[1;36m⚡ Select Text Editor to Open gryd.yaml\033[0m")
		for i, opt := range available {
			fmt.Printf("  [%d] \033[1m%s\033[0m  \033[2m(%s)\033[0m\n", i+1, opt.Name, opt.Path)
		}
		fmt.Printf("\n\033[1m? Choose editor [1-%d]\033[0m \033[2m[1]\033[0m: ", len(available))

		input, _ := readLineSafely(reader)
		choiceIdx := 0
		if input != "" {
			if num, err := strconv.Atoi(input); err == nil && num >= 1 && num <= len(available) {
				choiceIdx = num - 1
			} else {
				// Also allow typing the name: "nano" or "vim"
				lower := strings.ToLower(input)
				for i, opt := range available {
					if opt.Name == lower {
						choiceIdx = i
						break
					}
				}
			}
		}
		chosen = available[choiceIdx]
	}

	// 4. Ask user if they want to save this choice as the default editor
	fmt.Printf("\033[1m? Set '%s' as your default editor for Gryd?\033[0m \033[2m(y/N) [N]\033[0m: ", chosen.Name)
	setDefaultChoice, _ := readLineSafely(reader)
	trimmedChoice := strings.ToLower(strings.TrimSpace(setDefaultChoice))
	if trimmedChoice == "y" || trimmedChoice == "yes" {
		if err := state.SetDefaultEditor(chosen.Name); err == nil {
			fmt.Printf("\033[1;32m✓ Saved '%s' as default editor in ~/.gryd/preferences.json\033[0m\n", chosen.Name)
		}
	}

	// 5. Launch editor
	return openEditor(chosen.Path, configPath)
}

func openEditor(editorPath, filePath string) error {
	cmd := exec.Command(editorPath, filePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runRemoveDomainRouting() error {
	configPath, err := config.FindConfigFile(configFile, "")
	if err != nil {
		return fmt.Errorf("no gryd.yaml found: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	removedCount := 0
	for svcName, svc := range cfg.Services {
		if svc.Domain != "" {
			svc.Domain = ""
			cfg.Services[svcName] = svc
			removedCount++
		}
	}

	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	if err := os.WriteFile(configPath, yamlBytes, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	fmt.Printf("\n\033[1;32m✓ Removed domain routing from %d service(s) in %s\033[0m\n", removedCount, configPath)

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\033[1m? Clean up Gryd entries from system hosts file as well?\033[0m \033[2m(Y/n) [Y]\033[0m: ")
	cleanChoice, _ := readLineSafely(reader)
	cleanChoice = strings.ToLower(strings.TrimSpace(cleanChoice))
	if cleanChoice == "" || cleanChoice == "y" || cleanChoice == "yes" {
		if err := proxy.RemoveHostsEntries(); err != nil {
			fmt.Printf("  \033[1;31m✗\033[0m Could not update hosts file: %v\n", err)
			fmt.Println("  Please run: \033[1msudo gryd hosts clean\033[0m")
		} else {
			proxy.RemovePortForwarding()
			fmt.Println("  \033[1;32m✓\033[0m Cleaned up system hosts file successfully!")
		}
	}

	fmt.Println()
	return nil
}

func runDomainRoutingConfig() error {
	configPath, err := config.FindConfigFile(configFile, "")
	if err != nil {
		return fmt.Errorf("no gryd.yaml found: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n\033[1;36m⚡ Gryd Domain Routing Configuration\033[0m")
	fmt.Printf("Configuring project: \033[1m%s\033[0m (%s)\n\n", cfg.Name, configPath)

	// 1. Ask for local base domain (Default: gryd.test)
	fmt.Printf("\033[1m? Enter local base domain\033[0m \033[2m(e.g. gryd.test, localhost) [gryd.test]\033[0m: ")
	baseDomain, _ := readLineSafely(reader)
	baseDomain = strings.TrimSpace(baseDomain)
	if baseDomain == "" {
		baseDomain = "gryd.test"
	}
	baseDomain = strings.TrimPrefix(baseDomain, ".")

	fmt.Printf("✓ Using base domain: \033[1;32m.%s\033[0m\n\n", baseDomain)

	// 2. Iterate through each service deterministically sorted
	var svcNames []string
	for svcName := range cfg.Services {
		svcNames = append(svcNames, svcName)
	}
	sort.Strings(svcNames)

	configuredDomains := make(map[string]string)
	for _, svcName := range svcNames {
		svc := cfg.Services[svcName]
		defaultSub := strings.ToLower(svcName)
		currentDomain := svc.Domain
		promptDefault := defaultSub
		if currentDomain != "" {
			if strings.HasSuffix(currentDomain, "."+baseDomain) {
				promptDefault = strings.TrimSuffix(currentDomain, "."+baseDomain)
			}
		}

		portStr := "no port"
		if svc.Port > 0 {
			portStr = fmt.Sprintf(":%d", svc.Port)
		}

		fmt.Printf("\033[1m? Subdomain for service '%s' (%s)\033[0m \033[2m[%s] (or '-' to skip)\033[0m: ", svcName, portStr, promptDefault)
		subInput, _ := readLineSafely(reader)
		subInput = strings.TrimSpace(subInput)

		if subInput == "-" {
			svc.Domain = ""
			cfg.Services[svcName] = svc
			continue
		}

		subdomain := promptDefault
		if subInput != "" {
			subdomain = subInput
		}

		fullDomain := fmt.Sprintf("%s.%s", strings.ToLower(subdomain), strings.ToLower(baseDomain))
		svc.Domain = fullDomain
		cfg.Services[svcName] = svc
		configuredDomains[svcName] = fullDomain
		fmt.Printf("  ↳ Assigned: \033[1;32mhttp://%s\033[0m\n", fullDomain)
	}

	// 3. Write back the updated configuration preserving valid YAML structure
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	if err := os.WriteFile(configPath, yamlBytes, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	fmt.Printf("\n\033[1;32m✓ Successfully updated %s with local domain routing!\033[0m\n\n", configPath)
	fmt.Println("Configured routes:")
	for svcName, domain := range configuredDomains {
		fmt.Printf("  • %-16s ➔ \033[1mhttp://%s\033[0m\n", svcName, domain)
	}

	// 4. If baseDomain is not .localhost (e.g. gryd.test), check and suggest hosts file sync
	if !strings.HasSuffix(baseDomain, "localhost") {
		var domainList []string
		for _, d := range configuredDomains {
			domainList = append(domainList, d)
		}

		fmt.Printf("\n\033[1;33mNotice:\033[0m Domains ending in .%s require a local hosts entry to resolve.\n", baseDomain)
		fmt.Printf("\033[1m? Would you like to sync these domains to your system hosts file now?\033[0m \033[2m(Y/n) [Y]\033[0m: ")
		syncChoice, _ := readLineSafely(reader)
		syncChoice = strings.ToLower(strings.TrimSpace(syncChoice))
		if syncChoice == "" || syncChoice == "y" || syncChoice == "yes" {
			if err := proxy.SyncHostsFile(domainList); err != nil {
				fmt.Printf("  \033[1;31m✗\033[0m Could not update hosts file automatically: %v\n", err)
				fmt.Println("  Please run: \033[1msudo gryd hosts sync\033[0m (or run terminal as Administrator on Windows)")
			} else {
				_ = proxy.SetupPortForwarding(8080)
				fmt.Printf("  \033[1;32m✓\033[0m Hosts file and local port routing synchronized successfully!\n")
			}
		} else {
			fmt.Println("You can sync your hosts file at any time by running: \033[1msudo gryd hosts sync\033[0m")
		}
	}

	fmt.Printf("\nRun \033[1mgryd run\033[0m to start your services with domain routing active.\n\n")
	return nil
}

func init() {
	configCmd.Flags().BoolVar(&domainRoutingFlag, "domain-routing", false, "Configure local development domain routing interactively")
	configCmd.Flags().BoolVar(&removeDomainRoutingFlag, "remove-domain-routing", false, "Remove all configured domain routing and clean hosts")
	configCmd.Flags().BoolVar(&configJSONFlag, "json", false, "Output the configuration formatted as JSON")
	configCmd.Flags().BoolVarP(&configEditFlag, "edit", "e", false, "Open gryd.yaml in a verified text editor (nano or vim)")
	rootCmd.AddCommand(configCmd)
}
