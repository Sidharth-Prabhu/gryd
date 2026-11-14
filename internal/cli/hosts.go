package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/proxy"
	"github.com/spf13/cobra"
)

var hostsCmd = &cobra.Command{
	Use:   "hosts",
	Short: "Manage local development domain resolution",
	Long: `Inspect and synchronize local development domain names (such as *.gryd.test)
into your system hosts file for offline, zero-configuration local routing.`,
}

var hostsListCmd = &cobra.Command{
	Use:   "list [project]",
	Short: "List all local domains configured for the project",
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
			return err
		}

		p := proxy.NewReverseProxyServer(cfg, nil)
		routes := p.GetRoutes()

		fmt.Printf("\n\033[1m⚡ Local Domains for '%s':\033[0m\n\n", cfg.Name)
		fmt.Printf("  %-32s %-16s %s\n", "DOMAIN", "SERVICE", "TARGET PORT")
		fmt.Printf("  %-32s %-16s %s\n", strings.Repeat("-", 32), strings.Repeat("-", 16), strings.Repeat("-", 12))

		for domain, r := range routes {
			resolved := "✓ (native)"
			if !strings.HasSuffix(domain, ".localhost") {
				if proxy.CheckHostsResolved(domain) {
					resolved = "✓ (/etc/hosts)"
				} else {
					resolved = "✗ (run 'gryd hosts sync')"
				}
			}
			fmt.Printf("  http://%-25s %-16s :%d  \033[2m%s\033[0m\n", domain, r.ServiceName, r.TargetPort, resolved)
		}
		fmt.Println()
		return nil
	},
}

var hostsSyncCmd = &cobra.Command{
	Use:   "sync [project]",
	Short: "Add local domains (.test) to system hosts file",
	Long:  "Writes local development domain entries into /etc/hosts (or Windows hosts) pointing to 127.0.0.1. May require sudo on macOS/Linux.",
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
			return err
		}

		p := proxy.NewReverseProxyServer(cfg, nil)
		domains := p.GetDomains()

		err = proxy.SyncHostsFile(domains)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n\033[1;31mError:\033[0m %v\n\n", err)
			return err
		}

		// Also configure port 80 forwarding so URLs work without specifying :8080
		_ = proxy.SetupPortForwarding(8080)

		fmt.Printf("\n\033[1;32m✓ Successfully synchronized local domains and port 80 routing!\033[0m\n\n")
		return nil
	},
}

var hostsCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all Gryd managed entries from system hosts file",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := proxy.RemoveHostsEntries()
		if err != nil {
			return err
		}
		proxy.RemovePortForwarding()
		fmt.Printf("\n\033[1;32m✓ Cleaned up all Gryd entries and port routing from system.\033[0m\n\n")
		return nil
	},
}

func init() {
	hostsCmd.AddCommand(hostsListCmd)
	hostsCmd.AddCommand(hostsSyncCmd)
	hostsCmd.AddCommand(hostsCleanCmd)
	rootCmd.AddCommand(hostsCmd)
}
