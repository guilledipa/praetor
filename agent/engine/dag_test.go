package engine

import (
	"reflect"
	"testing"
	"github.com/guilledipa/praetor/agent/resources"
)

// mockResource implements the resources.Resource interface for unit testing.
type mockResource struct {
	id       string
	kind     string
	requires []resources.Dependency
	before   []resources.Dependency
}

func (m mockResource) Get() (resources.State, error) { return nil, nil }
func (m mockResource) Test(currentState resources.State) (bool, error) { return true, nil }
func (m mockResource) Set() error { return nil }
func (m mockResource) Type() string { return m.kind }
func (m mockResource) ID() string { return m.id }
func (m mockResource) Requires() []resources.Dependency { return m.requires }
func (m mockResource) Before() []resources.Dependency { return m.before }

func TestBuildDAG(t *testing.T) {
	tests := []struct {
		name        string
		input       []resources.Resource
		expectError bool
		// we verify order implicitly by checking dependencies
		expectedOrders []string
	}{
		{
			name: "Linear Chain (A requires B, B requires C)",
			input: []resources.Resource{
				mockResource{id: "A", kind: "Pkg", requires: []resources.Dependency{{Kind: "Pkg", Name: "B"}}},
				mockResource{id: "B", kind: "Pkg", requires: []resources.Dependency{{Kind: "Pkg", Name: "C"}}},
				mockResource{id: "C", kind: "Pkg"},
			},
			expectError:    false,
			expectedOrders: []string{"Pkg[C]", "Pkg[B]", "Pkg[A]"},
		},
		{
			name: "Independent Nodes (No dependencies)",
			input: []resources.Resource{
				mockResource{id: "A", kind: "Svc"},
				mockResource{id: "B", kind: "Svc"},
			},
			expectError: false,
		},
		{
			name: "Cyclic Dependency (A req B, B req A)",
			input: []resources.Resource{
				mockResource{id: "A", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "B"}}},
				mockResource{id: "B", kind: "File", requires: []resources.Dependency{{Kind: "File", Name: "A"}}},
			},
			expectError: true,
		},
		{
			name: "Missing Dependency",
			input: []resources.Resource{
				mockResource{id: "A", kind: "Exec", requires: []resources.Dependency{{Kind: "File", Name: "NonExistent"}}},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sorted, err := buildDAG(tc.input)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, but got none", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.name, err)
			}

			if tc.expectedOrders != nil {
				var got []string
				for _, res := range sorted {
					got = append(got, buildNodeName(res.Type(), res.ID()))
				}
				if !reflect.DeepEqual(got, tc.expectedOrders) {
					t.Errorf("expected order %v, got %v", tc.expectedOrders, got)
				}
			}
		})
	}
}
