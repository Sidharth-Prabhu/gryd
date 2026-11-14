package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version information set at build time or defaults
var (
	Version   = "0.2.1"
	Commit    = "none"
	BuildDate = "unknown"
	Release   = "Phase 2 Release"
)

// FormatVersion returns the formatted version and release information.
func FormatVersion() string {
	return fmt.Sprintf("gryd version %s (%s)\nRelease:    %s\nGo version: %s\nPlatform:   %s/%s",
		Version, Commit, Release, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and release of Gryd",
	Long:  `Display detailed version, release name, and build information for Gryd.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(FormatVersion())
	},
}
