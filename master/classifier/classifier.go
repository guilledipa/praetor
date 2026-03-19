package classifier

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type NodeClass struct {
	Name        string            `yaml:"name"`
	MatchLabels map[string]string `yaml:"matchLabels"`
	Roles       []string          `yaml:"roles"`
}

type Classifier interface {
	Evaluate(nodeID string, facts map[string]string) ([]json.RawMessage, error)
}

type FileClassifier struct {
	classFilePath string
	rolesDir      string
}

func NewFileClassifier(classFilePath, rolesDir string) *FileClassifier {
	return &FileClassifier{
		classFilePath: classFilePath,
		rolesDir:      rolesDir,
	}
}

func (c *FileClassifier) Evaluate(nodeID string, facts map[string]string) ([]json.RawMessage, error) {
	var cls struct {
		Classes []NodeClass `yaml:"classes"`
	}

	clsFile, err := ioutil.ReadFile(c.classFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read classification file: %w", err)
	}

	if err := yaml.Unmarshal(clsFile, &cls); err != nil {
		return nil, fmt.Errorf("failed to parse classification file: %w", err)
	}

	matchedRoles := make(map[string]bool)
	for _, class := range cls.Classes {
		match := true
		for k, v := range class.MatchLabels {
			if k == "node_id" {
				if nodeID != v {
					match = false
					break
				}
			} else {
				if facts[k] != v {
					match = false
					break
				}
			}
		}
		if match {
			for _, r := range class.Roles {
				matchedRoles[r] = true
			}
		}
	}

	var rawResources []json.RawMessage
	for role := range matchedRoles {
		roleFile, err := ioutil.ReadFile(fmt.Sprintf("%s/%s.yaml", c.rolesDir, role))
		if err != nil {
			continue // Or log it if log is passed
		}

		var catalogContainer map[string]any
		if err := yaml.Unmarshal(roleFile, &catalogContainer); err != nil {
			continue
		}

		spec, ok := catalogContainer["spec"].(map[string]any)
		if !ok {
			continue
		}
		resources, ok := spec["resources"].([]any)
		if !ok {
			continue
		}

		for _, res := range resources {
			raw, err := json.Marshal(res)
			if err != nil {
				continue
			}
			rawResources = append(rawResources, raw)
		}
	}

	return rawResources, nil
}
