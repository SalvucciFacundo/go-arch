package scaffold

import (
	"fmt"
	"go-arch/internal/pkg/template"
	"go-arch/internal/ui"
	"os"
	"path/filepath"
)

type Scaffolder struct {
	engine *template.Engine
	config *ui.ProjectConfig
}

func NewScaffolder(config *ui.ProjectConfig) *Scaffolder {
	return &Scaffolder{
		engine: template.NewEngine(),
		config: config,
	}
}

func (s *Scaffolder) Execute() error {
	fmt.Printf("🏗️ Creating project '%s' with %s architecture...\n", s.config.ProjectName, s.config.Architecture)

	// 1. Create base directory
	if err := os.MkdirAll(s.config.ProjectName, 0755); err != nil {
		return err
	}

	// 2. Generate structure according to the layout
	switch s.config.Architecture {
	case "Minimalist":
		return s.scaffoldMinimalist()
	case "Standard":
		return s.scaffoldStandard()
	case "Hexagonal":
		return s.scaffoldHexagonal()
	default:
		return fmt.Errorf("unsupported architecture: %s", s.config.Architecture)
	}
}

func (s *Scaffolder) createFile(path string, templatePath string, data interface{}) error {
	fullPath := filepath.Join(s.config.ProjectName, path)

	// Crear directorios intermedios
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if data == nil {
		data = s.config
	}

	return s.engine.Render(f, templatePath, data)
}

func (s *Scaffolder) scaffoldMinimalist() error {
	// Only main.go and go.mod
	if err := s.createFile("main.go", "minimalist/main.tmpl", nil); err != nil {
		return err
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) scaffoldStandard() error {
	dirs := []string{
		"cmd/api",
		"internal/handler",
		"internal/service",
		"internal/repository",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(s.config.ProjectName, d), 0755); err != nil {
			return err
		}
	}

	if err := s.createFile("cmd/api/main.go", "standard/main.tmpl", nil); err != nil {
		return err
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) scaffoldHexagonal() error {
	dirs := []string{
		"cmd/api",
		"internal/domain",
		"internal/ports",
		"internal/adapters",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(s.config.ProjectName, d), 0755); err != nil {
			return err
		}
	}

	if err := s.createFile("cmd/api/main.go", "hexagonal/main.tmpl", nil); err != nil {
		return err
	}
	return s.createCommonFiles()
}

func (s *Scaffolder) createCommonFiles() error {
	// go.mod, .go-arch.yaml
	if err := s.createFile("go.mod", "common/go.mod.tmpl", nil); err != nil {
		return err
	}
	if err := s.createFile(".go-arch.yaml", "common/config.tmpl", nil); err != nil {
		return err
	}

	// .env (Always useful)
	if err := s.createFile(".env", "common/env.tmpl", nil); err != nil {
		return err
	}

	// Docker Files (Optional)
	if s.config.UseDocker {
		if err := s.createFile("Dockerfile", "common/Dockerfile.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("docker-compose.yaml", "common/docker-compose.yaml.tmpl", nil); err != nil {
			return err
		}
	}

	// Observability (Optional)
	if s.config.UseObservability {
		if err := s.createFile("internal/telemetry/telemetry.go", "common/telemetry.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("internal/telemetry/middleware.go", "common/telemetry_middleware.tmpl", nil); err != nil {
			return err
		}
	}

	// gRPC / Microservices (Optional)
	if s.config.UseGRPC {
		if err := s.createFile("api/proto/service.proto", "common/service.proto.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("internal/adapters/grpc/server.go", "common/grpc_server.tmpl", nil); err != nil {
			return err
		}
		if err := s.createFile("Makefile", "common/Makefile.tmpl", nil); err != nil {
			return err
		}
	}

	return nil
}

// GenerateComponent generates a specific component (service, repository, handler)
func (s *Scaffolder) GenerateComponent(compType, name string) error {
	var targetPath string
	var templatePath string

	data := struct {
		ui.ProjectConfig
		EntityName string
	}{
		ProjectConfig: *s.config,
		EntityName:    name,
	}

	switch compType {
	case "service":
		templatePath = "common/service.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/domain", name+"_service.go")
		} else {
			targetPath = filepath.Join("internal/service", name+"_service.go")
		}
	case "repository":
		templatePath = "common/repository.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/ports", name+"_repository.go")
		} else {
			targetPath = filepath.Join("internal/repository", name+"_repository.go")
		}
	case "handler":
		templatePath = "common/handler.tmpl"
		if s.config.Architecture == "Hexagonal" {
			targetPath = filepath.Join("internal/adapters", name+"_handler.go")
		} else {
			targetPath = filepath.Join("internal/handler", name+"_handler.go")
		}
	default:
		return fmt.Errorf("unsupported component type: %s", compType)
	}

	return s.createFile(targetPath, templatePath, data)
}

// GenerateCRUD generates the full structure for a CRUD entity
func (s *Scaffolder) GenerateCRUD(name string) error {
	data := struct {
		ui.ProjectConfig
		EntityName string
	}{
		ProjectConfig: *s.config,
		EntityName:    name,
	}

	fmt.Printf("🚀 Generating full CRUD for '%s'...\n", name)

	var files map[string]string
	if s.config.Architecture == "Hexagonal" {
		files = map[string]string{
			filepath.Join("internal/domain", name+".go"):              "common/model.tmpl",
			filepath.Join("internal/domain", name+"_service.go"):      "common/crud_service.tmpl",
			filepath.Join("internal/ports", name+"_repository.go"):    "common/crud_port.tmpl", // Port interface lives in internal/ports
			filepath.Join("internal/adapters", name+"_repository.go"): "common/crud_repository.tmpl",
			filepath.Join("internal/adapters", name+"_handler.go"):    "common/crud_handler.tmpl",
		}
	} else {
		files = map[string]string{
			filepath.Join("internal/model", name+".go"):                 "common/model.tmpl",
			filepath.Join("internal/service", name+"_service.go"):       "common/crud_service.tmpl",
			filepath.Join("internal/repository", name+"_repository.go"): "common/crud_repository.tmpl",
			filepath.Join("internal/handler", name+"_handler.go"):       "common/crud_handler.tmpl",
		}
	}

	for path, tmpl := range files {
		if err := s.createFile(path, tmpl, data); err != nil {
			return err
		}
	}

	fmt.Println("\n✅ CRUD generated successfully.")
	fmt.Println("📍 Remember to register the routes in your main router.")
	return nil
}
