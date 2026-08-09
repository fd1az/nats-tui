package architecture_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestApplicationDoesNotImportFrameworksOrAdapters(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-json", "../app/...")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("list application packages: %v", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}

		for _, dependency := range pkg.Imports {
			assertAllowedDependency(t, pkg.ImportPath, dependency)
		}
	}
}

func assertAllowedDependency(t *testing.T, pkg, dependency string) {
	t.Helper()

	forbidden := []string{
		"bubbletea",
		"lipgloss",
		"cobra",
		"viper",
		"nats.go",
		"internal/adapters",
		"internal/ports",
	}

	for _, fragment := range forbidden {
		if strings.Contains(dependency, fragment) {
			t.Errorf("application package %s imports forbidden dependency %s", pkg, dependency)
		}
	}
}
