package schema

// TypeMeta is shared by all top-level objects.
// "+k8s:deepcopy-gen=true"
type TypeMeta struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion" validate:"required"`
	Kind       string `yaml:"kind" json:"kind" validate:"required"`
}

// Dependency represents a K8s-native object reference edge.
type Dependency struct {
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
	Name string `yaml:"name" json:"name" validate:"required"`
}

// ObjectMeta is shared by all top-level objects.
type ObjectMeta struct {
	Name        string            `yaml:"name,omitempty" json:"name,omitempty" validate:"required"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
	Requires    []Dependency      `yaml:"requires,omitempty" json:"requires,omitempty"`
	Before      []Dependency      `yaml:"before,omitempty" json:"before,omitempty"`
}
