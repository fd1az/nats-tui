package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	domain "github.com/clcsolutions/nats-tui/internal/domain/authority"
)

func TestOperatorViewShowsCapabilitiesAndAccountDetails(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{SystemAccount: true, OperatorMode: true, ClaimsRead: true}, []app.Account{{ID: "AORDERS", Name: "orders", Connections: 3, Subscriptions: 8, Limits: app.Limits{Connections: 10}, Exports: []app.Sharing{{Subject: "orders.*", Public: true}}}})
	view := model.operatorView(120, 30)
	for _, want := range []string{"Accounts & Users", "OPERATOR MODE", "orders", "AORDERS"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestOperatorAccountFormIsCapabilityGatedAndCentered(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{SystemAccount: true, ClaimsWrite: true, LocalAuthority: true}, nil)
	updated, _, handled := model.updateOperatorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !handled {
		t.Fatal("new account key was not handled")
	}
	opened := updated.(Model)
	if opened.operator.form == nil {
		t.Fatal("account form was not opened")
	}
	view := opened.operatorView(120, 34)
	for _, want := range []string{"Create operator account", "WHAT TO PUT HERE", "ctrl+s create"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form missing %q:\n%s", want, view)
		}
	}
}

func TestOperatorUsersViewExplainsManagedScopeAndPermissions(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{LocalAuthority: true}, nil, []app.User{{PublicKey: "UWORKER", Name: "worker", Account: "AORDERS", PublishAllow: []string{"orders.*"}}})
	model.operator.users = true
	view := model.operatorView(120, 30)
	for _, want := range []string{"MANAGED USERS", "worker", "UWORKER", "AORDERS"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestOperatorUserFormUsesOnlyManagedAccounts(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{LocalAuthority: true}, []app.Account{{ID: "AREMOTE", Name: "remote"}, {ID: "ALOCAL", Name: "orders", Managed: true}})
	model.operator.users = true
	updated, _, handled := model.updateOperatorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !handled {
		t.Fatal("new user key was not handled")
	}
	opened := updated.(Model)
	if opened.operator.userForm == nil || len(opened.operator.userForm.accounts) != 1 || opened.operator.userForm.accounts[0].ID != "ALOCAL" {
		t.Fatalf("form=%#v", opened.operator.userForm)
	}
	opened.operator.userForm.name = "worker"
	opened.operator.userForm.publishAllow = "orders.*, audit.created"
	definition, err := opened.operator.userForm.definition()
	if err != nil {
		t.Fatal(err)
	}
	if definition.Account() != "ALOCAL" || len(definition.Permissions().PublishAllow) != 2 {
		t.Fatalf("definition=%#v", definition)
	}
	view := opened.operatorView(120, 34)
	for _, want := range []string{"Create managed user", "orders", "no seed enters the UI"} {
		if !strings.Contains(view, want) {
			t.Fatalf("form missing %q:\n%s", want, view)
		}
	}
}

func TestOperatorAccountEditPreservesUnexposedLimits(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	account := app.Account{ID: "ALOCAL", Name: "orders", Managed: true, Limits: app.Limits{Connections: 10, LeafNodes: 3, Payload: 1024, Imports: 7, Exports: 8}}
	model.operator.catalog = app.NewCatalog(app.Capabilities{ClaimsWrite: true, LocalAuthority: true}, []app.Account{account})
	updated, _, handled := model.updateOperatorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !handled {
		t.Fatal("edit key was not handled")
	}
	opened := updated.(Model)
	if opened.operator.form == nil || opened.operator.form.accountID != "ALOCAL" {
		t.Fatalf("form=%#v", opened.operator.form)
	}
	definition, err := opened.operator.form.definition()
	if err != nil {
		t.Fatal(err)
	}
	if definition.Limits().LeafNodes != 3 || definition.Limits().Payload != 1024 || definition.Limits().Imports != 7 || definition.Limits().Exports != 8 {
		t.Fatalf("limits=%#v", definition.Limits())
	}
	if !strings.Contains(opened.operatorView(120, 34), "Edit operator account") {
		t.Fatal("edit form title is missing")
	}
}

func TestOperatorDestructiveActionsRequireCenteredConfirmation(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{ClaimsWrite: true, LocalAuthority: true}, []app.Account{{ID: "ALOCAL", Name: "orders", Managed: true}})
	updated, _, handled := model.updateOperatorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !handled {
		t.Fatal("delete key was not handled")
	}
	opened := updated.(Model)
	if opened.operator.confirm == nil || opened.operator.confirm.resourceID != "ALOCAL" {
		t.Fatalf("confirmation=%#v", opened.operator.confirm)
	}
	view := opened.operatorView(120, 34)
	for _, want := range []string{"Delete account", "recoverable", "confirm"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestOperatorScopedSigningKeyFormValidatesRole(t *testing.T) {
	t.Parallel()
	model := dashboardModel(t, 120, 34)
	model.operator.catalog = app.NewCatalog(app.Capabilities{ClaimsWrite: true, LocalAuthority: true}, []app.Account{{ID: "ALOCAL", Name: "orders", Managed: true}})
	updated, _, _ := model.updateOperatorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	opened := updated.(Model)
	if opened.operator.signingForm == nil {
		t.Fatal("signing key form was not opened")
	}
	opened.operator.signingForm.scoped = true
	definition, err := domain.NewSigningKeyDefinition(opened.operator.signingForm.role, "", true, domain.Permissions{SubscriptionLimit: -1})
	if err == nil || !definition.IsZero() {
		t.Fatalf("definition=%#v err=%v", definition, err)
	}
	view := opened.operatorView(120, 34)
	for _, want := range []string{"Add account signing key", "never rendered", "ctrl+s create"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
