package validator

import (
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"go/ast"
	"os"
	"testing"
)

func TestValidator_checkStructure(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "validator_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Change the working directory for the test
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	config := &ui.ProjectConfig{
		Architecture: "Hexagonal",
	}
	v := NewValidator(config)

	// Case 1: Empty structure -> Should have 3 errors (domain, ports, adapters)
	violations := v.checkStructure()
	if len(violations) != 3 {
		t.Errorf("Expected 3 structure violations, got %d", len(violations))
	}

	// Case 2: Create a folder -> Should have 2 errors
	if err := os.MkdirAll("internal/domain", 0755); err != nil {
		t.Fatal(err)
	}
	violations = v.checkStructure()
	if len(violations) != 2 {
		t.Errorf("Expected 2 structure violations, got %d", len(violations))
	}
}

func TestValidator_applyArchitectureRules_Hexagonal(t *testing.T) {
	config := &ui.ProjectConfig{
		ModuleName:   "github.com/test/app",
		Architecture: "Hexagonal",
	}
	v := NewValidator(config)

	tests := []struct {
		name       string
		path       string
		importPath string
		wantError  bool
	}{
		{
			name:       "Domain importing project root (Legal)",
			path:       "internal/domain/user.go",
			importPath: "github.com/test/app",
			wantError:  false,
		},
		{
			name:       "Domain importing adapters (Illegal)",
			path:       "internal/domain/user.go",
			importPath: "github.com/test/app/internal/adapters/db",
			wantError:  true,
		},
		{
			name:       "Adapters importing domain (Legal)",
			path:       "internal/adapters/db/user_repo.go",
			importPath: "github.com/test/app/internal/domain",
			wantError:  false,
		},
		{
			name:       "Ports importing adapters (Illegal)",
			path:       "internal/ports/user_repo.go",
			importPath: "github.com/test/app/internal/adapters/db",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We simulate the ast.ImportSpec object in a minimal way for the internal test
			// Note: In the real code we use applyArchitectureRules which receives []*ast.ImportSpec
			// For the test, we invoke the internal logic we extracted.

			// Since the logic is in a private or semi-private method, we can test it
			// by passing a dummy or adjusting visibility if necessary.
			// For simplicity in this environment, we verify the switch logic.

			// Quick re-implementation of the logic for the pure unit test
			violations := v.applyArchitectureRules(tt.path, createDummyImports(tt.importPath))
			if (len(violations) > 0) != tt.wantError {
				t.Errorf("applyArchitectureRules() error = %v, wantError %v", len(violations) > 0, tt.wantError)
			}
		})
	}
}

// Helper to create fake imports for the test
func createDummyImports(path string) []*ast.ImportSpec {
	importSpec := &ast.ImportSpec{
		Path: &ast.BasicLit{
			Value: "\"" + path + "\"",
		},
	}
	return []*ast.ImportSpec{importSpec}
}
