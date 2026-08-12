package natscontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	app "github.com/fd1az/nats-tui/internal/app/connections"
)

type FileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}
type OSFileSystem struct{}

func (OSFileSystem) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (OSFileSystem) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }

type Importer struct{ files FileSystem }

func NewImporter(files FileSystem) Importer {
	if files == nil {
		panic("nil NATS context filesystem")
	}
	return Importer{files: files}
}

type contextDTO struct {
	URL         string `json:"url"`
	User        string `json:"user"`
	Password    string `json:"password"`
	Token       string `json:"token"`
	Creds       string `json:"creds"`
	NKey        string `json:"nkey"`
	UserJWT     string `json:"user_jwt"`
	UserSeed    string `json:"user_seed"`
	Certificate string `json:"cert"`
	Key         string `json:"key"`
	CA          string `json:"ca"`
}

func (i Importer) Profiles(ctx context.Context, directory string) ([]app.ProfileInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("NATS context directory is required")
	}
	entries, err := i.files.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read NATS context directory: %w", err)
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() < entries[b].Name() })
	result := make([]app.ProfileInput, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := i.files.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read NATS context %s: %w", entry.Name(), err)
		}
		var source contextDTO
		if err := json.Unmarshal(data, &source); err != nil {
			return nil, fmt.Errorf("decode NATS context %s: %w", entry.Name(), err)
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		input := app.ProfileInput{Name: name, Servers: []string{source.URL}, Token: source.Token, Credentials: reference(source.Creds), Username: source.User, Password: source.Password, NKeySeed: reference(source.NKey), UserJWT: reference(source.UserJWT), UserSeed: reference(source.UserSeed), TLSCA: reference(source.CA), TLSCert: reference(source.Certificate), TLSKey: reference(source.Key)}
		if _, err := input.Profile(); err != nil {
			return nil, fmt.Errorf("validate NATS context %s: %w", entry.Name(), err)
		}
		result = append(result, input)
	}
	return result, nil
}

func reference(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "file://")
}
