package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/guilledipa/praetor/agent/resources"
	"github.com/stretchr/testify/assert"
)

type dummyResource struct {
	id       string
	kind     string
	requires []resources.Dependency
	before   []resources.Dependency
}

func (d *dummyResource) Type() string { return d.kind }
func (d *dummyResource) ID() string   { return d.id }
func (d *dummyResource) Get() (resources.State, error) {
	return resources.State{}, nil
}
func (d *dummyResource) Test(currentState resources.State) (bool, error) {
	return true, nil
}
func (d *dummyResource) Set() error { return nil }
func (d *dummyResource) Requires() []resources.Dependency { return d.requires }
func (d *dummyResource) Before() []resources.Dependency   { return d.before }

func TestScheduler_SuccessPath(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	resA := &dummyResource{id: "A", kind: "File"}
	resB := &dummyResource{id: "B", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "A"}}}
	resC := &dummyResource{id: "C", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "B"}}}

	sched, err := newScheduler([]resources.Resource{resA, resB, resC}, logger)
	assert.NoError(t, err)

	var mu sync.Mutex
	var runOrder []string

	applyFunc := func(ctx context.Context, res resources.Resource) (bool, string, error) {
		mu.Lock()
		runOrder = append(runOrder, res.ID())
		mu.Unlock()
		return true, "Success", nil
	}

	sched.run(context.Background(), 2, applyFunc)

	assert.Equal(t, []string{"A", "B", "C"}, runOrder)
	assert.Equal(t, StateSucceeded, sched.states["File[A]"])
	assert.Equal(t, StateSucceeded, sched.states["File[B]"])
	assert.Equal(t, StateSucceeded, sched.states["File[C]"])
}

func TestScheduler_FailureBypass(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	resA := &dummyResource{id: "A", kind: "File"}
	resB := &dummyResource{id: "B", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "A"}}}
	resC := &dummyResource{id: "C", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "B"}}}
	resD := &dummyResource{id: "D", kind: "File"} // Independent

	sched, err := newScheduler([]resources.Resource{resA, resB, resC, resD}, logger)
	assert.NoError(t, err)

	applyFunc := func(ctx context.Context, res resources.Resource) (bool, string, error) {
		if res.ID() == "A" {
			return false, "Failed deliberately", fmt.Errorf("A failed")
		}
		return true, "Success", nil
	}

	sched.run(context.Background(), 2, applyFunc)

	assert.Equal(t, StateFailed, sched.states["File[A]"])
	assert.Equal(t, StateSkipped, sched.states["File[B]"])
	assert.Equal(t, StateSkipped, sched.states["File[C]"])
	assert.Equal(t, StateSucceeded, sched.states["File[D]"])
}

func TestScheduler_Parallelism(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	resA := &dummyResource{id: "A", kind: "File"}
	resB := &dummyResource{id: "B", kind: "File"}

	sched, err := newScheduler([]resources.Resource{resA, resB}, logger)
	assert.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)

	applyFunc := func(ctx context.Context, res resources.Resource) (bool, string, error) {
		wg.Done()
		wg.Wait() // Blocks until both workers are inside applyFunc (confirming concurrent execution)
		return true, "Success", nil
	}

	// Run with 2 workers. If execution is sequential, wg.Wait() will deadlock and test will timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sched.run(ctx, 2, applyFunc)

	assert.Equal(t, StateSucceeded, sched.states["File[A]"])
	assert.Equal(t, StateSucceeded, sched.states["File[B]"])
}
