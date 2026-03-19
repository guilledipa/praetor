package local

import (
	"os"
	"testing"
)

func TestLocalProvider(t *testing.T) {
	mockYaml := `
default:
  db-creds:
    password: "supersecret"
  api-keys:
    dd: "abc"
other:
  db-creds:
    password: "notsecret"
`
	tmpfile, err := os.CreateTemp("", "secrets*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(mockYaml)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	prov, err := NewProvider(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to instantiate provider: %v", err)
	}

	val, err := prov.GetSecret("default", "db-creds", "password")
	if err != nil || val != "supersecret" {
		t.Errorf("expected supersecret, got %q (err: %v)", val, err)
	}

	val, err = prov.GetSecret("other", "db-creds", "password")
	if err != nil || val != "notsecret" {
		t.Errorf("expected notsecret, got %q (err: %v)", val, err)
	}

	_, err = prov.GetSecret("missing", "secret", "key")
	if err == nil {
		t.Errorf("expected error for missing namespace but got nil")
	}
}
