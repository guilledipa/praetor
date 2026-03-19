package schema

import (
	"github.com/go-playground/validator/v10"
)

// FileSpec defines the desired state of a file.
// +k8s:deepcopy-gen=true
type FileSpec struct {
	Path    string `yaml:"path" json:"path" validate:"required"`
	Content string `yaml:"content,omitempty" json:"content,omitempty"`
	Ensure  string `yaml:"ensure" json:"ensure" validate:"required,oneof=present absent"`
	Mode    string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Owner   string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Group   string `yaml:"group,omitempty" json:"group,omitempty"`
}

// File represents a file resource.
// +k8s:deepcopy-gen=true
type File struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty" validate:"required"`
	Spec       FileSpec `yaml:"spec" json:"spec" validate:"required"`
}

// Validate checks the File struct for errors.
func (f *File) Validate() error {
	validate := validator.New()
	return validate.Struct(f)
}
