package schema

import (
	"github.com/go-playground/validator/v10"
)

// PackageSpec defines the desired state of a package.
// +k8s:deepcopy-gen=true
type PackageSpec struct {
	Name   string `yaml:"name" json:"name" validate:"required"`
	Ensure string `yaml:"ensure" json:"ensure" validate:"required,oneof=present absent latest"`
}

// Package represents a package resource.
// +k8s:deepcopy-gen=true
type Package struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty" validate:"required"`
	Spec       PackageSpec `yaml:"spec" json:"spec" validate:"required"`
}

// Validate checks the Package struct for errors.
func (p *Package) Validate() error {
	validate := validator.New()
	return validate.Struct(p)
}
