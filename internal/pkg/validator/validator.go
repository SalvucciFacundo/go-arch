package validator

import (
	"fmt"
	"go-arch/internal/ui"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/oops"
)

// Violation represents an architectural rule violation.
type Violation struct {
	File     string
	Message  string
	Severity string // "ERROR" or "WARNING"
}

type Validator struct {
	config *ui.ProjectConfig
}

func NewValidator(config *ui.ProjectConfig) *Validator {
	return &Validator{config: config}
}

// Validate verifies the project structure and dependencies according to the configured architecture.
func (v *Validator) Validate() ([]Violation, error) {
	var violations []Violation

	// 1. Validate folder integrity
	structureViolations := v.checkStructure()
	violations = append(violations, structureViolations...)

	// 2. Validate import rules (Dependency Rule)
	dependencyViolations, err := v.checkDependencies()
	if err != nil {
		return nil, oops.
			Code("validator_io_error").
			Wrapf(err, "Error analyzing project dependencies")
	}
	violations = append(violations, dependencyViolations...)

	return violations, nil
}

func (v *Validator) checkStructure() []Violation {
	var violations []Violation
	var requiredDirs []string

	switch v.config.Architecture {
	case "Hexagonal":
		requiredDirs = []string{"internal/domain", "internal/ports", "internal/adapters"}
	case "Standard":
		requiredDirs = []string{"internal/handler", "internal/service", "internal/repository", "internal/model"}
	}

	for _, dir := range requiredDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			violations = append(violations, Violation{
				File:     dir,
				Message:  fmt.Sprintf("Missing required folder for layout %s", v.config.Architecture),
				Severity: "ERROR",
			})
		}
	}

	return violations
}

func (v *Validator) checkDependencies() ([]Violation, error) {
	var violations []Violation

	err := filepath.Walk("internal", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil // Ignore files with syntax errors for now
		}

		fileViolations := v.applyArchitectureRules(path, f.Imports)
		violations = append(violations, fileViolations...)

		return nil
	})

	return violations, err
}

func (v *Validator) applyArchitectureRules(path string, imports []*ast.ImportSpec) []Violation {
	var violations []Violation
	modulePrefix := v.config.ModuleName + "/internal"

	for _, imp := range imports {
		importPath := strings.Trim(imp.Path.Value, "\"")

		// We are only interested in the project's own internal imports
		if !strings.HasPrefix(importPath, modulePrefix) {
			continue
		}

		relImport := strings.TrimPrefix(importPath, modulePrefix)

		switch v.config.Architecture {
		case "Hexagonal":
			// Rule: domain cannot import anything from ports or adapters
			if strings.Contains(path, "internal/domain") {
				if strings.Contains(relImport, "/ports") || strings.Contains(relImport, "/adapters") {
					violations = append(violations, Violation{
						File:     path,
						Message:  fmt.Sprintf("Layer leak: domain must not import '%s'", importPath),
						Severity: "ERROR",
					})
				}
			}
			// Rule: ports cannot import adapters
			if strings.Contains(path, "internal/ports") {
				if strings.Contains(relImport, "/adapters") {
					violations = append(violations, Violation{
						File:     path,
						Message:  fmt.Sprintf("Layer leak: ports (interfaces) must not import adapters '%s'", importPath),
						Severity: "ERROR",
					})
				}
			}

		case "Standard":
			// Rule: model imports nothing
			if strings.Contains(path, "internal/model") {
				violations = append(violations, Violation{
					File:     path,
					Message:  fmt.Sprintf("Package 'model' must be self-contained and must not import '%s'", importPath),
					Severity: "ERROR",
				})
			}
			// Rule: repository does not import service or handler
			if strings.Contains(path, "internal/repository") {
				if strings.Contains(relImport, "/service") || strings.Contains(relImport, "/handler") {
					violations = append(violations, Violation{
						File:     path,
						Message:  fmt.Sprintf("Forbidden dependency inversion: repository must not depend on '%s'", importPath),
						Severity: "ERROR",
					})
				}
			}
		}
	}

	return violations
}
