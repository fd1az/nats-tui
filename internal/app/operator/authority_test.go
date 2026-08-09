package operator

import (
	"context"
	"testing"

	domain "github.com/clcsolutions/nats-tui/internal/domain/authority"
)

type stubAuthority struct{ account AccountBundle }

func (s stubAuthority) CreateAccount(domain.AccountDefinition) (AccountBundle, error) {
	return s.account, nil
}
func (stubAuthority) CreateUser([]byte, domain.UserDefinition) (UserBundle, error) {
	return UserBundle{}, nil
}

type recordingClaimPublisher struct{ jwt string }

func (p *recordingClaimPublisher) UpdateAccountClaim(_ context.Context, token string) error {
	p.jwt = token
	return nil
}

type recordingArtifactStore struct{ seed []byte }

func (s *recordingArtifactStore) StoreAccount(_ context.Context, _ string, bundle AccountBundle) (AccountArtifacts, error) {
	s.seed = append([]byte(nil), bundle.Seed...)
	return AccountArtifacts{SeedPath: "orders.seed", JWTPath: "orders.jwt"}, nil
}
func (*recordingArtifactStore) StoreUser(context.Context, string, UserBundle) (UserArtifacts, error) {
	return UserArtifacts{}, nil
}

func TestCreateAccountStoresSecretBeforePublishingClaim(t *testing.T) {
	t.Parallel()
	definition, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{})
	seed := []byte("secret")
	publisher, store := &recordingClaimPublisher{}, &recordingArtifactStore{}
	artifacts, err := NewCreateAccountHandler(stubAuthority{account: AccountBundle{PublicKey: "A", JWT: "jwt", Seed: seed}}, store, publisher).Handle(t.Context(), definition)
	if err != nil {
		t.Fatal(err)
	}
	if publisher.jwt != "jwt" || string(store.seed) != "secret" || artifacts.SeedPath != "orders.seed" {
		t.Fatalf("publisher=%q stored=%q artifacts=%#v", publisher.jwt, store.seed, artifacts)
	}
	store.seed[0] = 'X'
	if string(seed) != "secret" {
		t.Fatal("handler leaked signer-owned seed")
	}
}
