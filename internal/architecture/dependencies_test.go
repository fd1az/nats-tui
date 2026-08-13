package architecture_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestApplicationDoesNotImportFrameworksOrAdapters(t *testing.T) {
	t.Parallel()

	packages := listPackages(t, "../app/...")
	for _, pkg := range packages {
		for _, dependency := range pkg.Imports {
			assertApplicationDependencyAllowed(t, pkg.ImportPath, dependency)
		}
	}
}

func TestDomainDependsOnlyOnStandardLibraryAndDomain(t *testing.T) {
	t.Parallel()

	packages := listPackages(t, "../domain/...")
	for _, pkg := range packages {
		for _, dependency := range pkg.Imports {
			if strings.Contains(dependency, ".") && !strings.HasPrefix(dependency, "github.com/fd1az/nats-tui/internal/domain/") {
				t.Errorf("domain package %s imports non-domain dependency %s", pkg.ImportPath, dependency)
			}
		}
	}
}

type packageDependencies struct {
	ImportPath string
	Imports    []string
}

func listPackages(t *testing.T, pattern string) []packageDependencies {
	t.Helper()

	cmd := exec.Command("go", "list", "-json", pattern)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("list packages %s: %v", pattern, err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(out)))
	var packages []packageDependencies
	for decoder.More() {
		var pkg packageDependencies
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

func assertApplicationDependencyAllowed(t *testing.T, pkg, dependency string) {
	t.Helper()

	forbidden := []string{
		"bubbletea",
		"lipgloss",
		"cobra",
		"viper",
		"nats.go",
		"internal/adapters",
		"internal/ports",
		"internal/bootstrap",
	}

	for _, fragment := range forbidden {
		if strings.Contains(dependency, fragment) {
			t.Errorf("application package %s imports forbidden dependency %s", pkg, dependency)
		}
	}
}
