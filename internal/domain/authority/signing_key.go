package authority

import (
	"errors"
	"strings"
)

type SigningKeyDefinition struct {
	role, description string
	scoped            bool
	template          Permissions
}

func NewSigningKeyDefinition(role, description string, scoped bool, template Permissions) (SigningKeyDefinition, error) {
	role = strings.TrimSpace(role)
	if scoped && role == "" {
		return SigningKeyDefinition{}, errors.New("a scoped signing key requires a role")
	}
	if _, err := NewUserDefinition("template", "account", template); err != nil {
		return SigningKeyDefinition{}, err
	}
	return SigningKeyDefinition{role: role, description: strings.TrimSpace(description), scoped: scoped, template: clonePermissions(template)}, nil
}

func (d SigningKeyDefinition) Role() string          { return d.role }
func (d SigningKeyDefinition) Description() string   { return d.description }
func (d SigningKeyDefinition) Scoped() bool          { return d.scoped }
func (d SigningKeyDefinition) Template() Permissions { return clonePermissions(d.template) }
func (d SigningKeyDefinition) IsZero() bool          { return !d.scoped && d.role == "" && d.description == "" }
