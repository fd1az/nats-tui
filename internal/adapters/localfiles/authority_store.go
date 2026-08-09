package localfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	app "github.com/clcsolutions/nats-tui/internal/app/operator"
	jwt "github.com/nats-io/jwt/v2"
)

type AuthorityStore struct{ directory string }

func NewAuthorityStore(directory string) (AuthorityStore, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return AuthorityStore{}, fmt.Errorf("authority output directory is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return AuthorityStore{}, fmt.Errorf("create authority directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return AuthorityStore{}, fmt.Errorf("secure authority directory: %w", err)
	}
	return AuthorityStore{directory: directory}, nil
}

func (s AuthorityStore) StoreAccount(_ context.Context, name string, bundle app.AccountBundle) (app.AccountArtifacts, error) {
	stem := safeAuthorityName(name)
	seedPath, jwtPath := filepath.Join(s.directory, stem+".account.seed"), filepath.Join(s.directory, stem+".account.jwt")
	if err := writePrivate(seedPath, bundle.Seed); err != nil {
		return app.AccountArtifacts{}, err
	}
	if err := writePrivate(jwtPath, []byte(bundle.JWT+"\n")); err != nil {
		return app.AccountArtifacts{SeedPath: seedPath}, err
	}
	return app.AccountArtifacts{SeedPath: seedPath, JWTPath: jwtPath}, nil
}

func (s AuthorityStore) StoreUser(_ context.Context, name string, bundle app.UserBundle) (app.UserArtifacts, error) {
	path := filepath.Join(s.directory, safeAuthorityName(name)+".creds")
	if err := writePrivate(path, bundle.Credentials); err != nil {
		return app.UserArtifacts{}, err
	}
	return app.UserArtifacts{CredentialsPath: path}, nil
}

func (s AuthorityStore) ReplaceAccountClaim(_ context.Context, accountID, token string) (string, error) {
	claim, err := jwt.DecodeAccountClaims(strings.TrimSpace(token))
	if err != nil {
		return "", fmt.Errorf("decode replacement account claim: %w", err)
	}
	if claim.Subject != accountID {
		return "", fmt.Errorf("replacement claim subject does not match %s", accountID)
	}
	path, _, err := s.findAccount(accountID)
	if err != nil {
		return "", err
	}
	if err := replacePrivate(path, []byte(strings.TrimSpace(token)+"\n")); err != nil {
		return path, err
	}
	return path, nil
}

func (s AuthorityStore) ReplaceUser(_ context.Context, userID string, bundle app.UserBundle) (app.UserArtifacts, error) {
	if bundle.PublicKey != userID {
		return app.UserArtifacts{}, fmt.Errorf("replacement user does not match %s", userID)
	}
	token, err := jwt.ParseDecoratedJWT(bundle.Credentials)
	if err != nil {
		return app.UserArtifacts{}, fmt.Errorf("parse replacement credentials: %w", err)
	}
	claim, err := jwt.DecodeUserClaims(token)
	if err != nil {
		return app.UserArtifacts{}, fmt.Errorf("decode replacement user: %w", err)
	}
	if claim.Subject != userID {
		return app.UserArtifacts{}, fmt.Errorf("replacement credential subject does not match %s", userID)
	}
	path, _, err := s.findUser(userID)
	if err != nil {
		return app.UserArtifacts{}, err
	}
	if err := replacePrivate(path, bundle.Credentials); err != nil {
		return app.UserArtifacts{CredentialsPath: path}, err
	}
	return app.UserArtifacts{CredentialsPath: path}, nil
}

func (s AuthorityStore) StoreSigningKey(_ context.Context, accountID string, bundle app.SigningKeyBundle) (app.SigningKeyArtifacts, error) {
	if len(bundle.Seed) == 0 || bundle.PublicKey == "" {
		return app.SigningKeyArtifacts{}, fmt.Errorf("signing key bundle is incomplete")
	}
	stem := safeAuthorityName(accountID + "-" + shortPublicKey(bundle.PublicKey))
	path := filepath.Join(s.directory, stem+".signing.seed")
	if err := writePrivate(path, bundle.Seed); err != nil {
		return app.SigningKeyArtifacts{}, err
	}
	return app.SigningKeyArtifacts{SeedPath: path}, nil
}

func (s AuthorityStore) RemoveUser(_ context.Context, userID string) (string, error) {
	path, _, err := s.findUser(userID)
	if err != nil {
		return "", err
	}
	return s.moveToTrash(path)
}

func (s AuthorityStore) RemoveAccount(_ context.Context, accountID string) (string, error) {
	claimPath, _, err := s.findAccount(accountID)
	if err != nil {
		return "", err
	}
	trash, err := s.ensureTrash()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return "", fmt.Errorf("read authority directory: %w", err)
	}
	var paths []string
	paths = append(paths, claimPath, strings.TrimSuffix(claimPath, ".jwt")+".seed")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".signing.seed") && strings.HasPrefix(entry.Name(), safeAuthorityName(accountID)+"-") {
			paths = append(paths, filepath.Join(s.directory, entry.Name()))
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".creds") {
			continue
		}
		path := filepath.Join(s.directory, entry.Name())
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", fmt.Errorf("read managed user credentials: %w", readErr)
		}
		token, parseErr := jwt.ParseDecoratedJWT(contents)
		if parseErr != nil {
			return "", fmt.Errorf("parse managed user credentials: %w", parseErr)
		}
		claim, decodeErr := jwt.DecodeUserClaims(token)
		if decodeErr != nil {
			return "", fmt.Errorf("decode managed user claim: %w", decodeErr)
		}
		issuer := claim.IssuerAccount
		if issuer == "" {
			issuer = claim.Issuer
		}
		if issuer == accountID {
			paths = append(paths, path)
		}
	}
	for _, path := range paths {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			continue
		}
		target := filepath.Join(trash, filepath.Base(path)+".deleted")
		if err := renameWithoutOverwrite(path, target); err != nil {
			return trash, err
		}
	}
	return trash, nil
}

func (s AuthorityStore) moveToTrash(path string) (string, error) {
	trash, err := s.ensureTrash()
	if err != nil {
		return "", err
	}
	target := filepath.Join(trash, filepath.Base(path)+".deleted")
	if err := renameWithoutOverwrite(path, target); err != nil {
		return target, err
	}
	return target, nil
}

func (s AuthorityStore) ensureTrash() (string, error) {
	path := filepath.Join(s.directory, ".trash")
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("create authority trash: %w", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return "", fmt.Errorf("secure authority trash: %w", err)
	}
	return path, nil
}

func renameWithoutOverwrite(source, target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("recoverable artifact already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect recoverable artifact: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("move artifact to trash: %w", err)
	}
	return nil
}

func replacePrivate(path string, contents []byte) error {
	temporary := path + ".replacement"
	if err := writePrivate(temporary, contents); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replace private artifact %s: %w", path, err)
	}
	return os.Chmod(path, 0o600)
}

func shortPublicKey(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func writePrivate(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create private artifact %s: %w", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write private artifact %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close private artifact %s: %w", path, err)
	}
	return nil
}

func safeAuthorityName(value string) string {
	var result strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			result.WriteRune(r)
			continue
		}
		result.WriteByte('-')
	}
	name := strings.Trim(result.String(), "-")
	if name == "" {
		return "artifact"
	}
	return name
}
