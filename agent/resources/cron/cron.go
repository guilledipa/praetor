package cron

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/guilledipa/praetor/schema"
)

var execCommand = exec.Command

func init() {
	resources.RegisterType("cron", NewCron)
}

type Cron struct {
	schema.Cron
}

func NewCron(spec json.RawMessage) (resources.Resource, error) {
	var c schema.Cron
	if err := json.Unmarshal(spec, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cron spec: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &Cron{Cron: c}, nil
}

func (c *Cron) Type() string                      { return c.Kind }
func (c *Cron) ID() string                        { return c.ObjectMeta.Name }
func (c *Cron) Requires() []schema.Dependency     { return c.ObjectMeta.Requires }
func (c *Cron) Before() []schema.Dependency       { return c.ObjectMeta.Before }

func (c *Cron) generateCronLine() string {
	return fmt.Sprintf("%s %s %s %s %s %s", c.Spec.Minute, c.Spec.Hour, c.Spec.DayOfMonth, c.Spec.Month, c.Spec.DayOfWeek, c.Spec.Command)
}

func (c *Cron) getMarker() string {
	return fmt.Sprintf("# PRAETOR_ID: %s", c.ObjectMeta.Name)
}

func (c *Cron) Get() (resources.State, error) {
	cmd := execCommand("crontab", "-u", c.Spec.User, "-l")
	output, _ := cmd.Output() 

	state := resources.State{
		"ensure": "absent",
		"user":   c.Spec.User,
	}

	lines := strings.Split(string(output), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == c.getMarker() {
			state["ensure"] = "present"
			if i+1 < len(lines) {
				state["cronline"] = strings.TrimSpace(lines[i+1])
			}
			break
		}
	}

	return state, nil
}

func (c *Cron) Test(currentState resources.State) (bool, error) {
	if currentState["ensure"] != c.Spec.Ensure {
		return false, nil
	}
	if c.Spec.Ensure == "present" {
		if currentState["cronline"] != c.generateCronLine() {
			return false, nil
		}
	}
	return true, nil
}

func (c *Cron) Set() error {
	cmd := execCommand("crontab", "-u", c.Spec.User, "-l")
	output, _ := cmd.Output()

	lines := strings.Split(string(output), "\n")
	var newLines []string

	marker := c.getMarker()
	skipNext := false

	for _, line := range lines {
		if skipNext {
			skipNext = false
			continue
		}
		if strings.TrimSpace(line) == marker {
			skipNext = true
			continue
		}
		if strings.TrimSpace(line) != "" {
			newLines = append(newLines, line)
		}
	}

	if c.Spec.Ensure == "present" {
		newLines = append(newLines, marker, c.generateCronLine())
	}

	newCrontab := strings.Join(newLines, "\n") + "\n"

	setCmd := execCommand("crontab", "-u", c.Spec.User, "-")
	setCmd.Stdin = strings.NewReader(newCrontab)

	if out, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed configuring crontab: %s (%w)", string(out), err)
	}

	return nil
}
