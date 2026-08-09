package jwtauthority

import (
	"bytes"
	"fmt"
	"time"

	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	domain "github.com/clcsolutions/nats-tui/internal/domain/authority"
	jwt "github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

type KeyFactory interface {
	CreateAccount() (nkeys.KeyPair, error)
	CreateUser() (nkeys.KeyPair, error)
}

type nkeyFactory struct{}

func (nkeyFactory) CreateAccount() (nkeys.KeyPair, error) { return nkeys.CreateAccount() }
func (nkeyFactory) CreateUser() (nkeys.KeyPair, error)    { return nkeys.CreateUser() }

type Signer struct {
	operator nkeys.KeyPair
	keys     KeyFactory
}

func NewDefault(operatorSeed []byte) (Signer, error) { return New(operatorSeed, nkeyFactory{}) }

func NewDefaultFromAccount(accountSeed []byte) (Signer, error) {
	return NewFromAccount(accountSeed, nkeyFactory{})
}

func NewFromAccount(accountSeed []byte, keys KeyFactory) (Signer, error) {
	if keys == nil {
		return Signer{}, fmt.Errorf("key factory is required")
	}
	pair, err := nkeys.FromSeed(bytes.TrimSpace(accountSeed))
	if err != nil {
		return Signer{}, fmt.Errorf("load account seed: %w", err)
	}
	publicKey, err := pair.PublicKey()
	if err != nil || !nkeys.IsValidPublicAccountKey(publicKey) {
		return Signer{}, fmt.Errorf("seed is not an account seed")
	}
	return Signer{keys: keys}, nil
}

func New(operatorSeed []byte, keys KeyFactory) (Signer, error) {
	if keys == nil {
		return Signer{}, fmt.Errorf("key factory is required")
	}
	pair, err := nkeys.FromSeed(bytes.TrimSpace(operatorSeed))
	if err != nil {
		return Signer{}, fmt.Errorf("load operator seed: %w", err)
	}
	publicKey, err := pair.PublicKey()
	if err != nil || !nkeys.IsValidPublicOperatorKey(publicKey) {
		return Signer{}, fmt.Errorf("seed is not an operator seed")
	}
	return Signer{operator: pair, keys: keys}, nil
}

func (s Signer) CreateAccount(definition domain.AccountDefinition) (app.AccountBundle, error) {
	if s.operator == nil {
		return app.AccountBundle{}, fmt.Errorf("operator signer is unavailable")
	}
	pair, err := s.keys.CreateAccount()
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("generate account key: %w", err)
	}
	publicKey, err := pair.PublicKey()
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("read account public key: %w", err)
	}
	claim := jwt.NewAccountClaims(publicKey)
	claim.Name = definition.Name()
	claim.Description = definition.Description()
	applyAccountLimits(claim, definition.Limits())
	token, err := claim.Encode(s.operator)
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("sign account claim: %w", err)
	}
	seed, err := pair.Seed()
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("read account seed: %w", err)
	}
	return app.AccountBundle{PublicKey: publicKey, JWT: token, Seed: seed}, nil
}

func (s Signer) UpdateAccount(currentJWT string, definition domain.AccountDefinition) (app.AccountBundle, error) {
	if s.operator == nil {
		return app.AccountBundle{}, fmt.Errorf("operator signer is unavailable")
	}
	claim, err := jwt.DecodeAccountClaims(currentJWT)
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("decode account claim: %w", err)
	}
	claim.Name = definition.Name()
	claim.Description = definition.Description()
	applyAccountLimits(claim, definition.Limits())
	token, err := claim.Encode(s.operator)
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("sign account claim: %w", err)
	}
	return app.AccountBundle{PublicKey: claim.Subject, JWT: token}, nil
}

