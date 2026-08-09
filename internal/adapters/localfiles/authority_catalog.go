package localfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	jwt "github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

func (s AuthorityStore) OperatorCatalog(context.Context) (app.Catalog, error) {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return app.Catalog{}, fmt.Errorf("read authority directory: %w", err)
	}
	accounts, claims, err := s.readManagedAccounts(entries)
	if err != nil {
		return app.Catalog{}, err
	}
	users, err := s.readManagedUsers(entries, claims)
	if err != nil {
		return app.Catalog{}, err
	}
	return app.NewCatalog(app.Capabilities{ClaimsRead: true, LocalAuthority: true}, accounts, users), nil
}

func (s AuthorityStore) AccountSeed(_ context.Context, accountID string) ([]byte, error) {
	claimPath, _, err := s.findAccount(accountID)
	if err != nil {
		return nil, err
	}
	seed, err := os.ReadFile(strings.TrimSuffix(claimPath, ".jwt") + ".seed")
	if err != nil {
		return nil, fmt.Errorf("read managed account seed: %w", err)
	}
	return append([]byte(nil), seed...), nil
}

func (s AuthorityStore) AccountClaim(_ context.Context, accountID string) (string, error) {
	_, token, err := s.findAccount(accountID)
	return token, err
}

func (s AuthorityStore) UserSeed(_ context.Context, userID string) ([]byte, error) {
	path, _, err := s.findUser(userID)
	if err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read managed user credentials: %w", err)
	}
	pair, err := nkeys.ParseDecoratedNKey(contents)
	if err != nil {
		return nil, fmt.Errorf("parse managed user seed: %w", err)
	}
	seed, err := pair.Seed()
	if err != nil {
		return nil, fmt.Errorf("read managed user seed: %w", err)
	}
	return append([]byte(nil), seed...), nil
}

func (s AuthorityStore) UserClaim(_ context.Context, userID string) (string, error) {
	path, _, err := s.findUser(userID)
	if err != nil {
		return "", err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read managed user credentials: %w", err)
	}
	token, err := jwt.ParseDecoratedJWT(contents)
	if err != nil {
		return "", fmt.Errorf("parse managed user claim: %w", err)
	}
	return token, nil
}

func (s AuthorityStore) findAccount(accountID string) (string, string, error) {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return "", "", fmt.Errorf("read authority directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".account.jwt") {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("read managed account claim: %w", err)
		}
		token := strings.TrimSpace(string(contents))
		claim, err := jwt.DecodeAccountClaims(token)
		if err != nil {
			return "", "", fmt.Errorf("decode managed account claim: %w", err)
		}
		if claim.Subject == accountID {
			return path, token, nil
		}
	}
	return "", "", fmt.Errorf("account %s is not managed by this authority", accountID)
}

func (s AuthorityStore) findUser(userID string) (string, *jwt.UserClaims, error) {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return "", nil, fmt.Errorf("read authority directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".creds") {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", nil, fmt.Errorf("read managed user credentials: %w", err)
		}
		token, err := jwt.ParseDecoratedJWT(contents)
		if err != nil {
			return "", nil, fmt.Errorf("parse managed user credentials: %w", err)
		}
		claim, err := jwt.DecodeUserClaims(token)
		if err != nil {
			return "", nil, fmt.Errorf("decode managed user claim: %w", err)
		}
		if claim.Subject == userID {
			return path, claim, nil
		}
	}
	return "", nil, fmt.Errorf("user %s is not managed by this authority", userID)
}

func (s AuthorityStore) readManagedAccounts(entries []os.DirEntry) ([]app.Account, map[string]*jwt.AccountClaims, error) {
	accounts := make([]app.Account, 0)
	claims := make(map[string]*jwt.AccountClaims)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".account.jwt") {
			continue
		}
		token, err := os.ReadFile(filepath.Join(s.directory, entry.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("read managed account %s: %w", entry.Name(), err)
		}
		claim, err := jwt.DecodeAccountClaims(strings.TrimSpace(string(token)))
		if err != nil {
			return nil, nil, fmt.Errorf("decode managed account %s: %w", entry.Name(), err)
		}
		claims[claim.Subject] = claim
		accounts = append(accounts, managedAccount(claim, strings.TrimSpace(string(token))))
	}
	return accounts, claims, nil
}

