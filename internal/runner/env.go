package runner

import (
	"fmt"
	"os"

	"github.com/loom-cli/gryd/internal/config"
	"github.com/loom-cli/gryd/internal/doctor"
)

// BuildServiceEnv prepares the environment variables for a service command,
// applying any configured tool path overrides and custom service variables.
func BuildServiceEnv(cfg *config.Config, svcEnv map[string]string) []string {
	base := doctor.ApplyToolsToEnv(cfg, os.Environ())
	for k, v := range svcEnv {
		base = append(base, fmt.Sprintf("%s=%s", k, v))
	}
	return base
}
