package authority

import "testing"

func TestScopedSigningKeyRequiresRoleAndOwnsTemplate(t *testing.T) {
	t.Parallel()
	if _, err := NewSigningKeyDefinition("", "", true, Permissions{}); err == nil {
		t.Fatal("scoped key without role accepted")
	}
	allow := []string{"orders.*"}
	definition, err := NewSigningKeyDefinition("worker", "orders workers", true, Permissions{PublishAllow: allow})
	if err != nil {
		t.Fatal(err)
	}
	allow[0] = "mutated"
	if !definition.Scoped() || definition.Template().PublishAllow[0] != "orders.*" {
		t.Fatalf("definition=%#v", definition)
	}
}
