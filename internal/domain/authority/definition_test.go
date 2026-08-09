package authority

import "testing"

func TestAccountDefinitionRejectsInvalidLimits(t *testing.T) {
	t.Parallel()
	if _, err := NewAccountDefinition("orders", AccountLimits{Connections: -2}); err == nil {
		t.Fatal("limit below unlimited sentinel accepted")
	}
	definition, err := NewAccountDefinition("orders", AccountLimits{Connections: 10, Subscriptions: 20})
	if err != nil {
		t.Fatal(err)
	}
	if definition.Name() != "orders" || definition.Limits().Connections != 10 {
		t.Fatalf("definition=%#v", definition)
	}
}

func TestUserDefinitionOwnsPermissionSlices(t *testing.T) {
	t.Parallel()
	allow := []string{"orders.*"}
	definition, err := NewUserDefinition("worker", "AACCOUNT", Permissions{PublishAllow: allow, SubscribeAllow: []string{"events.>"}})
	if err != nil {
		t.Fatal(err)
	}
	allow[0] = "mutated"
	if definition.Permissions().PublishAllow[0] != "orders.*" {
		t.Fatal("definition leaked caller-owned permissions")
	}
	if _, err := NewUserDefinition("worker", "AACCOUNT", Permissions{PublishAllow: []string{"bad subject"}}); err == nil {
		t.Fatal("invalid subject accepted")
	}
}
