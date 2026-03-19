package user

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

	if cmd == "getent" && cmdArgs[0] == "passwd" {
		if cmdArgs[1] == "existing-user" {
			fmt.Fprint(os.Stdout, "existing-user:x:1001:1001:GECOS:/home/existing-user:/bin/bash")
			os.Exit(0)
		} else {
			os.Exit(2)
		}
	} else if cmd == "id" {
		fmt.Fprint(os.Stdout, "existing-user app-workers")
		os.Exit(0)
	} else if cmd == "useradd" || cmd == "userdel" || cmd == "usermod" {
		os.Exit(0) // Success
	}

	os.Exit(1)
}

func TestUserGet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	spec := []byte(`{"apiVersion": "v1", "kind": "User", "metadata": {"name": "existing-user"}, "spec": {"ensure": "present"}}`)
	res, _ := NewUser(spec)

	state, err := res.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if state["ensure"] != "present" {
		t.Errorf("Expected ensure present, got %v", state["ensure"])
	}
	if state["uid"] != 1001 {
		t.Errorf("Expected uid 1001, got %v", state["uid"])
	}
	if state["shell"] != "/bin/bash" {
		t.Errorf("Expected shell /bin/bash, got %v", state["shell"])
	}
	groups, ok := state["groups"].([]string)
	if !ok || len(groups) != 1 || groups[0] != "app-workers" {
		t.Errorf("Expected groups [app-workers], got %v", state["groups"])
	}
}

func TestUserSet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	spec := []byte(`{"apiVersion": "v1", "kind": "User", "metadata": {"name": "missing-user"}, "spec": {"ensure": "present", "uid": 1002}}`)
	res, _ := NewUser(spec)
	err := res.Set()
	if err != nil {
		t.Errorf("Expected Set() successful execution, got err: %v", err)
	}
}
