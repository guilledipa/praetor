package facts

import (
	"log/slog"
)

// Facter represents a source of facts.
type Facter interface {
	Name() string
	GetFacts() (map[string]any, error)
}

var facterRegistry = make(map[string]Facter)

// RegisterFacter adds a facter to the registry.
func RegisterFacter(facter Facter) {
	if _, exists := facterRegistry[facter.Name()]; exists {
		slog.Warn("Facter with name already exists, overwriting", "name", facter.Name())
	}
	facterRegistry[facter.Name()] = facter
	slog.Info("Registered facter", "name", facter.Name())
}

// Collect gathers facts from all registered facters.
func Collect() map[string]any {
	allFacts := make(map[string]any)
	for name, facter := range facterRegistry {
		facts, err := facter.GetFacts()
		if err != nil {
			slog.Error("Error getting facts from facter", "name", name, "error", err)
			continue
		}
		for k, v := range facts {
			if _, exists := allFacts[k]; exists {
				slog.Warn("Fact key conflict, fact will be overwritten", "key", k, "facter", name)
			}
			allFacts[k] = v
		}
	}
	return allFacts
}
