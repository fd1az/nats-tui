package operator

import (
	"context"
	"errors"
	"time"

	domain "github.com/fd1az/nats-tui/internal/domain/authority"
)

type AccountBundle struct {
	PublicKey string
	JWT       string
	Seed      []byte
}

type UserBundle struct {
	PublicKey   string
	JWT         string
	Credentials []byte
}

type AccountArtifacts struct{ SeedPath, JWTPath string }
type UserArtifacts struct{ CredentialsPath string }
type SigningKeyBundle struct {
	PublicKey string
	Seed      []byte
}
type SigningKeyArtifacts struct{ SeedPath string }

type AuthoritySigner interface {
	CreateAccount(domain.AccountDefinition) (AccountBundle, error)
	UpdateAccount(string, domain.AccountDefinition) (AccountBundle, error)
	CreateUser([]byte, domain.UserDefinition) (UserBundle, error)
	UpdateUser([]byte, []byte, string, domain.UserDefinition) (UserBundle, error)
	RevokeUser(string, string, time.Time) (AccountBundle, error)
	AddSigningKey(string, domain.SigningKeyDefinition) (AccountBundle, SigningKeyBundle, error)
	DeleteAccountRequest(string) (string, error)
}

type ArtifactStore interface {
	StoreAccount(context.Context, string, AccountBundle) (AccountArtifacts, error)
	StoreUser(context.Context, string, UserBundle) (UserArtifacts, error)
}

type AuthorityRepository interface {
	ArtifactStore
	Source
	AccountSeed(context.Context, string) ([]byte, error)
	AccountClaim(context.Context, string) (string, error)
	ReplaceAccountClaim(context.Context, string, string) (string, error)
	UserSeed(context.Context, string) ([]byte, error)
	UserClaim(context.Context, string) (string, error)
	ReplaceUser(context.Context, string, UserBundle) (UserArtifacts, error)
	StoreSigningKey(context.Context, string, SigningKeyBundle) (SigningKeyArtifacts, error)
	RemoveUser(context.Context, string) (string, error)
	RemoveAccount(context.Context, string) (string, error)
}

type ClaimPublisher interface {
	UpdateAccountClaim(context.Context, string) error
	DeleteAccountClaim(context.Context, string) error
}

type AccountCreator interface {
	CreateAccount(domain.AccountDefinition) (AccountBundle, error)
}

type AccountUpdater interface {
	UpdateAccount(string, domain.AccountDefinition) (AccountBundle, error)
}

type UserCreator interface {
	CreateUser([]byte, domain.UserDefinition) (UserBundle, error)
}

type UserUpdater interface {
	UpdateUser([]byte, []byte, string, domain.UserDefinition) (UserBundle, error)
}

type UserRevoker interface {
	RevokeUser(string, string, time.Time) (AccountBundle, error)
}

type SigningKeyCreator interface {
	AddSigningKey(string, domain.SigningKeyDefinition) (AccountBundle, SigningKeyBundle, error)
}

type AccountDeleteSigner interface {
	DeleteAccountRequest(string) (string, error)
}

type ClaimUpdater interface {
	UpdateAccountClaim(context.Context, string) error
}
type ClaimDeleter interface {
	DeleteAccountClaim(context.Context, string) error
}

type CreateAccountHandler struct {
	signer    AccountCreator
	store     ArtifactStore
	publisher ClaimUpdater
}

func NewCreateAccountHandler(signer AccountCreator, store ArtifactStore, publisher ClaimUpdater) CreateAccountHandler {
	if signer == nil || store == nil || publisher == nil {
		panic("account authority dependencies cannot be nil")
	}
	return CreateAccountHandler{signer: signer, store: store, publisher: publisher}
}

func (h CreateAccountHandler) Handle(ctx context.Context, definition domain.AccountDefinition) (AccountArtifacts, error) {
	if definition.IsZero() {
		return AccountArtifacts{}, errors.New("account definition is required")
	}
	bundle, err := h.signer.CreateAccount(definition)
	if err != nil {
		return AccountArtifacts{}, err
	}
	artifacts, err := h.store.StoreAccount(ctx, definition.Name(), cloneAccountBundle(bundle))
	if err != nil {
		return AccountArtifacts{}, err
	}
	if err := h.publisher.UpdateAccountClaim(ctx, bundle.JWT); err != nil {
		return artifacts, err
	}
	return artifacts, nil
}

