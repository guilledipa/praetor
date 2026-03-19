package main

import (
	"fmt"
	"github.com/guilledipa/praetor/agent/resources"
)

// buildNodeName formats the strict DAG Node string
func buildNodeName(kind, id string) string {
	return fmt.Sprintf("%s[%s]", kind, id)
}

// buildDAG topologically sorts the resource slice using Kahn's algorithm
func buildDAG(rawList []resources.Resource) ([]resources.Resource, error) {
	nodeNames := make([]string, len(rawList))
	resMap := make(map[string]resources.Resource)

	for i, res := range rawList {
		name := buildNodeName(res.Type(), res.ID())
		nodeNames[i] = name
		resMap[name] = res
	}

	adj := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, name := range nodeNames {
		inDegree[name] = 0
	}

	for _, name := range nodeNames {
		res := resMap[name]

		// Requires: Dependencies must evaluate BEFORE me. Edge: Dependency -> Me
		for _, req := range res.Requires() {
			reqName := buildNodeName(req.Kind, req.Name)
			if _, exists := resMap[reqName]; !exists {
				return nil, fmt.Errorf("resource %s requires missing dependency %s", name, reqName)
			}
			adj[reqName] = append(adj[reqName], name)
			inDegree[name]++
		}

		// Before: I must evaluate BEFORE dependency. Edge: Me -> Dependency
		for _, bef := range res.Before() {
			befName := buildNodeName(bef.Kind, bef.Name)
			if _, exists := resMap[befName]; !exists {
				return nil, fmt.Errorf("resource %s specifies before missing dependency %s", name, befName)
			}
			adj[name] = append(adj[name], befName)
			inDegree[befName]++
		}
	}

	queue := make([]string, 0)
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var sortedNames []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		sortedNames = append(sortedNames, u)

		for _, v := range adj[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if len(sortedNames) != len(nodeNames) {
		return nil, fmt.Errorf("cyclic dependency detected inside the catalog payload")
	}

	sortedResources := make([]resources.Resource, len(sortedNames))
	for i, name := range sortedNames {
		sortedResources[i] = resMap[name]
	}

	return sortedResources, nil
}
