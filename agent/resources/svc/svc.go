package svc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

// Service represents a service resource to be managed.
type Service struct {
	schema.Service
}

// init registers the service resource type.
func init() {
	resources.RegisterType("Service", func(spec json.RawMessage) (resources.Resource, error) {
		var s schema.Service
		if err := json.Unmarshal(spec, &s); err != nil {
			return nil, fmt.Errorf("failed to unmarshal service spec: %w", err)
		}
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("service spec validation failed: %w", err)
		}
		return &Service{Service: s}, nil
	})
}

func (s *Service) Type() string { return s.Kind }

// ID returns the unique identifier for this service resource.
func (s *Service) ID() string {
	return s.Spec.Name
}

// Requires returns resources this service must run after.
func (s *Service) Requires() []schema.Dependency {
	return s.Metadata.Requires
}

// Before returns resources this service must explicitly run before.
func (s *Service) Before() []schema.Dependency {
	return s.Metadata.Before
}

func (s *Service) Get() (resources.State, error) {
	currentState := make(resources.State)
	
	initSys, err := detectInitSystem()
	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	if initSys == "systemd" {
		cmd = exec.Command("systemctl", "is-active", s.Spec.Name)
	} else {
		cmd = exec.Command("service", s.Spec.Name, "status")
	}

	err = cmd.Run()
	if err == nil {
		currentState["ensure"] = "running"
	} else {
		currentState["ensure"] = "stopped"
	}

	if initSys == "systemd" {
		cmdEn := exec.Command("systemctl", "is-enabled", s.Spec.Name)
		errEn := cmdEn.Run()
		currentState["enable"] = (errEn == nil)
	} else {
		currentState["enable"] = false 
	}
	
	return currentState, nil
}

func (s *Service) Test(currentState resources.State) (bool, error) {
	currentEnsure, _ := currentState["ensure"].(string)
	currentEnable, _ := currentState["enable"].(bool)

	if s.Spec.Ensure != currentEnsure {
		slog.Debug("Service.Test: Drift detected", "id", s.ID(), "reason", "ensure mismatch")
		return false, nil
	}
	
	if s.Spec.Enable != currentEnable {
		initSys, _ := detectInitSystem()
		if initSys == "systemd" {
			slog.Debug("Service.Test: Drift detected", "id", s.ID(), "reason", "enable mismatch")
			return false, nil
		}
	}

	slog.Debug("Service.Test: No drift detected", "id", s.ID())
	return true, nil
}

func detectInitSystem() (string, error) {
	if _, err := exec.LookPath("systemctl"); err == nil {
		return "systemd", nil
	}
	if _, err := exec.LookPath("service"); err == nil {
		return "sysvinit", nil
	}
	return "", fmt.Errorf("no supported init system found on this system")
}

func (s *Service) Set() error {
	initSys, err := detectInitSystem()
	if err != nil {
		return err
	}

	if s.Spec.Ensure == "running" {
		slog.Info("Service.Set: Starting service", "id", s.ID())
		var cmd *exec.Cmd
		if initSys == "systemd" {
			cmd = exec.Command("systemctl", "start", s.Spec.Name)
		} else {
			cmd = exec.Command("service", s.Spec.Name, "start")
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start service %s: %w", s.Spec.Name, err)
		}
	} else if s.Spec.Ensure == "stopped" {
		slog.Info("Service.Set: Stopping service", "id", s.ID())
		var cmd *exec.Cmd
		if initSys == "systemd" {
			cmd = exec.Command("systemctl", "stop", s.Spec.Name)
		} else {
			cmd = exec.Command("service", s.Spec.Name, "stop")
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stop service %s: %w", s.Spec.Name, err)
		}
	}

	if initSys == "systemd" {
		if s.Spec.Enable {
			slog.Info("Service.Set: Enabling service", "id", s.ID())
			cmd := exec.Command("systemctl", "enable", s.Spec.Name)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to enable service %s: %w", s.Spec.Name, err)
			}
		} else {
			slog.Info("Service.Set: Disabling service", "id", s.ID())
			cmd := exec.Command("systemctl", "disable", s.Spec.Name)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to disable service %s: %w", s.Spec.Name, err)
			}
		}
	} else {
		slog.Warn("Service.Set: Enable/disable not supported for sysvinit fallback", "id", s.ID())
	}

	return nil
}
