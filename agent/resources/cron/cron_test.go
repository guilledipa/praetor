package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	cmd, cmdArgs := args[0], args[1:]

	if cmd == "crontab" && cmdArgs[0] == "-u" {
		if cmdArgs[1] == "existing_user" && len(cmdArgs) > 2 && cmdArgs[2] == "-l" {
			fmt.Fprint(os.Stdout, "0 0 * * * /bin/old_cron\n# PRAETOR_ID: my-cron\n* * * * * /bin/true\n30 * * * * /bin/other")
			os.Exit(0)
		} else if cmdArgs[1] == "missing_user" && len(cmdArgs) > 2 && cmdArgs[2] == "-l" {
			os.Exit(1)
		} else if len(cmdArgs) > 2 && cmdArgs[2] == "-" {
			// Write command mock
			os.Exit(0)
		}
	}

	os.Exit(1)
}

func TestCronGet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	spec := []byte(`{"apiVersion": "v1", "kind": "Cron", "metadata": {"name": "my-cron"}, "spec": {"ensure": "present", "user": "existing_user", "command": "/bin/true"}}`)
	res, _ := NewCron(spec)

	state, err := res.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if state["ensure"] != "present" {
		t.Errorf("Expected ensure present, got %v", state["ensure"])
	}
	if state["cronline"] != "* * * * * /bin/true" {
		t.Errorf("Expected cronline * * * * * /bin/true, got %v", state["cronline"])
	}
}

func TestCronSet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	spec := []byte(`{"apiVersion": "v1", "kind": "Cron", "metadata": {"name": "missing-cron"}, "spec": {"ensure": "present", "user": "missing_user", "command": "/bin/false"}}`)
	res, _ := NewCron(spec)

	err := res.Set()
	if err != nil {
		t.Errorf("Expected Set() successful execution, got err: %v", err)
	}
}
