package schema

import "github.com/go-playground/validator/v10"

type Group struct {
	TypeMeta   `yaml:",inline" json:",inline"`
	ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec       GroupSpec `yaml:"spec" json:"spec"`
}

type GroupSpec struct {
	Ensure string `yaml:"ensure" json:"ensure" validate:"oneof=present absent"`
	Gid    *int   `yaml:"gid,omitempty" json:"gid,omitempty"`
}

func (g *Group) Validate() error {
	validate := validator.New()
	if g.Spec.Ensure == "" {
		g.Spec.Ensure = "present"
	}
	return validate.Struct(g)
}
