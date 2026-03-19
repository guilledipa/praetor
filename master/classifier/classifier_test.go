package classifier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileClassifierEvaluate(t *testing.T) {
	dir := t.TempDir()

	classFile := filepath.Join(dir, "classification.yaml")
	classContent := `
classes:
  - name: "web servers"
    matchLabels:
      role: "web"
    roles:
      - "webrole"
`
	err := os.WriteFile(classFile, []byte(classContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write class file: %v", err)
	}

	rolesDir := filepath.Join(dir, "roles")
	err = os.MkdirAll(rolesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to write roles dir: %v", err)
	}

	roleFile := filepath.Join(rolesDir, "webrole.yaml")
	roleContent := `
spec:
  resources:
    - testResource: true
`
	err = os.WriteFile(roleFile, []byte(roleContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write role file: %v", err)
	}

	classifier := NewFileClassifier(classFile, rolesDir)

	facts := map[string]string{
		"role": "web",
	}

	resources, err := classifier.Evaluate("test-node", facts)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("Expected 1 resource, got %d", len(resources))
	}
}
