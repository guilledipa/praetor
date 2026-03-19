package exec

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	osexec "os/exec"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

type Exec struct {
	schema.Exec
}

func init() {
	resources.RegisterType("Exec", func(spec json.RawMessage) (resources.Resource, error) {
		var e schema.Exec
		if err := json.Unmarshal(spec, &e); err != nil {
			return nil, fmt.Errorf("failed to unmarshal exec spec: %w", err)
		}
		if err := e.Validate(); err != nil {
			return nil, fmt.Errorf("exec spec validation failed: %w", err)
		}
		return &Exec{Exec: e}, nil
	})
}

func (e *Exec) Type() string { return e.Kind }
func (e *Exec) ID() string {
	name := e.Name
	if name == "" {
		name = e.Spec.Command
	}
	return name
}

// Requires returns resources this exec must run after.
func (e *Exec) Requires() []schema.Dependency {
	return e.ObjectMeta.Requires
}

// Before returns resources this exec must explicitly run before.
func (e *Exec) Before() []schema.Dependency {
	return e.ObjectMeta.Before
}

// Get retrieves the current state of the exec. Since execution is an action, its "state" is whether it needs to be run.
func (e *Exec) Get() (resources.State, error) {
	currentState := make(resources.State)
	
	shouldRun := true

	if e.Spec.Creates != "" {
		if _, err := os.Stat(e.Spec.Creates); err == nil {
			// File exists, do not run
			shouldRun = false
		}
	}

	if shouldRun && e.Spec.OnlyIf != "" {
		cmd := osexec.Command("sh", "-c", e.Spec.OnlyIf)
		if err := cmd.Run(); err != nil {
			// onlyif command failed (non-zero exit), so we should NOT run
			shouldRun = false
		}
	}

	if shouldRun && e.Spec.Unless != "" {
		cmd := osexec.Command("sh", "-c", e.Spec.Unless)
		if err := cmd.Run(); err == nil {
			// unless command succeeded (zero exit), so we should NOT run
			shouldRun = false
		}
	}

	currentState["should_run"] = shouldRun
	return currentState, nil
}

func (e *Exec) Test(currentState resources.State) (bool, error) {
	shouldRun, ok := currentState["should_run"].(bool)
	if !ok {
		return false, fmt.Errorf("invalid state format for should_run")
	}

	if shouldRun {
		slog.Debug("Exec.Test: execution required", "id", e.ID())
		return false, nil // Needs to execute, so test returns false
	}
	
	slog.Debug("Exec.Test: No execution required", "id", e.ID())
	return true, nil
}

func (e *Exec) Set() error {
	slog.Info("Exec.Set: Executing command", "id", e.ID(), "command", e.Spec.Command)
	cmd := osexec.Command("sh", "-c", e.Spec.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute command '%s': %s, %w", e.Spec.Command, string(out), err)
	}
	return nil
}
