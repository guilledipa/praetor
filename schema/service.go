package schema

import (
	"github.com/go-playground/validator/v10"
)

// ServiceSpec defines the desired state of a service.
// +k8s:deepcopy-gen=true
type ServiceSpec struct {
	Name   string `yaml:"name" json:"name" validate:"required"`
	Ensure string `yaml:"ensure" json:"ensure" validate:"required,oneof=running stopped"`
	Enable bool   `yaml:"enable" json:"enable"`
}

// Service represents a service resource.
// +k8s:deepcopy-gen=true
type Service struct {
	TypeMeta   `yaml:",inline"`
	ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty" validate:"required"`
	Spec       ServiceSpec `yaml:"spec" json:"spec" validate:"required"`
}

// Validate checks the Service struct for errors.
func (s *Service) Validate() error {
	validate := validator.New()
	return validate.Struct(s)
}