func (s AuthorityStore) readManagedUsers(entries []os.DirEntry, accounts map[string]*jwt.AccountClaims) ([]app.User, error) {
	users := make([]app.User, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".creds") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(s.directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read managed user %s: %w", entry.Name(), err)
		}
		token, err := jwt.ParseDecoratedJWT(contents)
		if err != nil {
			return nil, fmt.Errorf("parse managed user %s: %w", entry.Name(), err)
		}
		claim, err := jwt.DecodeUserClaims(token)
		if err != nil {
			return nil, fmt.Errorf("decode managed user %s: %w", entry.Name(), err)
		}
		accountID := claim.IssuerAccount
		if accountID == "" {
			accountID = claim.Issuer
		}
		revoked := false
		if account := accounts[accountID]; account != nil {
			revoked = account.IsClaimRevoked(claim)
		}
		users = append(users, managedUser(claim, token, accountID, revoked))
	}
	return users, nil
}

func managedAccount(claim *jwt.AccountClaims, token string) app.Account {
	decoded, _ := json.Marshal(claim)
	keys := make([]string, 0, len(claim.SigningKeys))
	details := make([]app.SigningKey, 0, len(claim.SigningKeys))
	for key, scope := range claim.SigningKeys {
		keys = append(keys, key)
		detail := app.SigningKey{PublicKey: key}
		if userScope, ok := scope.(*jwt.UserScope); ok {
			detail.Scoped, detail.Role, detail.Description = true, userScope.Role, userScope.Description
			detail.PublishAllow = append([]string(nil), userScope.Template.Permissions.Pub.Allow...)
			detail.PublishDeny = append([]string(nil), userScope.Template.Permissions.Pub.Deny...)
			detail.SubscribeAllow = append([]string(nil), userScope.Template.Permissions.Sub.Allow...)
			detail.SubscribeDeny = append([]string(nil), userScope.Template.Permissions.Sub.Deny...)
		}
		details = append(details, detail)
	}
	sort.Strings(keys)
	sort.Slice(details, func(i, j int) bool { return details[i].PublicKey < details[j].PublicKey })
	imports := make([]app.Sharing, 0, len(claim.Imports))
	for _, item := range claim.Imports {
		if item != nil {
			imports = append(imports, app.Sharing{Name: item.Name, Subject: string(item.Subject), Type: item.Type.String(), Account: item.Account, Token: item.Token})
		}
	}
	exports := make([]app.Sharing, 0, len(claim.Exports))
	for _, item := range claim.Exports {
		if item != nil {
			exports = append(exports, app.Sharing{Name: item.Name, Subject: string(item.Subject), Type: item.Type.String(), Public: !item.TokenReq})
		}
	}
	return app.Account{ID: claim.Subject, Name: claim.Name, Description: claim.Description, Issuer: claim.Issuer, Managed: true, JetStream: claim.Limits.IsJSEnabled(), Limits: app.Limits{Connections: claim.Limits.Conn, LeafNodes: claim.Limits.LeafNodeConn, Subscriptions: claim.Limits.Subs, Payload: claim.Limits.Payload, Data: claim.Limits.Data, Exports: claim.Limits.Exports, Imports: claim.Limits.Imports, Streams: claim.Limits.Streams, Consumers: claim.Limits.Consumer, Memory: claim.Limits.MemoryStorage, Storage: claim.Limits.DiskStorage}, SigningKeys: keys, SigningKeyDetails: details, Imports: imports, Exports: exports, JWT: token, DecodedClaim: decoded}
}

func managedUser(claim *jwt.UserClaims, token, account string, revoked bool) app.User {
	responseMax, responseTTL := 0, int64(0)
	if claim.Permissions.Resp != nil {
		responseMax, responseTTL = claim.Permissions.Resp.MaxMsgs, int64(claim.Permissions.Resp.Expires)
	}
	return app.User{PublicKey: claim.Subject, Name: claim.Name, Account: account, Issuer: claim.Issuer, Revoked: revoked, PublishAllow: append([]string(nil), claim.Permissions.Pub.Allow...), PublishDeny: append([]string(nil), claim.Permissions.Pub.Deny...), SubscribeAllow: append([]string(nil), claim.Permissions.Sub.Allow...), SubscribeDeny: append([]string(nil), claim.Permissions.Sub.Deny...), ResponseMax: responseMax, ResponseTTL: responseTTL, SubscriptionLimit: claim.Limits.Subs, JWT: token}
}
