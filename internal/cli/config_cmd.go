package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/proxy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	domainRoutingFlag bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure Gryd settings and project features",
	Long: `Interactive configuration helper for gryd.yaml.
Use --domain-routing to interactively assign local development subdomains
and domain routing for your services.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if domainRoutingFlag {
			return runDomainRoutingConfig()
		}
		return cmd.Help()
	},
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

	// 2. Iterate through each service with a configured port
	configuredDomains := make(map[string]string)
	for svcName, svc := range cfg.Services {
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
		fmt.Printf("\033[1m? Would you like to sync these domains to /etc/hosts now?\033[0m \033[2m(Y/n) [Y]\033[0m: ")
		syncChoice, _ := readLineSafely(reader)
		syncChoice = strings.ToLower(strings.TrimSpace(syncChoice))
		if syncChoice == "" || syncChoice == "y" || syncChoice == "yes" {
			if err := proxy.SyncHostsFile(domainList); err != nil {
				fmt.Printf("  \033[1;31m✗\033[0m Could not update hosts file automatically: %v\n", err)
				fmt.Println("  Please run: \033[1msudo gryd hosts sync\033[0m")
			} else {
				fmt.Printf("  \033[1;32m✓\033[0m Hosts file synchronized successfully!\n")
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
	rootCmd.AddCommand(configCmd)
}
