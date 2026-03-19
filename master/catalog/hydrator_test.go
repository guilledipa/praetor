package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

// mockProvider implements secrets.Provider for testing
type mockProvider struct {
	data map[string]string
}

func (m mockProvider) GetSecret(namespace, name, key string) (string, error) {
	val, exists := m.data[namespace+"/"+name+"/"+key]
	if !exists {
		return "", nil // or error, but let's just return empty for mock
	}
	return val, nil
}

func TestHydrateCatalog(t *testing.T) {
	rawJSON := []byte(`{
		"apiVersion": "praetor.io/v1alpha1",
		"kind": "File",
		"metadata": {"name": "test"},
		"spec": {
			"path": "/tmp/test.txt",
			"content": "OS IS {{ .facts.os }} AND PASS IS {{ secret \"default\" \"db\" \"pw\" }}"
		}
	}`)

	rawList := []json.RawMessage{rawJSON}

	facts := map[string]string{
		"os": "ubuntu",
	}

	secProv := mockProvider{
		data: map[string]string{
			"default/db/pw": "hunter2",
		},
	}

	hydrated, err := HydrateCatalog(rawList, facts, secProv)
	if err != nil {
		t.Fatalf("HydrateCatalog failed: %v", err)
	}

	if len(hydrated) != 1 {
		t.Fatalf("expected 1 separated resource, got %d", len(hydrated))
	}

	outJSON, _ := json.Marshal(hydrated[0])
	outStr := string(outJSON)

	expectedSubstring := "OS IS ubuntu AND PASS IS hunter2"
	if !strings.Contains(outStr, expectedSubstring) {
		t.Errorf("Expected hydrator to inject %q but it did not. Output: %s", expectedSubstring, outStr)
	}
}
