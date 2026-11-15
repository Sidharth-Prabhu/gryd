package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ToolConfig defines a custom path and requirements for an individual tool.
type ToolConfig struct {
	Path     string `yaml:"path,omitempty" json:"path,omitempty"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

// ToolsMap allows flexible YAML syntax:
//
// 1. Simple mapping of name to path:
//
//	tools:
//	  pnpm: /usr/local/bin/pnpm
//	  node: /opt/homebrew/bin/node
//
// 2. Structured mapping:
//
//	tools:
//	  pnpm:
//	    path: /usr/local/bin/pnpm
//	    required: true
//
// 3. Sequence / list:
//
//	tools:
//	  - pnpm
//	  - node
type ToolsMap map[string]ToolConfig

// UnmarshalYAML implements flexible unmarshaling for ToolsMap.
func (t *ToolsMap) UnmarshalYAML(value *yaml.Node) error {
	*t = make(map[string]ToolConfig)
	if value.Kind == yaml.SequenceNode {
		for _, item := range value.Content {
			if item.Kind == yaml.ScalarNode {
				var name string
				if err := item.Decode(&name); err != nil {
					return err
				}
				(*t)[name] = ToolConfig{}
			} else if item.Kind == yaml.MappingNode {
				var entry struct {
					Name     string `yaml:"name"`
					Path     string `yaml:"path"`
					Required bool   `yaml:"required"`
				}
				if err := item.Decode(&entry); err != nil {
					return err
				}
				if entry.Name != "" {
					(*t)[entry.Name] = ToolConfig{Path: entry.Path, Required: entry.Required}
				}
			}
		}
		return nil
	}
	if value.Kind == yaml.MappingNode {
		for i := 0; i < len(value.Content); i += 2 {
			var key string
			if err := value.Content[i].Decode(&key); err != nil {
				return err
			}
			valNode := value.Content[i+1]
			if valNode.Kind == yaml.ScalarNode {
				var path string
				if err := valNode.Decode(&path); err != nil {
					return err
				}
				(*t)[key] = ToolConfig{Path: path}
			} else if valNode.Kind == yaml.MappingNode {
				var tc ToolConfig
				if err := valNode.Decode(&tc); err != nil {
					return err
				}
				(*t)[key] = tc
			}
		}
		return nil
	}
	return fmt.Errorf("tools must be a mapping of tool names or a list")
}

// MarshalYAML implements custom marshaling for ToolsMap.
func (t ToolsMap) MarshalYAML() (interface{}, error) {
	if len(t) == 0 {
		return nil, nil
	}
	simple := true
	for _, v := range t {
		if v.Required {
			simple = false
			break
		}
	}
	if simple {
		strMap := make(map[string]string)
		for k, v := range t {
			strMap[k] = v.Path
		}
		return strMap, nil
	}
	res := make(map[string]interface{})
	for k, v := range t {
		res[k] = v
	}
	return res, nil
}
