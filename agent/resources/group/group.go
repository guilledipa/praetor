package group

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

var execCommand = exec.Command

func init() {
	resources.RegisterType("group", NewGroup)
}

type Group struct {
	schema.Group
}

func NewGroup(spec json.RawMessage) (resources.Resource, error) {
	var g schema.Group
	if err := json.Unmarshal(spec, &g); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group spec: %w", err)
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return &Group{Group: g}, nil
}

func (g *Group) Type() string                       { return g.Kind }
func (g *Group) ID() string                         { return g.ObjectMeta.Name }
func (g *Group) Requires() []schema.Dependency      { return g.ObjectMeta.Requires }
func (g *Group) Before() []schema.Dependency        { return g.ObjectMeta.Before }

func (g *Group) Get() (resources.State, error) {
	cmd := execCommand("getent", "group", g.ObjectMeta.Name)
	output, err := cmd.Output()

	state := resources.State{
		"name":   g.ObjectMeta.Name,
		"ensure": "absent",
	}

	if err == nil && len(output) > 0 {
		state["ensure"] = "present"
		parts := strings.Split(strings.TrimSpace(string(output)), ":")
		if len(parts) >= 3 {
			if gid, err := strconv.Atoi(parts[2]); err == nil {
				state["gid"] = gid
			}
		}
	}

	return state, nil
}

func (g *Group) Test(currentState resources.State) (bool, error) {
	expectedEnsure := g.Spec.Ensure
	actualEnsure, ok := currentState["ensure"].(string)
	if !ok || expectedEnsure != actualEnsure {
		return false, nil
	}

	if expectedEnsure == "present" && g.Spec.Gid != nil {
		actualGid, ok := currentState["gid"].(int)
		if !ok || *g.Spec.Gid != actualGid {
			return false, nil
		}
	}

	return true, nil
}

func (g *Group) Set() error {
	if g.Spec.Ensure == "absent" {
		cmd := execCommand("groupdel", g.ObjectMeta.Name)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete group: %s (%w)", string(out), err)
		}
		return nil
	}

	// Ensure == present
	state, _ := g.Get()
	if state["ensure"] == "absent" {
		// Create
		args := []string{g.ObjectMeta.Name}
		if g.Spec.Gid != nil {
			args = []string{"-g", strconv.Itoa(*g.Spec.Gid), g.ObjectMeta.Name}
		}
		cmd := execCommand("groupadd", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to create group: %s (%w)", string(out), err)
		}
	} else {
		// Modify Gid
		if g.Spec.Gid != nil {
			cmd := execCommand("groupmod", "-g", strconv.Itoa(*g.Spec.Gid), g.ObjectMeta.Name)
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to modify group gid: %s (%w)", string(out), err)
			}
		}
	}

	return nil
}
