package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/guilledipa/praetor/agent/resources"
)

type NodeState int

const (
	StatePending NodeState = iota
	StateRunning
	StateSucceeded
	StateFailed
	StateSkipped
)

func (s NodeState) String() string {
	return [...]string{"Pending", "Running", "Succeeded", "Failed", "Skipped"}[s]
}

type scheduler struct {
	mu           sync.Mutex
	states       map[string]NodeState
	inDegree     map[string]int
	adj          map[string][]string
	resMap       map[string]resources.Resource
	readyQueue   chan string
	doneChan     chan struct{}
	totalNodes   int
	logger       *slog.Logger
	appliedCount int
	skipReasons  map[string]string
}

func newScheduler(rawList []resources.Resource, logger *slog.Logger) (*scheduler, error) {
	nodeNames := make([]string, len(rawList))
	resMap := make(map[string]resources.Resource)

	for i, res := range rawList {
		name := buildNodeName(res.Type(), res.ID())
		nodeNames[i] = name
		resMap[name] = res
	}

	adj := make(map[string][]string)
	inDegree := make(map[string]int)
	states := make(map[string]NodeState)

	for _, name := range nodeNames {
		inDegree[name] = 0
		states[name] = StatePending
	}

	for _, name := range nodeNames {
		res := resMap[name]

		// Requires: Dependency -> Me
		for _, req := range res.Requires() {
			reqName := buildNodeName(req.Kind, req.Name)
			if _, exists := resMap[reqName]; !exists {
				return nil, fmt.Errorf("resource %s requires missing dependency %s", name, reqName)
			}
			adj[reqName] = append(adj[reqName], name)
			inDegree[name]++
		}

		// Before: Me -> Dependency
		for _, bef := range res.Before() {
			befName := buildNodeName(bef.Kind, bef.Name)
			if _, exists := resMap[befName]; !exists {
				return nil, fmt.Errorf("resource %s specifies before missing dependency %s", name, befName)
			}
			adj[name] = append(adj[name], befName)
			inDegree[befName]++
		}
	}

	return &scheduler{
		states:      states,
		inDegree:    inDegree,
		adj:         adj,
		resMap:      resMap,
		readyQueue:  make(chan string, len(rawList)),
		doneChan:    make(chan struct{}),
		totalNodes:  len(rawList),
		logger:      logger,
		skipReasons: make(map[string]string),
	}, nil
}

func (s *scheduler) run(ctx context.Context, numWorkers int, applyFunc func(ctx context.Context, res resources.Resource) (bool, string, error)) {
	// Initialize ready queue with 0 in-degree nodes
	s.mu.Lock()
	initialReady := 0
	for name, deg := range s.inDegree {
		if deg == 0 {
			s.readyQueue <- name
			initialReady++
		}
	}
	if s.totalNodes == 0 {
		close(s.doneChan)
	}
	s.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case name, ok := <-s.readyQueue:
					if !ok {
						return
					}
					s.processNode(ctx, name, applyFunc)
				}
			}
		}(i)
	}

	// Wait for done state
	select {
	case <-ctx.Done():
		s.logger.Warn("Scheduler execution cancelled by context")
	case <-s.doneChan:
		s.logger.Info("Scheduler execution completed successfully")
	}

	close(s.readyQueue)
	wg.Wait()
}

func (s *scheduler) processNode(ctx context.Context, name string, applyFunc func(ctx context.Context, res resources.Resource) (bool, string, error)) {
	s.mu.Lock()
	if s.states[name] == StateSkipped {
		s.mu.Unlock()
		return
	}
	s.states[name] = StateRunning
	res := s.resMap[name]
	s.mu.Unlock()

	s.logger.Debug("Executing resource", "name", name)
	compliant, msg, err := applyFunc(ctx, res)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil || !compliant {
		s.states[name] = StateFailed
		s.logger.Error("Resource failed execution", "name", name, "error", err, "message", msg)
		s.skipDependents(name, name)
	} else {
		s.states[name] = StateSucceeded
		s.logger.Debug("Resource completed successfully", "name", name)
		for _, dep := range s.adj[name] {
			s.inDegree[dep]--
			if s.inDegree[dep] == 0 && s.states[dep] == StatePending {
				s.readyQueue <- dep
			}
		}
	}

	s.appliedCount++
	if s.appliedCount >= s.totalNodes {
		select {
		case <-s.doneChan:
		default:
			close(s.doneChan)
		}
	}
}

func (s *scheduler) skipDependents(name string, rootCause string) {
	for _, dep := range s.adj[name] {
		if s.states[dep] == StatePending || s.states[dep] == StateRunning {
			s.states[dep] = StateSkipped
			reason := fmt.Sprintf("Dependency %s failed", rootCause)
			s.skipReasons[dep] = reason
			s.logger.Warn("Skipping resource due to parent failure", "name", dep, "reason", reason)
			s.appliedCount++
			s.skipDependents(dep, rootCause)
		}
	}
}
