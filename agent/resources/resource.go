package resources

import (
	"encoding/json"
	"fmt"
)

type State map[string]interface{}

type Dependency struct {
	Kind string
	Name string
}

type Resource interface {
	Type() string
	ID() string
	Get() (State, error)
	Test(currentState State) (bool, error)
	Set() error
	Requires() []Dependency
	Before() []Dependency
}

type FactoryFunc func(spec json.RawMessage) (Resource, error)

var registry = make(map[string]FactoryFunc)

func RegisterType(kind string, factory FactoryFunc) {
	registry[kind] = factory
}

func GetRegistry() map[string]FactoryFunc {
	return registry
}

func CreateResource(kind string, spec json.RawMessage) (Resource, error) {
	factory, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported resource kind: %s", kind)
	}
	return factory(spec)
}
