package schema

import (
	"github.com/go-playground/validator/v10"
)

// ExecSpec defines the desired state of an exec resource.
// +k8s:deepcopy-gen=true
type ExecSpec struct {
	Command string `yaml:"command" json:"command" validate:"required"`
	OnlyIf  string `yaml:"onlyif,omitempty" json:"onlyif,omitempty"`
	Unless  string `yaml:"unless,omitempty" json:"unless,omitempty"`
	Creates string `yaml:"creates,omitempty" json:"creates,omitempty"`
}

// Exec represents an execution resource.
// +k8s:deepcopy-gen=true
type Exec struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty" validate:"required"`
	Spec       ExecSpec `yaml:"spec" json:"spec" validate:"required"`
}

// Validate checks the Exec struct for errors.
func (e *Exec) Validate() error {
	validate := validator.New()
	return validate.Struct(e)
}
