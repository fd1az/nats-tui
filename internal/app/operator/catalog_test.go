package operator

import (
	"testing"
)

func TestCatalogAggregatesAccountUsageAcrossServers(t *testing.T) {
	t.Parallel()
	catalog := NewCatalog(Capabilities{SystemAccount: true, OperatorMode: true}, []Account{{ID: "A", Name: "orders", Connections: 2, Subscriptions: 3}, {ID: "A", Name: "orders", Connections: 4, Subscriptions: 5}, {ID: "B", Name: "billing", Connections: 1}})
	accounts := catalog.Accounts()
	if len(accounts) != 2 || accounts[1].ID != "A" || accounts[1].Connections != 6 || accounts[1].Subscriptions != 8 {
		t.Fatalf("accounts=%#v", accounts)
	}
}

func TestMergeCatalogAddsManagedUsersAndCapabilities(t *testing.T) {
	t.Parallel()
	network := NewCatalog(Capabilities{SystemAccount: true}, []Account{{ID: "A", Name: "orders", Connections: 2}})
	managed := NewCatalog(Capabilities{LocalAuthority: true, ClaimsWrite: true}, []Account{{ID: "A", Name: "orders", JWT: "jwt", Managed: true}}, []User{{PublicKey: "U", Name: "worker", Account: "A", PublishAllow: []string{"orders.*"}}})
	result := MergeCatalog(network, managed)
	if !result.Capabilities().SystemAccount || !result.Capabilities().LocalAuthority || !result.Capabilities().ClaimsWrite || result.Accounts()[0].Connections != 2 || result.Accounts()[0].JWT != "jwt" || !result.Accounts()[0].Managed || len(result.Users()) != 1 {
		t.Fatalf("catalog=%#v accounts=%#v users=%#v", result.Capabilities(), result.Accounts(), result.Users())
	}
	users := result.Users()
	users[0].PublishAllow[0] = "mutated"
	if result.Users()[0].PublishAllow[0] != "orders.*" {
		t.Fatal("managed user permissions leaked")
	}
}