func (s Signer) CreateUser(accountSeed []byte, definition domain.UserDefinition) (app.UserBundle, error) {
	accountPair, err := nkeys.FromSeed(bytes.TrimSpace(accountSeed))
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("load account seed: %w", err)
	}
	accountPublic, err := accountPair.PublicKey()
	if err != nil || !nkeys.IsValidPublicAccountKey(accountPublic) {
		return app.UserBundle{}, fmt.Errorf("seed is not an account seed")
	}
	if accountPublic != definition.Account() {
		return app.UserBundle{}, fmt.Errorf("account seed does not match %s", definition.Account())
	}
	userPair, err := s.keys.CreateUser()
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("generate user key: %w", err)
	}
	userPublic, err := userPair.PublicKey()
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("read user public key: %w", err)
	}
	claim := jwt.NewUserClaims(userPublic)
	claim.Name = definition.Name()
	applyUserPermissions(claim, definition.Permissions())
	token, err := claim.Encode(accountPair)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("sign user claim: %w", err)
	}
	seed, err := userPair.Seed()
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("read user seed: %w", err)
	}
	credentials, err := jwt.FormatUserConfig(token, seed)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("format user credentials: %w", err)
	}
	return app.UserBundle{PublicKey: userPublic, JWT: token, Credentials: credentials}, nil
}

func (s Signer) UpdateUser(accountSeed, userSeed []byte, currentJWT string, definition domain.UserDefinition) (app.UserBundle, error) {
	accountPair, accountPublic, err := accountKey(accountSeed)
	if err != nil {
		return app.UserBundle{}, err
	}
	if accountPublic != definition.Account() {
		return app.UserBundle{}, fmt.Errorf("account seed does not match %s", definition.Account())
	}
	userPair, err := nkeys.FromSeed(bytes.TrimSpace(userSeed))
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("load user seed: %w", err)
	}
	userPublic, err := userPair.PublicKey()
	if err != nil || !nkeys.IsValidPublicUserKey(userPublic) {
		return app.UserBundle{}, fmt.Errorf("seed is not a user seed")
	}
	claim, err := jwt.DecodeUserClaims(currentJWT)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("decode user claim: %w", err)
	}
	if claim.Subject != userPublic {
		return app.UserBundle{}, fmt.Errorf("user seed does not match current claim")
	}
	claim.Name = definition.Name()
	claim.Permissions = jwt.Permissions{}
	claim.Limits.Subs = definition.Permissions().SubscriptionLimit
	applyUserPermissions(claim, definition.Permissions())
	token, err := claim.Encode(accountPair)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("sign user claim: %w", err)
	}
	seed, err := userPair.Seed()
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("read user seed: %w", err)
	}
	credentials, err := jwt.FormatUserConfig(token, seed)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("format user credentials: %w", err)
	}
	return app.UserBundle{PublicKey: userPublic, JWT: token, Credentials: credentials}, nil
}

func (s Signer) RevokeUser(currentJWT, userPublicKey string, at time.Time) (app.AccountBundle, error) {
	if s.operator == nil {
		return app.AccountBundle{}, fmt.Errorf("operator signer is unavailable")
	}
	if !nkeys.IsValidPublicUserKey(userPublicKey) {
		return app.AccountBundle{}, fmt.Errorf("invalid user public key")
	}
	claim, err := jwt.DecodeAccountClaims(currentJWT)
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("decode account claim: %w", err)
	}
	claim.RevokeAt(userPublicKey, at)
	token, err := claim.Encode(s.operator)
	if err != nil {
		return app.AccountBundle{}, fmt.Errorf("sign account revocation: %w", err)
	}
	return app.AccountBundle{PublicKey: claim.Subject, JWT: token}, nil
}

func (s Signer) AddSigningKey(currentJWT string, definition domain.SigningKeyDefinition) (app.AccountBundle, app.SigningKeyBundle, error) {
	if s.operator == nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("operator signer is unavailable")
	}
	claim, err := jwt.DecodeAccountClaims(currentJWT)
	if err != nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("decode account claim: %w", err)
	}
	pair, err := s.keys.CreateAccount()
	if err != nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("generate signing key: %w", err)
	}
	publicKey, err := pair.PublicKey()
	if err != nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("read signing public key: %w", err)
	}
	if definition.Scoped() {
		scope := jwt.NewUserScope()
		scope.Key, scope.Role, scope.Description = publicKey, definition.Role(), definition.Description()
		applyPermissionLimits(&scope.Template, definition.Template())
		claim.SigningKeys.AddScopedSigner(scope)
	} else {
		claim.SigningKeys.Add(publicKey)
	}
	token, err := claim.Encode(s.operator)
	if err != nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("sign account claim: %w", err)
	}
	seed, err := pair.Seed()
	if err != nil {
		return app.AccountBundle{}, app.SigningKeyBundle{}, fmt.Errorf("read signing seed: %w", err)
	}
	return app.AccountBundle{PublicKey: claim.Subject, JWT: token}, app.SigningKeyBundle{PublicKey: publicKey, Seed: seed}, nil
}

