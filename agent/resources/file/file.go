package file

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

// File represents a file resource to be managed.
type File struct {
	schema.File
}

// init registers the file resource type.
func init() {
	resources.RegisterType("File", func(spec json.RawMessage) (resources.Resource, error) {
		var f schema.File
		if err := json.Unmarshal(spec, &f); err != nil {
			return nil, fmt.Errorf("failed to unmarshal file spec: %w", err)
		}
		if err := f.Validate(); err != nil {
			return nil, fmt.Errorf("file spec validation failed: %w", err)
		}
		return &File{File: f}, nil
	})
}

// Type returns the resource type name.
func (f *File) Type() string {
	return f.Kind
}

// ID returns the unique identifier for this file.
func (f *File) ID() string {
	return f.Spec.Path
}

// Requires returns resources this file must run after.
func (f *File) Requires() []schema.Dependency {
	return f.ObjectMeta.Requires
}

// Before returns resources this file must explicitly run before.
func (f *File) Before() []schema.Dependency {
	return f.ObjectMeta.Before
}

// Get retrieves the current state of the file.
func (f *File) Get() (resources.State, error) {
	currentState := make(resources.State)
	_, err := os.Stat(f.Spec.Path)
	if err == nil {
		currentState["ensure"] = "present"
		content, err := os.ReadFile(f.Spec.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", f.Spec.Path, err)
		}
		currentState["content"] = string(content)
		// TODO: Get Mode, Owner, Group
	} else if os.IsNotExist(err) {
		currentState["ensure"] = "absent"
	} else {
		return nil, fmt.Errorf("failed to stat file %s: %w", f.Spec.Path, err)
	}
	return currentState, nil
}

// Test compares the current state against the desired state for the file.
func (f *File) Test(currentState resources.State) (bool, error) {
	desiredEnsure := f.Spec.Ensure
	currentEnsure, ok := currentState["ensure"].(string)
	if !ok {
		return false, fmt.Errorf("invalid state format for ensure")
	}

	if desiredEnsure == "present" {
		if currentEnsure != "present" {
			slog.Debug("File.Test: Drift detected", "id", f.ID(), "reason", "file not present")
			return false, nil
		}
		currentContent, ok := currentState["content"].(string)
		if !ok {
			return false, fmt.Errorf("invalid state format for content")
		}
		if currentContent != f.Spec.Content {
			slog.Debug("File.Test: Drift detected", "id", f.ID(), "reason", "content mismatch")
			return false, nil
		}
		// TODO: Test Mode, Owner, Group
	} else if desiredEnsure == "absent" {
		if currentEnsure != "absent" {
			slog.Debug("File.Test: Drift detected", "id", f.ID(), "reason", "file not absent")
			return false, nil
		}
	} else {
		return false, fmt.Errorf("invalid ensure value: %s", desiredEnsure)
	}
	slog.Debug("File.Test: No drift detected", "id", f.ID())
	return true, nil // No drift
}

// Set enforces the desired state for the file.
func (f *File) Set() error {
	if f.Spec.Ensure == "present" {
		slog.Info("File.Set: Ensuring file is present", "id", f.ID())
		// TODO: Set Mode, Owner, Group before writing
		err := os.WriteFile(f.Spec.Path, []byte(f.Spec.Content), 0644) // TODO: Use f.Spec.Mode
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", f.Spec.Path, err)
		}
		slog.Info("File.Set: Successfully wrote file", "id", f.ID())
	} else if f.Spec.Ensure == "absent" {
		slog.Info("File.Set: Ensuring file is absent", "id", f.ID())
		err := os.Remove(f.Spec.Path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove file %s: %w", f.Spec.Path, err)
		}
		slog.Info("File.Set: Successfully removed file", "id", f.ID())
	}
	return nil
}
