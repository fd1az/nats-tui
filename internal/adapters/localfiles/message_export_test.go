package localfiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMessageFileWriterPublishesSecurelyAndRefusesOverwrite(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "messages.jsonl")
	writer := NewOSMessageFileWriter()
	if err := writer.WriteMessageFile(context.Background(), path, []byte("first\n"), false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions=%#o, want 0600", info.Mode().Perm())
	}
	if err := writer.WriteMessageFile(context.Background(), path, []byte("second\n"), false); err == nil {
		t.Fatal("writer overwrote destination without force")
	}
	if err := writer.WriteMessageFile(context.Background(), path, []byte("second\n"), true); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "second\n" {
		t.Fatalf("content=%q error=%v", content, err)
	}
}
