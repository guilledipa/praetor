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
func (s *Service) ID() string   { return s.Spec.Name }

func (s *Service) Get() (resources.State, error) {
	currentState := make(resources.State)
	
	cmd := exec.Command("systemctl", "is-active", s.Spec.Name)
	err := cmd.Run()
	if err == nil {
		currentState["ensure"] = "running"
	} else {
		currentState["ensure"] = "stopped"
	}

	cmdEn := exec.Command("systemctl", "is-enabled", s.Spec.Name)
	errEn := cmdEn.Run()
	currentState["enable"] = (errEn == nil)
	
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
		slog.Debug("Service.Test: Drift detected", "id", s.ID(), "reason", "enable mismatch")
		return false, nil
	}

	slog.Debug("Service.Test: No drift detected", "id", s.ID())
	return true, nil
}

func (s *Service) Set() error {
	if s.Spec.Ensure == "running" {
		slog.Info("Service.Set: Starting service", "id", s.ID())
		cmd := exec.Command("systemctl", "start", s.Spec.Name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to start service %s: %w", s.Spec.Name, err)
		}
	} else if s.Spec.Ensure == "stopped" {
		slog.Info("Service.Set: Stopping service", "id", s.ID())
		cmd := exec.Command("systemctl", "stop", s.Spec.Name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stop service %s: %w", s.Spec.Name, err)
		}
	}

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

	return nil
}
