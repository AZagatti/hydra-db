package execution

import (
	"fmt"
)

// DAG models a directed acyclic graph of Jobs with dependency edges, enabling
// ordered execution of interdependent work.
type DAG struct {
	nodes map[string]*Job
	edges map[string][]string
}

// NewDAG creates an empty DAG with no jobs or edges.
func NewDAG() *DAG {
	return &DAG{
		nodes: make(map[string]*Job),
		edges: make(map[string][]string),
	}
}

// AddJob inserts a Job as a node in the graph, rejecting duplicate IDs.
func (d *DAG) AddJob(job *Job) error {
	if _, exists := d.nodes[job.ID]; exists {
		return fmt.Errorf("job %q already exists", job.ID)
	}
	d.nodes[job.ID] = job
	return nil
}

// AddDependency creates a directed edge from parent to child, rolling back if
// the edge would introduce a cycle.
func (d *DAG) AddDependency(parentID, childID string) error {
	if _, ok := d.nodes[parentID]; !ok {
		return fmt.Errorf("parent job %q not found", parentID)
	}
	if _, ok := d.nodes[childID]; !ok {
		return fmt.Errorf("child job %q not found", childID)
	}

	d.edges[parentID] = append(d.edges[parentID], childID)

	if d.HasCycle() {
		d.edges[parentID] = d.edges[parentID][:len(d.edges[parentID])-1]
		return fmt.Errorf("adding dependency %q -> %q would create a cycle", parentID, childID)
	}

	return nil
}

// HasCycle performs a DFS-based cycle detection across the entire graph.
func (d *DAG) HasCycle() bool {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int)
	for id := range d.nodes {
		color[id] = white
	}

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for _, child := range d.edges[node] {
			switch color[child] {
			case gray:
				return true
			case white:
				if dfs(child) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}

	for id := range d.nodes {
		if color[id] == white {
			if dfs(id) {
				return true
			}
		}
	}
	return false
}

// TopologicalSort returns a valid execution order using Kahn's algorithm,
// ensuring every job appears after its dependencies.
func (d *DAG) TopologicalSort() ([]string, error) {
	if d.HasCycle() {
		return nil, fmt.Errorf("DAG contains a cycle")
	}

	inDegree := make(map[string]int)
	for id := range d.nodes {
		inDegree[id] = 0
	}
	for _, children := range d.edges {
		for _, child := range children {
			inDegree[child]++
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, child := range d.edges[node] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	return order, nil
}
