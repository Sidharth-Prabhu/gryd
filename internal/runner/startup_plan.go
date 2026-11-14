package runner

import (
	"fmt"
	"strings"

	"github.com/loom-cli/gryd/internal/config"
)

// DependencyTier represents a group of services that can be started concurrently.
type DependencyTier []string

// BuildStartupPlan computes an ordered execution plan (topological sort / tiers)
// respecting service dependencies and healthcheck wait requirements.
func BuildStartupPlan(services map[string]config.Service) ([][]string, error) {
	inDegree := make(map[string]int)
	graph := make(map[string][]string) // dependency -> list of services depending on it

	for name := range services {
		inDegree[name] = 0
	}

	for name, svc := range services {
		for dep := range svc.DependsOn {
			if _, exists := services[dep]; exists {
				inDegree[name]++
				graph[dep] = append(graph[dep], name)
			}
		}
	}

	var tiers [][]string
	ready := make([]string, 0)
	for name, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, name)
		}
	}

	visitedCount := 0
	for len(ready) > 0 {
		currentTier := ready
		tiers = append(tiers, currentTier)
		visitedCount += len(currentTier)

		var nextReady []string
		for _, node := range currentTier {
			for _, dependent := range graph[node] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextReady = append(nextReady, dependent)
				}
			}
		}
		ready = nextReady
	}

	if visitedCount < len(services) {
		var cycleNodes []string
		for name, deg := range inDegree {
			if deg > 0 {
				cycleNodes = append(cycleNodes, name)
			}
		}
		return nil, fmt.Errorf("circular dependency detected involving services: %s", strings.Join(cycleNodes, ", "))
	}

	return tiers, nil
}