func (s Signer) DeleteAccountRequest(accountID string) (string, error) {
	if s.operator == nil {
		return "", fmt.Errorf("operator signer is unavailable")
	}
	if !nkeys.IsValidPublicAccountKey(accountID) {
		return "", fmt.Errorf("invalid account public key")
	}
	operatorPublic, err := s.operator.PublicKey()
	if err != nil {
		return "", fmt.Errorf("read operator public key: %w", err)
	}
	claim := jwt.NewGenericClaims(operatorPublic)
	claim.Data["accounts"] = []string{accountID}
	token, err := claim.Encode(s.operator)
	if err != nil {
		return "", fmt.Errorf("sign account delete request: %w", err)
	}
	return token, nil
}

func accountKey(seed []byte) (nkeys.KeyPair, string, error) {
	pair, err := nkeys.FromSeed(bytes.TrimSpace(seed))
	if err != nil {
		return nil, "", fmt.Errorf("load account seed: %w", err)
	}
	publicKey, err := pair.PublicKey()
	if err != nil || !nkeys.IsValidPublicAccountKey(publicKey) {
		return nil, "", fmt.Errorf("seed is not an account seed")
	}
	return pair, publicKey, nil
}

func signUser(accountPair, userPair nkeys.KeyPair, userPublic string, definition domain.UserDefinition) (app.UserBundle, error) {
	claim := jwt.NewUserClaims(userPublic)
	claim.Name = definition.Name()
	applyUserPermissions(claim, definition.Permissions())
	token, err := claim.Encode(accountPair)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("sign user claim: %w", err)
	}
	seed, err := userPair.Seed()
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("read user seed: %w", err)
	}
	credentials, err := jwt.FormatUserConfig(token, seed)
	if err != nil {
		return app.UserBundle{}, fmt.Errorf("format user credentials: %w", err)
	}
	return app.UserBundle{PublicKey: userPublic, JWT: token, Credentials: credentials}, nil
}

func applyAccountLimits(claim *jwt.AccountClaims, limits domain.AccountLimits) {
	claim.Limits.Conn = limits.Connections
	claim.Limits.LeafNodeConn = limits.LeafNodes
	claim.Limits.Subs = limits.Subscriptions
	claim.Limits.Payload = limits.Payload
	claim.Limits.Data = limits.Data
	claim.Limits.Exports = limits.Exports
	claim.Limits.Imports = limits.Imports
	claim.Limits.Streams = limits.Streams
	claim.Limits.Consumer = limits.Consumers
	claim.Limits.MemoryStorage = limits.Memory
	claim.Limits.DiskStorage = limits.Storage
}

func applyUserPermissions(claim *jwt.UserClaims, permissions domain.Permissions) {
	applyPermissionLimits(&claim.UserPermissionLimits, permissions)
}

func applyPermissionLimits(target *jwt.UserPermissionLimits, permissions domain.Permissions) {
	target.Permissions.Pub.Allow = append(jwt.StringList(nil), permissions.PublishAllow...)
	target.Permissions.Pub.Deny = append(jwt.StringList(nil), permissions.PublishDeny...)
	target.Permissions.Sub.Allow = append(jwt.StringList(nil), permissions.SubscribeAllow...)
	target.Permissions.Sub.Deny = append(jwt.StringList(nil), permissions.SubscribeDeny...)
	if permissions.ResponseMax > 0 || permissions.ResponseTTL > 0 {
		target.Permissions.Resp = &jwt.ResponsePermission{MaxMsgs: permissions.ResponseMax, Expires: permissions.ResponseTTL}
	}
	target.Limits.Subs = permissions.SubscriptionLimit
}
