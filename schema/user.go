package schema

import "github.com/go-playground/validator/v10"

type User struct {
	TypeMeta   `yaml:",inline" json:",inline"`
	ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec       UserSpec `yaml:"spec" json:"spec"`
}

type UserSpec struct {
	Ensure string   `yaml:"ensure" json:"ensure" validate:"oneof=present absent"`
	Uid    *int     `yaml:"uid,omitempty" json:"uid,omitempty"`
	Gid    *int     `yaml:"gid,omitempty" json:"gid,omitempty"` // Primary group ID
	Groups []string `yaml:"groups,omitempty" json:"groups,omitempty"` // Secondary groups
	Shell  string   `yaml:"shell,omitempty" json:"shell,omitempty"`
	Home   string   `yaml:"home,omitempty" json:"home,omitempty"`
}

func (u *User) Validate() error {
	validate := validator.New()
	if u.Spec.Ensure == "" {
		u.Spec.Ensure = "present"
	}
	return validate.Struct(u)
}
