package user

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
	resources.RegisterType("user", NewUser)
}

type User struct {
	schema.User
}

func NewUser(spec json.RawMessage) (resources.Resource, error) {
	var u schema.User
	if err := json.Unmarshal(spec, &u); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user spec: %w", err)
	}
	if err := u.Validate(); err != nil {
		return nil, err
	}
	return &User{User: u}, nil
}

func (u *User) Type() string                       { return u.Kind }
func (u *User) ID() string                         { return u.ObjectMeta.Name }
func (u *User) Requires() []schema.Dependency      { return u.ObjectMeta.Requires }
func (u *User) Before() []schema.Dependency        { return u.ObjectMeta.Before }

func (u *User) Get() (resources.State, error) {
	cmd := execCommand("getent", "passwd", u.ObjectMeta.Name)
	output, err := cmd.Output()

	state := resources.State{
		"name":   u.ObjectMeta.Name,
		"ensure": "absent",
	}

	if err == nil && len(output) > 0 {
		state["ensure"] = "present"
		parts := strings.Split(strings.TrimSpace(string(output)), ":")
		if len(parts) >= 7 {
			if uid, err := strconv.Atoi(parts[2]); err == nil {
				state["uid"] = uid
			}
			if gid, err := strconv.Atoi(parts[3]); err == nil {
				state["gid"] = gid
			}
			state["home"] = parts[5]
			state["shell"] = parts[6]
		}

		grpCmd := execCommand("id", "-Gn", u.ObjectMeta.Name)
		if grpOutput, err := grpCmd.Output(); err == nil {
			groups := strings.Split(strings.TrimSpace(string(grpOutput)), " ")
			var secondary []string
			for _, g := range groups {
				if g != u.ObjectMeta.Name {
					secondary = append(secondary, g)
				}
			}
			state["groups"] = secondary
		}
	}
	return state, nil
}

func (u *User) Test(currentState resources.State) (bool, error) {
	if currentState["ensure"] != u.Spec.Ensure {
		return false, nil
	}

	if u.Spec.Ensure == "present" {
		if u.Spec.Uid != nil {
			if actualUid, ok := currentState["uid"].(int); !ok || *u.Spec.Uid != actualUid {
				return false, nil
			}
		}
		if u.Spec.Gid != nil {
			if actualGid, ok := currentState["gid"].(int); !ok || *u.Spec.Gid != actualGid {
				return false, nil
			}
		}
		if u.Spec.Shell != "" {
			if currentState["shell"] != u.Spec.Shell {
				return false, nil
			}
		}
		if u.Spec.Home != "" {
			if currentState["home"] != u.Spec.Home {
				return false, nil
			}
		}
		
		if len(u.Spec.Groups) > 0 {
			actualGroups, ok := currentState["groups"].([]string)
			if !ok {
				return false, nil
			}
			for _, expectedGrp := range u.Spec.Groups {
				found := false
				for _, actualGrp := range actualGroups {
					if expectedGrp == actualGrp {
						found = true
						break
					}
				}
				if !found {
					return false, nil
				}
			}
		}
	}
	return true, nil
}

func (u *User) Set() error {
	if u.Spec.Ensure == "absent" {
		cmd := execCommand("userdel", u.ObjectMeta.Name)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to delete user: %s (%w)", string(out), err)
		}
		return nil
	}

	state, _ := u.Get()
	var bin string
	var args []string

	if state["ensure"] == "absent" {
		bin = "useradd"
		args = append(args, "-m") 
	} else {
		bin = "usermod"
	}

	if u.Spec.Uid != nil {
		args = append(args, "-u", strconv.Itoa(*u.Spec.Uid))
	}
	if u.Spec.Gid != nil {
		args = append(args, "-g", strconv.Itoa(*u.Spec.Gid))
	}
	if u.Spec.Shell != "" {
		args = append(args, "-s", u.Spec.Shell)
	}
	if u.Spec.Home != "" {
		args = append(args, "-d", u.Spec.Home)
	}
	if len(u.Spec.Groups) > 0 {
		args = append(args, "-G", strings.Join(u.Spec.Groups, ","))
	}

	args = append(args, u.ObjectMeta.Name)

	cmd := execCommand(bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed executing %s: %s (%w)", bin, string(out), err)
	}

	return nil
}
