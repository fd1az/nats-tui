package natsmonitor

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

func TestNKeyOptionsReadSeedThroughInjectedBoundaryAndSign(t *testing.T) {
	pair, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := pair.Seed()
	if err != nil {
		t.Fatal(err)
	}
	reader := func(path string) ([]byte, error) {
		if path != "user.seed" {
			return nil, errors.New("unexpected path")
		}
		return append([]byte(nil), seed...), nil
	}
	options, err := authenticationOptions(config.Settings{NKeySeed: "user.seed"}, reader)
	if err != nil || len(options) != 1 {
		t.Fatalf("options=%d err=%v", len(options), err)
	}
	var values nats.Options
	if err := options[0](&values); err != nil {
		t.Fatal(err)
	}
	if values.Nkey == "" || values.SignatureCB == nil {
		t.Fatalf("values=%+v", values)
	}
	if _, err := values.SignatureCB([]byte("nonce")); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialReferencesSupportEnvironmentAndEmbeddedData(t *testing.T) {
	t.Setenv("NATS_TUI_TEST_SEED", "secret-value")
	value, err := readCredential("env://NATS_TUI_TEST_SEED")
	if err != nil || string(value) != "secret-value" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	embedded := "data:;base64," + base64.StdEncoding.EncodeToString([]byte("credential"))
	value, err = readCredential(embedded)
	if err != nil || string(value) != "credential" {
		t.Fatalf("value=%q err=%v", value, err)
	}
	if _, err := readCredential("op://vault/item"); err == nil {
		t.Fatal("unsupported resolver accepted")
	}
}

func TestTokenAndPasswordCanUseInjectedSecretReferences(t *testing.T) {
	reader := func(reference string) ([]byte, error) {
		if reference != "env://NATS_TOKEN" {
			return nil, errors.New("unexpected reference")
		}
		return []byte("resolved"), nil
	}
	options, err := authenticationOptions(config.Settings{Token: "env://NATS_TOKEN"}, reader)
	if err != nil {
		t.Fatal(err)
	}
	var values nats.Options
	if err := options[0](&values); err != nil {
		t.Fatal(err)
	}
	if values.Token != "resolved" {
		t.Fatalf("token=%q", values.Token)
	}
}

func TestAuthenticationOptionsRejectMixedAndIncompleteMethods(t *testing.T) {
	reader := func(string) ([]byte, error) { return nil, errors.New("unused") }
	if _, err := authenticationOptions(config.Settings{Token: "a", Credentials: "b"}, reader); err == nil {
		t.Fatal("mixed authentication accepted")
	}
	if _, err := authenticationOptions(config.Settings{UserJWT: "user.jwt"}, reader); err == nil {
		t.Fatal("incomplete JWT accepted")
	}
}

func TestTransportSecurityRejectsInvalidCAAndIncompleteClientPair(t *testing.T) {
	reader := func(string) ([]byte, error) { return []byte("not a certificate"), nil }
	if _, err := transportSecurityOption(config.Settings{TLSCA: "ca.pem"}, reader); err == nil {
		t.Fatal("invalid CA accepted")
	}
	if _, err := transportSecurityOption(config.Settings{TLSCert: "client.pem"}, reader); err == nil {
		t.Fatal("incomplete pair accepted")
	}
}
