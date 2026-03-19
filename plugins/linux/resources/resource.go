package resources

import (
	"encoding/json"
	"fmt"
	"github.com/guilledipa/praetor/schema"
	"log"
)

// Resource represents a manageable component on a node.
type Resource interface {
	// Get retrieves the current state of the resource.
	Get() (State, error)
	// Test compares the current state against the desired state.
	Test(currentState State) (bool, error)
	// Set enforces the desired state.
	Set() error
	// Type returns the resource type name.
	Type() string
	// ID returns a unique identifier for this resource instance.
	ID() string
	// Requires returns dependency targets that must run before this resource.
	Requires() []schema.Dependency
	// Before returns dependency targets that must explicitly run after this resource.
	Before() []schema.Dependency
}

// State represents the current state of a resource as a map.
type State map[string]interface{}

// Factory is a function that creates a Resource instance from a JSON spec.
type Factory func(spec json.RawMessage) (Resource, error)

var typeRegistry = make(map[string]Factory)

// RegisterType adds a resource type factory to the registry.
func RegisterType(typeName string, factory Factory) {
	if _, exists := typeRegistry[typeName]; exists {
		log.Printf("Warning: Resource type '%s' is being re-registered", typeName)
	}
	typeRegistry[typeName] = factory
	log.Printf("Registered resource type: %s", typeName)
}

// CreateResource creates a Resource instance based on the typeName.
func CreateResource(typeName string, spec json.RawMessage) (Resource, error) {
	factory, exists := typeRegistry[typeName]
	if !exists {
		return nil, fmt.Errorf("unrecognized resource type: %s", typeName)
	}
	return factory(spec)
}