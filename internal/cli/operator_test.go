package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/authority"
)

type stubOperatorCatalog struct{ catalog app.Catalog }

func (s stubOperatorCatalog) ListOperatorCatalog(context.Context, config.Settings) (app.Catalog, error) {
	return s.catalog, nil
}

type recordingAccountCreator struct{ definition domain.AccountDefinition }

func (s *recordingAccountCreator) CreateOperatorAccount(_ context.Context, _ config.Settings, seed, directory string, definition domain.AccountDefinition) (app.AccountArtifacts, error) {
	s.definition = definition
	return app.AccountArtifacts{SeedPath: directory + "/orders.account.seed", JWTPath: directory + "/orders.account.jwt"}, nil
}

type recordingUserCreator struct{ definition domain.UserDefinition }

func (s *recordingUserCreator) CreateOperatorUser(_ context.Context, _, directory string, definition domain.UserDefinition) (app.UserArtifacts, error) {
	s.definition = definition
	return app.UserArtifacts{CredentialsPath: directory + "/worker.creds"}, nil
}

type recordingOperatorMutator struct {
	deletedAccount string
	signingAccount string
	signing        domain.SigningKeyDefinition
}

func (*recordingOperatorMutator) UpdateOperatorAccount(context.Context, config.Settings, string, string, string, domain.AccountDefinition) (app.AccountArtifacts, error) {
	return app.AccountArtifacts{}, nil
}
func (m *recordingOperatorMutator) DeleteOperatorAccount(_ context.Context, _ config.Settings, _, _, account string) (string, error) {
	m.deletedAccount = account
	return "private/.trash", nil
}
func (*recordingOperatorMutator) UpdateOperatorUser(context.Context, string, string, domain.UserDefinition) (app.UserArtifacts, error) {
	return app.UserArtifacts{}, nil
}
func (*recordingOperatorMutator) RevokeOperatorUser(context.Context, config.Settings, string, string, string, string) (app.AccountArtifacts, error) {
	return app.AccountArtifacts{}, nil
}
func (*recordingOperatorMutator) DeleteOperatorUser(context.Context, config.Settings, string, string, string, string) (string, error) {
	return "private/.trash/user", nil
}
func (m *recordingOperatorMutator) AddOperatorSigningKey(_ context.Context, _ config.Settings, _, _, account string, definition domain.SigningKeyDefinition) (app.SigningKeyArtifacts, error) {
	m.signingAccount, m.signing = account, definition
	return app.SigningKeyArtifacts{SeedPath: "private/key.seed"}, nil
}

func TestAccountsPrintCapabilitiesLimitsAndSharing(t *testing.T) {
	t.Parallel()
	service := stubOperatorCatalog{catalog: app.NewCatalog(app.Capabilities{SystemAccount: true, OperatorMode: true, ClaimsRead: true}, []app.Account{{
		ID: "AORDERS", Name: "orders", Connections: 2, Subscriptions: 4,
		Limits:  app.Limits{Connections: 10, Subscriptions: 20},
		Exports: []app.Sharing{{Subject: "orders.*", Type: "stream", Public: true}},
	}}, []app.User{{PublicKey: "UWORKER", Name: "worker", Account: "AORDERS", PublishAllow: []string{"orders.*"}}})}
	output := new(bytes.Buffer)
	root := NewRoot(Dependencies{OperatorReader: service})
	root.SetOut(output)
	root.SetArgs([]string{"accounts"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OPERATOR MODE", "orders", "AORDERS", "2", "4", "10", "20", "MANAGED USER", "worker", "UWORKER"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestAccountsJSONIncludesDecodedClaims(t *testing.T) {
	t.Parallel()
	service := stubOperatorCatalog{catalog: app.NewCatalog(app.Capabilities{ClaimsRead: true}, []app.Account{{ID: "A", Name: "orders"}})}
	output := new(bytes.Buffer)
	root := NewRoot(Dependencies{OperatorReader: service})
	root.SetOut(output)
	root.SetArgs([]string{"accounts", "--json"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"capabilities"`, `"accounts"`, `"claims_read":true`, `"id":"A"`} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("JSON missing %q: %s", want, output)
		}
	}
}

func TestAccountAndUserCreateMapSecureAuthorityInputs(t *testing.T) {
	t.Parallel()
	accountCreator := &recordingAccountCreator{}
	accountOutput := new(bytes.Buffer)
	accountRoot := NewRoot(Dependencies{OperatorAccountCreator: accountCreator})
	accountRoot.SetOut(accountOutput)
	accountRoot.SetArgs([]string{"accounts", "create", "orders", "--operator-seed", "operator.seed", "--output-dir", "private", "--max-connections", "10", "--max-storage", "2048"})
	if err := accountRoot.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if accountCreator.definition.Name() != "orders" || accountCreator.definition.Limits().Connections != 10 || accountCreator.definition.Limits().Storage != 2048 {
		t.Fatalf("definition=%#v", accountCreator.definition)
	}
	if !strings.Contains(accountOutput.String(), "private/orders.account.seed") {
		t.Fatalf("output=%s", accountOutput)
	}

	userCreator := &recordingUserCreator{}
	userRoot := NewRoot(Dependencies{OperatorUserCreator: userCreator})
	userRoot.SetArgs([]string{"accounts", "user", "create", "worker", "--account", "AACCOUNT", "--account-seed", "orders.seed", "--output-dir", "private", "--pub-allow", "orders.*", "--response-max", "2", "--response-ttl", "1s"})
	if err := userRoot.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	permissions := userCreator.definition.Permissions()
	if userCreator.definition.Name() != "worker" || len(permissions.PublishAllow) != 1 || permissions.ResponseMax != 2 || permissions.ResponseTTL != time.Second {
		t.Fatalf("definition=%#v permissions=%#v", userCreator.definition, permissions)
	}
}

func TestAccountDeleteRequiresConfirmationAndSigningKeyMapsScope(t *testing.T) {
	t.Parallel()
	mutator := &recordingOperatorMutator{}
	root := NewRoot(Dependencies{OperatorMutator: mutator})
	root.SetArgs([]string{"accounts", "delete", "AACCOUNT", "--operator-seed", "operator.seed", "--output-dir", "private"})
	if err := root.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation error, got %v", err)
	}

	root = NewRoot(Dependencies{OperatorMutator: mutator})
	root.SetArgs([]string{"accounts", "delete", "AACCOUNT", "--operator-seed", "operator.seed", "--output-dir", "private", "--yes"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mutator.deletedAccount != "AACCOUNT" {
		t.Fatalf("deleted=%q", mutator.deletedAccount)
	}

	root = NewRoot(Dependencies{OperatorMutator: mutator})
	root.SetArgs([]string{"accounts", "signing-key", "add", "AACCOUNT", "--operator-seed", "operator.seed", "--output-dir", "private", "--scoped", "--role", "reader", "--sub-allow", "orders.*"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mutator.signingAccount != "AACCOUNT" || !mutator.signing.Scoped() || mutator.signing.Role() != "reader" || len(mutator.signing.Template().SubscribeAllow) != 1 {
		t.Fatalf("account=%q definition=%#v", mutator.signingAccount, mutator.signing)
	}
}
