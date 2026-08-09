package natsmonitor

import (
	"encoding/json"
	"testing"
)

func TestMapAccountIncludesLimitsSharingAndSigningKeys(t *testing.T) {
	t.Parallel()
	claim := json.RawMessage(`{"nats":{"signing_keys":["SA"],"limits":{"conn":10,"subs":20,"disk_storage":30},"imports":[{"token":"ACT"}]}}`)
	account := mapAccount("A", accountInfoWire{NameTag: "orders", JWT: "token", Claim: claim, Imports: []sharingWire{{Subject: "events", Account: "B"}}, Exports: []sharingWire{{Subject: "orders.*"}}})
	if account.Name != "orders" || account.Limits.Connections != 10 || account.Limits.Subscriptions != 20 || account.Limits.Storage != 30 || len(account.SigningKeys) != 1 || len(account.Imports) != 1 || !account.Exports[0].Public || len(account.ActivationTokens) != 1 {
		t.Fatalf("account=%#v", account)
	}
}

func TestClaimUpdateErrorPreservesServerRejection(t *testing.T) {
	t.Parallel()
	if err := claimUpdateError([]byte(`{"error":{"code":400,"description":"resolver is read only"}}`)); err == nil || err.Error() != "server rejected claim (400): resolver is read only" {
		t.Fatalf("error=%v", err)
	}
	if err := claimUpdateError([]byte(`{"data":{"code":200}}`)); err != nil {
		t.Fatal(err)
	}
}
