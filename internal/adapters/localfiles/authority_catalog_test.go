package localfiles

import (
	"testing"

	"github.com/clcsolutions/nats-tui/internal/adapters/jwtauthority"
	domain "github.com/clcsolutions/nats-tui/internal/domain/authority"
	"github.com/nats-io/nkeys"
)

func TestAuthorityCatalogListsManagedAccountsAndUsersWithoutSeeds(t *testing.T) {
	t.Parallel()
	operatorPair, err := nkeys.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	operatorSeed, _ := operatorPair.Seed()
	signer, err := jwtauthority.NewDefault(operatorSeed)
	if err != nil {
		t.Fatal(err)
	}
	accountDefinition, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{Connections: 10})
	accountBundle, err := signer.CreateAccount(accountDefinition)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewAuthorityStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StoreAccount(t.Context(), "orders", accountBundle); err != nil {
		t.Fatal(err)
	}
	userDefinition, _ := domain.NewUserDefinition("worker", accountBundle.PublicKey, domain.Permissions{PublishAllow: []string{"orders.*"}, SubscriptionLimit: 5})
	userBundle, err := signer.CreateUser(accountBundle.Seed, userDefinition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.StoreUser(t.Context(), "worker", userBundle); err != nil {
		t.Fatal(err)
	}
	catalog, err := store.OperatorCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	accounts, users := catalog.Accounts(), catalog.Users()
	if !catalog.Capabilities().LocalAuthority || len(accounts) != 1 || accounts[0].Limits.Connections != 10 || len(users) != 1 || users[0].Account != accountBundle.PublicKey || users[0].PublishAllow[0] != "orders.*" || users[0].SubscriptionLimit != 5 {
		t.Fatalf("capabilities=%#v accounts=%#v users=%#v", catalog.Capabilities(), accounts, users)
	}
	if users[0].JWT == "" || accounts[0].JWT == "" {
		t.Fatal("claims were not retained")
	}
	seed, err := store.AccountSeed(t.Context(), accountBundle.PublicKey)
	if err != nil || string(seed) != string(accountBundle.Seed) {
		t.Fatalf("seed lookup error=%v", err)
	}
}