type CreateUserHandler struct {
	signer UserCreator
	store  ArtifactStore
}

func NewCreateUserHandler(signer UserCreator, store ArtifactStore) CreateUserHandler {
	if signer == nil || store == nil {
		panic("user authority dependencies cannot be nil")
	}
	return CreateUserHandler{signer: signer, store: store}
}

type UpdateAccountHandler struct {
	signer    AccountUpdater
	repo      AuthorityRepository
	publisher ClaimUpdater
}

func NewUpdateAccountHandler(signer AccountUpdater, repo AuthorityRepository, publisher ClaimUpdater) UpdateAccountHandler {
	if signer == nil || repo == nil || publisher == nil {
		panic("account update dependencies cannot be nil")
	}
	return UpdateAccountHandler{signer: signer, repo: repo, publisher: publisher}
}

func (h UpdateAccountHandler) Handle(ctx context.Context, accountID string, definition domain.AccountDefinition) (AccountArtifacts, error) {
	if accountID == "" || definition.IsZero() {
		return AccountArtifacts{}, errors.New("account and definition are required")
	}
	current, err := h.repo.AccountClaim(ctx, accountID)
	if err != nil {
		return AccountArtifacts{}, err
	}
	bundle, err := h.signer.UpdateAccount(current, definition)
	if err != nil {
		return AccountArtifacts{}, err
	}
	path, err := h.repo.ReplaceAccountClaim(ctx, accountID, bundle.JWT)
	artifacts := AccountArtifacts{JWTPath: path}
	if err != nil {
		return artifacts, err
	}
	if err := h.publisher.UpdateAccountClaim(ctx, bundle.JWT); err != nil {
		return artifacts, err
	}
	return artifacts, nil
}

type UpdateUserHandler struct {
	signer UserUpdater
	repo   AuthorityRepository
}

func NewUpdateUserHandler(signer UserUpdater, repo AuthorityRepository) UpdateUserHandler {
	if signer == nil || repo == nil {
		panic("user update dependencies cannot be nil")
	}
	return UpdateUserHandler{signer: signer, repo: repo}
}

func (h UpdateUserHandler) Handle(ctx context.Context, userID string, definition domain.UserDefinition) (UserArtifacts, error) {
	if userID == "" || definition.IsZero() {
		return UserArtifacts{}, errors.New("user and definition are required")
	}
	accountSeed, err := h.repo.AccountSeed(ctx, definition.Account())
	if err != nil {
		return UserArtifacts{}, err
	}
	userSeed, err := h.repo.UserSeed(ctx, userID)
	if err != nil {
		return UserArtifacts{}, err
	}
	currentJWT, err := h.repo.UserClaim(ctx, userID)
	if err != nil {
		return UserArtifacts{}, err
	}
	bundle, err := h.signer.UpdateUser(accountSeed, userSeed, currentJWT, definition)
	if err != nil {
		return UserArtifacts{}, err
	}
	return h.repo.ReplaceUser(ctx, userID, cloneUserBundle(bundle))
}

type RevokeUserHandler struct {
	signer    UserRevoker
	repo      AuthorityRepository
	publisher ClaimUpdater
}

func NewRevokeUserHandler(signer UserRevoker, repo AuthorityRepository, publisher ClaimUpdater) RevokeUserHandler {
	if signer == nil || repo == nil || publisher == nil {
		panic("user revoke dependencies cannot be nil")
	}
	return RevokeUserHandler{signer: signer, repo: repo, publisher: publisher}
}

func (h RevokeUserHandler) Handle(ctx context.Context, accountID, userID string, at time.Time) (AccountArtifacts, error) {
	if accountID == "" || userID == "" || at.IsZero() {
		return AccountArtifacts{}, errors.New("account, user, and revocation time are required")
	}
	current, err := h.repo.AccountClaim(ctx, accountID)
	if err != nil {
		return AccountArtifacts{}, err
	}
	bundle, err := h.signer.RevokeUser(current, userID, at)
	if err != nil {
		return AccountArtifacts{}, err
	}
	path, err := h.repo.ReplaceAccountClaim(ctx, accountID, bundle.JWT)
	artifacts := AccountArtifacts{JWTPath: path}
	if err != nil {
		return artifacts, err
	}
	if err := h.publisher.UpdateAccountClaim(ctx, bundle.JWT); err != nil {
		return artifacts, err
	}
	return artifacts, nil
}

