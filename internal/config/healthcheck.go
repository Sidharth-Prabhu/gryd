package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// DepCondition defines detailed dependency requirements (e.g. condition: service_healthy).
type DepCondition struct {
	Condition string `yaml:"condition,omitempty"`
}

// DependsOnMap supports both list syntax:
//
//	depends_on:
//	  - backend
//
// and map syntax:
//
//	depends_on:
//	  backend:
//	    condition: service_healthy
type DependsOnMap map[string]DepCondition

// UnmarshalYAML implements custom unmarshaling for DependsOnMap.
func (d *DependsOnMap) UnmarshalYAML(value *yaml.Node) error {
	*d = make(map[string]DepCondition)
	if value.Kind == yaml.SequenceNode {
		for _, item := range value.Content {
			var name string
			if err := item.Decode(&name); err != nil {
				return err
			}
			(*d)[name] = DepCondition{}
		}
		return nil
	}
	if value.Kind == yaml.MappingNode {
		var raw map[string]DepCondition
		if err := value.Decode(&raw); err != nil {
			return err
		}
		*d = raw
		return nil
	}
	return fmt.Errorf("depends_on must be a list of service names or a mapping")
}

// MarshalYAML implements custom marshaling for DependsOnMap.
func (d DependsOnMap) MarshalYAML() (interface{}, error) {
	allSimple := true
	var list []string
	for k, v := range d {
		list = append(list, k)
		if v.Condition != "" {
			allSimple = false
		}
	}
	if allSimple {
		return list, nil
	}
	return map[string]DepCondition(d), nil
}

// Names returns all dependency names as a slice of strings.
func (d DependsOnMap) Names() []string {
	var names []string
	for k := range d {
		names = append(names, k)
	}
	return names
}

// HealthCheckConfig defines health readiness probes for a service.
type HealthCheckConfig struct {
	HTTP     string `yaml:"http,omitempty"`     // URL to poll (e.g., http://localhost:8000/health)
	TCP      string `yaml:"tcp,omitempty"`      // host:port to check (e.g., localhost:8000)
	Interval string `yaml:"interval,omitempty"` // polling interval (e.g. 2s, 500ms)
	Timeout  string `yaml:"timeout,omitempty"`  // request timeout (e.g. 1s)
	Retries  int    `yaml:"retries,omitempty"`  // max retries (default: 10)
}

// GetInterval returns the parsed interval or default 1s.
func (h *HealthCheckConfig) GetInterval() time.Duration {
	if h == nil || h.Interval == "" {
		return 1 * time.Second
	}
	d, err := time.ParseDuration(h.Interval)
	if err != nil || d <= 0 {
		return 1 * time.Second
	}
	return d
}

// GetTimeout returns the parsed timeout or default 2s.
func (h *HealthCheckConfig) GetTimeout() time.Duration {
	if h == nil || h.Timeout == "" {
		return 2 * time.Second
	}
	d, err := time.ParseDuration(h.Timeout)
	if err != nil || d <= 0 {
		return 2 * time.Second
	}
	return d
}

// GetRetries returns the max retries or default 10.
func (h *HealthCheckConfig) GetRetries() int {
	if h == nil || h.Retries <= 0 {
		return 10
	}
	return h.Retries
}
