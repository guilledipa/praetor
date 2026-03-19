package group

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/guilledipa/praetor/agent/resources"
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

	if cmd == "getent" && cmdArgs[0] == "group" {
		if cmdArgs[1] == "existing-group" {
			fmt.Fprint(os.Stdout, "existing-group:x:1001:")
			os.Exit(0)
		} else if cmdArgs[1] == "missing-group" {
			os.Exit(2) // Missing
		}
	} else if cmd == "groupadd" {
		os.Exit(0) // Success mock
	} else if cmd == "groupdel" {
		os.Exit(0) // Success mock
	} else if cmd == "groupmod" {
		os.Exit(0) // Success mock
	}

	os.Exit(1)
}

func TestGroupGet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	// Test Present
	spec := []byte(`{"apiVersion": "v1", "kind": "Group", "metadata": {"name": "existing-group"}, "spec": {"ensure": "present"}}`)
	res, err := NewGroup(spec)
	if err != nil {
		t.Fatalf("Failed to create resource: %v", err)
	}

	state, err := res.Get()
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}

	if state["ensure"] != "present" {
		t.Errorf("Expected ensure present, got %v", state["ensure"])
	}

	// Test Absent
	specAbsent := []byte(`{"apiVersion": "v1", "kind": "Group", "metadata": {"name": "missing-group"}, "spec": {"ensure": "absent"}}`)
	resAbsent, _ := NewGroup(specAbsent)

	stateAbsent, _ := resAbsent.Get()
	if stateAbsent["ensure"] != "absent" {
		t.Errorf("Expected ensure absent, got %v", stateAbsent["ensure"])
	}
}

func TestGroupTest(t *testing.T) {
	spec := []byte(`{"apiVersion": "v1", "kind": "Group", "metadata": {"name": "existing-group"}, "spec": {"ensure": "present", "gid": 1001}}`)
	res, _ := NewGroup(spec)

	// Matching state
	matchState := resources.State{"ensure": "present", "gid": 1001}
	ok, _ := res.Test(matchState)
	if !ok {
		t.Errorf("Expected Test to return true for matching state")
	}

	// Drifting state
	driftState := resources.State{"ensure": "absent"}
	ok, _ = res.Test(driftState)
	if ok {
		t.Errorf("Expected Test to return false for drifting state")
	}
}

func TestGroupSet(t *testing.T) {
	oldExecCommand := execCommand
	execCommand = fakeExecCommand
	defer func() { execCommand = oldExecCommand }()

	// Test Set Create
	spec := []byte(`{"apiVersion": "v1", "kind": "Group", "metadata": {"name": "missing-group"}, "spec": {"ensure": "present", "gid": 1001}}`)
	res, _ := NewGroup(spec)
	err := res.Set()
	if err != nil {
		t.Errorf("Expected Set() successful execution, got err: %v", err)
	}
}