type DeleteUserHandler struct {
	revoke RevokeUserHandler
	repo   AuthorityRepository
}

func NewDeleteUserHandler(signer UserRevoker, repo AuthorityRepository, publisher ClaimUpdater) DeleteUserHandler {
	return DeleteUserHandler{revoke: NewRevokeUserHandler(signer, repo, publisher), repo: repo}
}

func (h DeleteUserHandler) Handle(ctx context.Context, accountID, userID string, at time.Time) (string, error) {
	if _, err := h.revoke.Handle(ctx, accountID, userID, at); err != nil {
		return "", err
	}
	return h.repo.RemoveUser(ctx, userID)
}

type AddSigningKeyHandler struct {
	signer    SigningKeyCreator
	repo      AuthorityRepository
	publisher ClaimUpdater
}

func NewAddSigningKeyHandler(signer SigningKeyCreator, repo AuthorityRepository, publisher ClaimUpdater) AddSigningKeyHandler {
	if signer == nil || repo == nil || publisher == nil {
		panic("signing key dependencies cannot be nil")
	}
	return AddSigningKeyHandler{signer: signer, repo: repo, publisher: publisher}
}

func (h AddSigningKeyHandler) Handle(ctx context.Context, accountID string, definition domain.SigningKeyDefinition) (SigningKeyArtifacts, error) {
	if accountID == "" {
		return SigningKeyArtifacts{}, errors.New("account is required")
	}
	current, err := h.repo.AccountClaim(ctx, accountID)
	if err != nil {
		return SigningKeyArtifacts{}, err
	}
	account, signingKey, err := h.signer.AddSigningKey(current, definition)
	if err != nil {
		return SigningKeyArtifacts{}, err
	}
	artifacts, err := h.repo.StoreSigningKey(ctx, accountID, SigningKeyBundle{PublicKey: signingKey.PublicKey, Seed: append([]byte(nil), signingKey.Seed...)})
	if err != nil {
		return SigningKeyArtifacts{}, err
	}
	if _, err := h.repo.ReplaceAccountClaim(ctx, accountID, account.JWT); err != nil {
		return artifacts, err
	}
	if err := h.publisher.UpdateAccountClaim(ctx, account.JWT); err != nil {
		return artifacts, err
	}
	return artifacts, nil
}

type DeleteAccountHandler struct {
	signer    AccountDeleteSigner
	repo      AuthorityRepository
	publisher ClaimDeleter
}

func NewDeleteAccountHandler(signer AccountDeleteSigner, repo AuthorityRepository, publisher ClaimDeleter) DeleteAccountHandler {
	if signer == nil || repo == nil || publisher == nil {
		panic("account delete dependencies cannot be nil")
	}
	return DeleteAccountHandler{signer: signer, repo: repo, publisher: publisher}
}

func (h DeleteAccountHandler) Handle(ctx context.Context, accountID string) (string, error) {
	if accountID == "" {
		return "", errors.New("account is required")
	}
	token, err := h.signer.DeleteAccountRequest(accountID)
	if err != nil {
		return "", err
	}
	if err := h.publisher.DeleteAccountClaim(ctx, token); err != nil {
		return "", err
	}
	return h.repo.RemoveAccount(ctx, accountID)
}

func (h CreateUserHandler) Handle(ctx context.Context, accountSeed []byte, definition domain.UserDefinition) (UserArtifacts, error) {
	if len(accountSeed) == 0 || definition.IsZero() {
		return UserArtifacts{}, errors.New("account seed and user definition are required")
	}
	bundle, err := h.signer.CreateUser(append([]byte(nil), accountSeed...), definition)
	if err != nil {
		return UserArtifacts{}, err
	}
	return h.store.StoreUser(ctx, definition.Name(), cloneUserBundle(bundle))
}

func cloneAccountBundle(value AccountBundle) AccountBundle {
	value.Seed = append([]byte(nil), value.Seed...)
	return value
}

func cloneUserBundle(value UserBundle) UserBundle {
	value.Credentials = append([]byte(nil), value.Credentials...)
	return value
}
