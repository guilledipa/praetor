package pkg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

// Package represents a package resource to be managed.
type Package struct {
	schema.Package
}

// init registers the package resource type.
func init() {
	resources.RegisterType("Package", func(spec json.RawMessage) (resources.Resource, error) {
		var p schema.Package
		if err := json.Unmarshal(spec, &p); err != nil {
			return nil, fmt.Errorf("failed to unmarshal package spec: %w", err)
		}
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("package spec validation failed: %w", err)
		}
		return &Package{Package: p}, nil
	})
}

// Type returns the resource type name.
func (p *Package) Type() string {
	return p.Kind
}

// ID returns the name of the package.
func (p *Package) ID() string {
	return p.Spec.Name
}

// Get retrieves the current state of the package.
func (p *Package) Get() (resources.State, error) {
	currentState := make(resources.State)
	
	cmd := exec.Command("dpkg", "-s", p.Spec.Name)
	err := cmd.Run()
	if err == nil {
		currentState["ensure"] = "present"
	} else {
		currentState["ensure"] = "absent"
	}
	
	return currentState, nil
}

// Test compares the current state against the desired state for the package.
func (p *Package) Test(currentState resources.State) (bool, error) {
	desiredEnsure := p.Spec.Ensure
	if desiredEnsure == "latest" {
		desiredEnsure = "present" // Basic implementation treats latest as present
	}
	currentEnsure, ok := currentState["ensure"].(string)
	if !ok {
		return false, fmt.Errorf("invalid state format for ensure")
	}

	if desiredEnsure == "present" {
		if currentEnsure != "present" {
			slog.Debug("Package.Test: Drift detected", "id", p.ID(), "reason", "package not present")
			return false, nil
		}
	} else if desiredEnsure == "absent" {
		if currentEnsure != "absent" {
			slog.Debug("Package.Test: Drift detected", "id", p.ID(), "reason", "package not absent")
			return false, nil
		}
	} else {
		return false, fmt.Errorf("invalid ensure value: %s", desiredEnsure)
	}
	slog.Debug("Package.Test: No drift detected", "id", p.ID())
	return true, nil
}

// Set enforces the desired state for the package.
func (p *Package) Set() error {
	desiredEnsure := p.Spec.Ensure
	if desiredEnsure == "latest" {
		desiredEnsure = "present"
	}

	if desiredEnsure == "present" {
		slog.Info("Package.Set: Ensuring package is present", "id", p.ID())
		cmd := exec.Command("apt-get", "install", "-y", p.Spec.Name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to install package %s: %s, %w", p.Spec.Name, string(out), err)
		}
		slog.Info("Package.Set: Successfully installed package", "id", p.ID())
	} else if desiredEnsure == "absent" {
		slog.Info("Package.Set: Ensuring package is absent", "id", p.ID())
		cmd := exec.Command("apt-get", "remove", "-y", p.Spec.Name)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to remove package %s: %s, %w", p.Spec.Name, string(out), err)
		}
		slog.Info("Package.Set: Successfully removed package", "id", p.ID())
	}
	return nil
}
