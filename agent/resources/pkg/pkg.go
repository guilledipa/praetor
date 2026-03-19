package pkg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

// Provider abstracts the underlying package manager (apt, yum, etc.)
type Provider interface {
	Name() string
	IsInstalled(pkgName string) (bool, error)
	Install(pkgName string) error
	Remove(pkgName string) error
}

func getProvider() (Provider, error) {
	if _, err := exec.LookPath("apt-get"); err == nil {
		return &aptProvider{}, nil
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return &dnfProvider{}, nil
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return &yumProvider{}, nil
	}
	if _, err := exec.LookPath("zypper"); err == nil {
		return &zypperProvider{}, nil
	}
	if _, err := exec.LookPath("apk"); err == nil {
		return &apkProvider{}, nil
	}
	return nil, fmt.Errorf("no supported package manager found on this system")
}

// aptProvider implements Provider for Debian/Ubuntu
type aptProvider struct{}
func (p *aptProvider) Name() string { return "apt" }
func (p *aptProvider) IsInstalled(name string) (bool, error) {
	err := exec.Command("dpkg", "-s", name).Run()
	return err == nil, nil
}
func (p *aptProvider) Install(name string) error {
	out, err := exec.Command("apt-get", "install", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apt-get install failed: %s, %w", string(out), err)
	}
	return nil
}
func (p *aptProvider) Remove(name string) error {
	out, err := exec.Command("apt-get", "remove", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apt-get remove failed: %s, %w", string(out), err)
	}
	return nil
}

// dnfProvider implements Provider for modern RHEL/Fedora
type dnfProvider struct{}
func (p *dnfProvider) Name() string { return "dnf" }
func (p *dnfProvider) IsInstalled(name string) (bool, error) {
	err := exec.Command("rpm", "-q", name).Run()
	return err == nil, nil
}
func (p *dnfProvider) Install(name string) error {
	out, err := exec.Command("dnf", "install", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dnf install failed: %s, %w", string(out), err)
	}
	return nil
}
func (p *dnfProvider) Remove(name string) error {
	out, err := exec.Command("dnf", "remove", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dnf remove failed: %s, %w", string(out), err)
	}
	return nil
}

// yumProvider implements Provider for legacy RHEL/CentOS
type yumProvider struct{}
func (p *yumProvider) Name() string { return "yum" }
func (p *yumProvider) IsInstalled(name string) (bool, error) {
	err := exec.Command("rpm", "-q", name).Run()
	return err == nil, nil
}
func (p *yumProvider) Install(name string) error {
	out, err := exec.Command("yum", "install", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("yum install failed: %s, %w", string(out), err)
	}
	return nil
}
func (p *yumProvider) Remove(name string) error {
	out, err := exec.Command("yum", "remove", "-y", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("yum remove failed: %s, %w", string(out), err)
	}
	return nil
}

// zypperProvider implements Provider for SUSE
type zypperProvider struct{}
func (p *zypperProvider) Name() string { return "zypper" }
func (p *zypperProvider) IsInstalled(name string) (bool, error) {
	err := exec.Command("rpm", "-q", name).Run()
	return err == nil, nil
}
func (p *zypperProvider) Install(name string) error {
	out, err := exec.Command("zypper", "--non-interactive", "install", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("zypper install failed: %s, %w", string(out), err)
	}
	return nil
}
func (p *zypperProvider) Remove(name string) error {
	out, err := exec.Command("zypper", "--non-interactive", "remove", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("zypper remove failed: %s, %w", string(out), err)
	}
	return nil
}

// apkProvider implements Provider for Alpine
type apkProvider struct{}
func (p *apkProvider) Name() string { return "apk" }
func (p *apkProvider) IsInstalled(name string) (bool, error) {
	err := exec.Command("apk", "info", "-e", name).Run()
	return err == nil, nil
}
func (p *apkProvider) Install(name string) error {
	out, err := exec.Command("apk", "add", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apk add failed: %s, %w", string(out), err)
	}
	return nil
}
func (p *apkProvider) Remove(name string) error {
	out, err := exec.Command("apk", "del", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apk del failed: %s, %w", string(out), err)
	}
	return nil
}


// Package represents a package resource to be managed.
type Package struct {
	schema.Package
}

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

// ID returns the unique identifier for this package resource.
func (p *Package) ID() string {
	return p.Spec.Name
}

// Requires returns resources this package must run after.
func (p *Package) Requires() []schema.Dependency {
	return p.ObjectMeta.Requires
}

// Before returns resources this package must explicitly run before.
func (p *Package) Before() []schema.Dependency {
	return p.ObjectMeta.Before
}

// Get retrieves the current state of the package.
func (p *Package) Get() (resources.State, error) {
	currentState := make(resources.State)
	
	provider, err := getProvider()
	if err != nil {
		return nil, err
	}

	installed, err := provider.IsInstalled(p.Spec.Name)
	if installed {
		currentState["ensure"] = "present"
	} else {
		currentState["ensure"] = "absent"
	}
	
	return currentState, nil
}

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

// Set enforces the desired state of the package.
func (p *Package) Set() error {
	desiredEnsure := p.Spec.Ensure
	if desiredEnsure == "latest" {
		desiredEnsure = "present"
	}

	provider, err := getProvider()
	if err != nil {
		return err
	}

	if desiredEnsure == "present" {
		slog.Info("Package.Set: Ensuring package is present", "id", p.ID(), "provider", provider.Name())
		if err := provider.Install(p.Spec.Name); err != nil {
			return err
		}
		slog.Info("Package.Set: Successfully installed package", "id", p.ID())
	} else if desiredEnsure == "absent" {
		slog.Info("Package.Set: Ensuring package is absent", "id", p.ID(), "provider", provider.Name())
		if err := provider.Remove(p.Spec.Name); err != nil {
			return err
		}
		slog.Info("Package.Set: Successfully removed package", "id", p.ID())
	}
	return nil
}
