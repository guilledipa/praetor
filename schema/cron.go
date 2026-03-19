package schema

import "github.com/go-playground/validator/v10"

type Cron struct {
	TypeMeta   `yaml:",inline" json:",inline"`
	ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec       CronSpec `yaml:"spec" json:"spec"`
}

type CronSpec struct {
	User       string `yaml:"user" json:"user" validate:"required"`
	Command    string `yaml:"command" json:"command" validate:"required"`
	Ensure     string `yaml:"ensure" json:"ensure" validate:"oneof=present absent"`
	Minute     string `yaml:"minute,omitempty" json:"minute,omitempty"`
	Hour       string `yaml:"hour,omitempty" json:"hour,omitempty"`
	DayOfMonth string `yaml:"day_of_month,omitempty" json:"day_of_month,omitempty"`
	Month      string `yaml:"month,omitempty" json:"month,omitempty"`
	DayOfWeek  string `yaml:"day_of_week,omitempty" json:"day_of_week,omitempty"`
}

func (c *Cron) Validate() error {
	validate := validator.New()
	if c.Spec.Ensure == "" {
		c.Spec.Ensure = "present"
	}
	if c.Spec.User == "" {
		c.Spec.User = "root"
	}
	if c.Spec.Minute == "" {
		c.Spec.Minute = "*"
	}
	if c.Spec.Hour == "" {
		c.Spec.Hour = "*"
	}
	if c.Spec.DayOfMonth == "" {
		c.Spec.DayOfMonth = "*"
	}
	if c.Spec.Month == "" {
		c.Spec.Month = "*"
	}
	if c.Spec.DayOfWeek == "" {
		c.Spec.DayOfWeek = "*"
	}
	return validate.Struct(c)
}
