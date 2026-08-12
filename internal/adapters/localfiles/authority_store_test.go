package localfiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fd1az/nats-tui/internal/adapters/jwtauthority"
	app "github.com/fd1az/nats-tui/internal/app/operator"
	domain "github.com/fd1az/nats-tui/internal/domain/authority"
	"github.com/nats-io/nkeys"
)

func TestAuthorityStoreWritesPrivateFilesWithoutOverwrite(t *testing.T) {
	t.Parallel()
	store, err := NewAuthorityStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := store.StoreAccount(t.Context(), "orders / prod", app.AccountBundle{JWT: "token", Seed: []byte("seed")})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{artifacts.SeedPath, artifacts.JWTPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode for %s = %o", path, info.Mode().Perm())
		}
	}
	if filepath.Base(artifacts.SeedPath) != "orders---prod.account.seed" {
		t.Fatalf("path=%s", artifacts.SeedPath)
	}
	if _, err := store.StoreAccount(t.Context(), "orders / prod", app.AccountBundle{JWT: "new", Seed: []byte("new")}); err == nil {
		t.Fatal("existing credentials were overwritten")
	}
}

func TestAuthorityStoreSupportsAtomicMutationAndRecoverableRemoval(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	store, err := NewAuthorityStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	operatorPair, _ := nkeys.CreateOperator()
	operatorSeed, _ := operatorPair.Seed()
	signer, _ := jwtauthority.NewDefault(operatorSeed)
	accountDefinition, _ := domain.NewAccountDefinition("orders", domain.AccountLimits{Connections: 10})
	account, _ := signer.CreateAccount(accountDefinition)
	accountFiles, err := store.StoreAccount(t.Context(), "orders", account)
	if err != nil {
		t.Fatal(err)
	}
	userDefinition, _ := domain.NewUserDefinition("worker", account.PublicKey, domain.Permissions{SubscriptionLimit: -1})
	user, _ := signer.CreateUser(account.Seed, userDefinition)
	userFiles, err := store.StoreUser(t.Context(), "worker", user)
	if err != nil {
		t.Fatal(err)
	}
	if seed, err := store.AccountSeed(t.Context(), account.PublicKey); err != nil || len(seed) == 0 {
		t.Fatalf("account seed err=%v", err)
	}
	if seed, err := store.UserSeed(t.Context(), user.PublicKey); err != nil || len(seed) == 0 {
		t.Fatalf("user seed err=%v", err)
	}

	changed, _ := domain.NewAccountDefinition("orders-v2", domain.AccountLimits{Connections: 20})
	updated, _ := signer.UpdateAccount(account.JWT, changed)
	claimPath, err := store.ReplaceAccountClaim(t.Context(), account.PublicKey, updated.JWT)
	if err != nil || claimPath != accountFiles.JWTPath {
		t.Fatalf("claim path=%q err=%v", claimPath, err)
	}
	info, _ := os.Stat(claimPath)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("replacement mode=%o", info.Mode().Perm())
	}

	updatedUserDefinition, _ := domain.NewUserDefinition("worker-v2", account.PublicKey, domain.Permissions{PublishAllow: []string{"orders.*"}, SubscriptionLimit: -1})
	userSeed, _ := store.UserSeed(t.Context(), user.PublicKey)
	updatedUser, _ := signer.UpdateUser(account.Seed, userSeed, user.JWT, updatedUserDefinition)
	updatedFiles, err := store.ReplaceUser(t.Context(), user.PublicKey, updatedUser)
	if err != nil || updatedFiles.CredentialsPath != userFiles.CredentialsPath {
		t.Fatalf("user files=%#v err=%v", updatedFiles, err)
	}

	keyDefinition, _ := domain.NewSigningKeyDefinition("", "automation", false, domain.Permissions{SubscriptionLimit: -1})
	_, signing, _ := signer.AddSigningKey(updated.JWT, keyDefinition)
	keyFiles, err := store.StoreSigningKey(t.Context(), account.PublicKey, signing)
	if err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(keyFiles.SeedPath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("signing key mode/info=%v err=%v", info, statErr)
	}

	trashUser, err := store.RemoveUser(t.Context(), user.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(trashUser); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(userFiles.CredentialsPath); !os.IsNotExist(err) {
		t.Fatalf("credentials still active: %v", err)
	}
	trashAccount, err := store.RemoveAccount(t.Context(), account.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(trashAccount) != ".trash" {
		t.Fatalf("trash=%s", trashAccount)
	}
	if _, err := os.Stat(filepath.Join(trashAccount, filepath.Base(accountFiles.JWTPath)+".deleted")); err != nil {
		t.Fatal(err)
	}
}
