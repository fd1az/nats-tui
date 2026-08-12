package jwtauthority

import (
	"testing"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/authority"
	jwt "github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func TestSignerCreatesAccountAndRestrictedUserCredentials(t *testing.T) {
	t.Parallel()
	operator, err := nkeys.CreateOperator()
	if err != nil {
		t.Fatal(err)
	}
	operatorSeed, _ := operator.Seed()
	signer, err := New(operatorSeed, nkeyFactory{})
	if err != nil {
		t.Fatal(err)
	}
	accountDefinition, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{Connections: 10, Subscriptions: 20, Storage: 1024})
	account, err := signer.CreateAccount(accountDefinition)
	if err != nil {
		t.Fatal(err)
	}
	accountClaim, err := jwt.DecodeAccountClaims(account.JWT)
	if err != nil {
		t.Fatal(err)
	}
	if accountClaim.Name != "orders" || accountClaim.Limits.Conn != 10 || accountClaim.Limits.Subs != 20 || accountClaim.Limits.DiskStorage != 1024 {
		t.Fatalf("claim=%#v", accountClaim)
	}
	userDefinition, _ := domain.NewUserDefinition("worker", account.PublicKey, domain.Permissions{PublishAllow: []string{"orders.*"}, SubscribeDeny: []string{"admin.>"}, ResponseMax: 2, ResponseTTL: time.Second})
	user, err := signer.CreateUser(account.Seed, userDefinition)
	if err != nil {
		t.Fatal(err)
	}
	userClaim, err := jwt.DecodeUserClaims(user.JWT)
	if err != nil {
		t.Fatal(err)
	}
	if userClaim.Name != "worker" || !userClaim.Permissions.Pub.Allow.Contains("orders.*") || userClaim.Permissions.Resp == nil || userClaim.Permissions.Resp.MaxMsgs != 2 {
		t.Fatalf("claim=%#v", userClaim)
	}
	if len(user.Credentials) == 0 {
		t.Fatal("credentials are empty")
	}
}

func TestSignerUpdatesRevokesAndAddsScopedSigningKey(t *testing.T) {
	t.Parallel()
	operatorPair, _ := nkeys.CreateOperator()
	operatorSeed, _ := operatorPair.Seed()
	signer, err := New(operatorSeed, nkeyFactory{})
	if err != nil {
		t.Fatal(err)
	}
	initial, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{Connections: 10, Streams: 2})
	account, err := signer.CreateAccount(initial)
	if err != nil {
		t.Fatal(err)
	}
	updatedDefinition, _ := domain.NewAccountDefinition("orders-prod", domain.AccountLimits{Connections: 20, Streams: 4})
	updated, err := signer.UpdateAccount(account.JWT, updatedDefinition.WithDescription("production"))
	if err != nil {
		t.Fatal(err)
	}
	updatedClaim, _ := jwt.DecodeAccountClaims(updated.JWT)
	if updatedClaim.Subject != account.PublicKey || updatedClaim.Name != "orders-prod" || updatedClaim.Limits.Conn != 20 {
		t.Fatalf("updated claim=%#v", updatedClaim)
	}

	userDefinition, _ := domain.NewUserDefinition("worker", account.PublicKey, domain.Permissions{PublishAllow: []string{"orders.*"}})
	user, err := signer.CreateUser(account.Seed, userDefinition)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := signer.RevokeUser(updated.JWT, user.PublicKey, time.Unix(42, 0))
	if err != nil {
		t.Fatal(err)
	}
	revokedClaim, _ := jwt.DecodeAccountClaims(revoked.JWT)
	if revokedClaim.Revocations[user.PublicKey] != 42 {
		t.Fatalf("revocations=%#v", revokedClaim.Revocations)
	}

	scope, _ := domain.NewSigningKeyDefinition("reader", "read orders", true, domain.Permissions{SubscribeAllow: []string{"orders.*"}, SubscriptionLimit: -1})
	withKey, key, err := signer.AddSigningKey(revoked.JWT, scope)
	if err != nil {
		t.Fatal(err)
	}
	claim, _ := jwt.DecodeAccountClaims(withKey.JWT)
	rawScope, ok := claim.SigningKeys[key.PublicKey]
	userScope, scoped := rawScope.(*jwt.UserScope)
	if !ok || !scoped || userScope.Role != "reader" || !userScope.Template.Permissions.Sub.Allow.Contains("orders.*") || len(key.Seed) == 0 {
		t.Fatalf("scope=%#v key=%#v", rawScope, key)
	}

	deleteToken, err := signer.DeleteAccountRequest(account.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	deleteClaim, err := jwt.DecodeGeneric(deleteToken)
	if err != nil {
		t.Fatal(err)
	}
	accounts, ok := deleteClaim.Data["accounts"].([]interface{})
	if !ok || len(accounts) != 1 || accounts[0] != account.PublicKey {
		t.Fatalf("delete payload=%#v", deleteClaim.Data)
	}
}

func TestSignerUpdatesUserWithoutRotatingPublicKey(t *testing.T) {
	t.Parallel()
	operatorPair, _ := nkeys.CreateOperator()
	operatorSeed, _ := operatorPair.Seed()
	signer, _ := New(operatorSeed, nkeyFactory{})
	accountDefinition, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{})
	account, _ := signer.CreateAccount(accountDefinition)
	definition, _ := domain.NewUserDefinition("worker", account.PublicKey, domain.Permissions{SubscriptionLimit: -1})
	user, _ := signer.CreateUser(account.Seed, definition)
	pair, err := nkeys.ParseDecoratedNKey(user.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := pair.Seed()
	changed, _ := domain.NewUserDefinition("worker-v2", account.PublicKey, domain.Permissions{PublishAllow: []string{"orders.created"}, SubscriptionLimit: -1})
	updated, err := signer.UpdateUser(account.Seed, seed, user.JWT, changed)
	if err != nil {
		t.Fatal(err)
	}
	claim, _ := jwt.DecodeUserClaims(updated.JWT)
	if updated.PublicKey != user.PublicKey || claim.Name != "worker-v2" || !claim.Permissions.Pub.Allow.Contains("orders.created") {
		t.Fatalf("updated=%#v claim=%#v", updated, claim)
	}
}
