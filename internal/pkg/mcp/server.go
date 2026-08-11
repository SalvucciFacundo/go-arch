package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/generators"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/hooks"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/packs"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/scaffold"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/validator"
	"github.com/SalvucciFacundo/go-arch/v2/internal/pkg/workspace"
	"github.com/SalvucciFacundo/go-arch/v2/internal/ui"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/samber/oops"
	"github.com/spf13/viper"
)

// Version is the go-arch CLI version, set from main via GoReleaser or explicitly.
// The mcp package uses its own Version to avoid an import cycle with cmd.
var Version = "dev"

// Request representation for JSON-RPC 2.0
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

// Response representation for JSON-RPC 2.0
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error representation for JSON-RPC 2.0
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// StartServer starts the MCP server over stdio
func StartServer() {
	// Redirect UI output to stderr to prevent stdio JSON-RPC corruption
	ui.Out = os.Stderr

	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		handleRequest(&req)
	}
}

func sendResponse(id interface{}, result interface{}) {
	res := Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
	data, err := json.Marshal(res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling response: %v\n", err)
		return
	}
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}

func sendError(id interface{}, code int, message string, data interface{}) {
	res := Response{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
	bytes, _ := json.Marshal(res)
	os.Stdout.Write(bytes)
	os.Stdout.Write([]byte("\n"))
}

func handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		sendResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "go-arch-mcp",
				"version": "1.0.0",
			},
		})
	case "tools/list":
		sendResponse(req.ID, map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{
					"name":        "new_project",
					"description": "Initialize a new Go project with selected clean architecture layout and optional setups like database driver, Docker, Observability, gRPC, and templ+HTMX frontend.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"projectName": map[string]interface{}{
								"type":        "string",
								"description": "Name of the project directory (e.g. my-project)",
							},
							"moduleName": map[string]interface{}{
								"type":        "string",
								"description": "Go module name (e.g. github.com/user/my-project)",
							},
							"architecture": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"Minimalist", "Standard", "Hexagonal"},
								"description": "Clean architecture layout (optional when template is set)",
							},
							"dbDriver": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"PostgreSQL", "MySQL", "MongoDB", "None"},
								"description": "Database driver to pre-configure",
							},
							"useDocker": map[string]interface{}{
								"type":        "boolean",
								"description": "Generate Dockerfile and docker-compose.yaml",
							},
							"useObservability": map[string]interface{}{
								"type":        "boolean",
								"description": "Enable OpenTelemetry and telemetry middleware",
							},
							"observabilityBackend": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"Console", "Jaeger", "Zipkin", "Prometheus", "SigNoz"},
								"description": "Observability visualization backend",
							},
							"useGRPC": map[string]interface{}{
								"type":        "boolean",
								"description": "Enable gRPC and Protocol Buffers support",
							},
							"useTemplHTMX": map[string]interface{}{
								"type":        "boolean",
								"description": "Generate a server-rendered templ + HTMX frontend (views, static assets, web-aware main)",
							},
							"template": map[string]interface{}{
								"type":        "string",
								"description": "Installed template pack name (e.g. 'express' or 'express@1.0.0'). When set, architecture is optional.",
							},
						},
						"required": []string{"projectName", "moduleName"},
					},
				},
				map[string]interface{}{
					"name":        "list_generators",
					"description": "List available generators for the current project: pack generators (if installed), builtin generators, and component types.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml if not running in the current directory",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "generate_component",
					"description": "Generate components using pack generators, builtin generators, or standard component types (service, repository, handler, crud, page, component) for the project.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Type of the component to generate or generator name (pack/builtin/component type)",
							},
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Name of the entity or component (e.g. User, Product)",
							},
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml if not running in the current directory",
							},
							"route": map[string]interface{}{
								"type":        "string",
								"description": "Route pattern for handler type (e.g. 'GET /stats'). Ignored for other types.",
							},
							"generatorArgs": map[string]interface{}{
								"type":        "object",
								"description": "Optional: Arguments for pack generator prompt resolution (e.g. {\"port\": \"3000\"})",
							},
						},
						"required": []string{"type", "name"},
					},
				},
				map[string]interface{}{
					"name":        "check_architecture",
					"description": "Run architectural rules checks, validating import rules and package directory structures.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml if not running in the current directory",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "serve_project",
					"description": "Check how to run the project: validates the go-arch config, detects whether air (hot-reload) is installed, and returns the exact command to run (air, or go run with the architecture-specific main path). Never starts a long-running process.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml if not running in the current directory",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "setup_environment",
					"description": "Detect the Go development environment: OS/architecture, whether the go binary is installed, and whether air (hot-reload) is installed. When install is true it installs the missing air binary via go install (no sudo, user-level only); it never installs the Go toolchain itself (that requires sudo and stays a manual decision).",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"install": map[string]interface{}{
								"type":        "boolean",
								"description": "When true, install the missing air binary with go install. Default false: only detect and return the exact install commands.",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "upgrade_project",
					"description": "Propagate embedded template changes to a previously generated project. Returns a classified plan (upgradable / protected / absent). Dry-run by default — mutates nothing. Set apply: true to commit changes. When service is set, upgrades that service from its workspace (chdir-free via root injection).",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml",
							},
							"service": map[string]interface{}{
								"type":        "string",
								"description": "Optional: upgrade only this service from the workspace. Requires workspacePath or a discoverable workspace.",
							},
							"workspacePath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: path to go-arch.workspace.yaml (used with service).",
							},
							"apply": map[string]interface{}{
								"type":        "boolean",
								"description": "When true, apply all upgradable changes and return the applied plan. Default: false.",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "install_template",
					"description": "Install a template pack from a Go module (e.g. 'github.com/user/go-arch-express' or 'github.com/user/go-arch-express@v1.0.0'). Fetches via the Go module proxy, validates the pack contract, and installs it under ~/.go-arch/packs/<name>@<version>/. If the pack declares hooks or generators that run commands, they are disabled unless allowHooks is true.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"module": map[string]interface{}{
								"type":        "string",
								"description": "Go module path of the pack (e.g. github.com/user/go-arch-express). Optional @version suffix.",
							},
							"allowHooks": map[string]interface{}{
								"type":        "boolean",
								"description": "When true, enable hooks/generators that run shell commands from this pack. Default false (safest).",
							},
						},
						"required": []string{"module"},
					},
				},
				map[string]interface{}{
					"name":        "list_packs",
					"description": "List installed template packs with their versions. Returns an empty list when no packs are installed.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
				map[string]interface{}{
					"name":        "remove_pack",
					"description": "Remove an installed template pack. Without @version, the latest installed version is removed. With @version, only that specific version is removed.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Pack name, optionally with @version (e.g. 'express' or 'express@1.0.0').",
							},
						},
						"required": []string{"name"},
					},
				},
				map[string]interface{}{
					"name":        "update_pack",
					"description": "Update an installed template pack to its latest version. Re-fetches the module recorded in the pack's sidecar. If the pack declares hooks/generators that run commands, they are disabled unless allowHooks is true.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Pack name to update (e.g. 'express').",
							},
							"allowHooks": map[string]interface{}{
								"type":        "boolean",
								"description": "When true, enable hooks/generators that run shell commands after the update. Default false (safest).",
							},
						},
						"required": []string{"name"},
					},
				},
				map[string]interface{}{
					"name":        "workspace_list",
					"description": "List the services in a go-arch.workspace.yaml (name, path, template). Requires a workspacePath or a discoverable workspace from the current directory.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"workspacePath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: path to go-arch.workspace.yaml. Default: discover upward from the current directory.",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "workspace_upgrade",
					"description": "Upgrade services in a workspace (all by default, or a single service via the service param). Dry-run by default; set apply: true to commit changes. Chdir-free — resolves each service root and injects it.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"workspacePath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: path to go-arch.workspace.yaml. Default: discover upward.",
							},
							"service": map[string]interface{}{
								"type":        "string",
								"description": "Optional: upgrade only this service. Default: all services.",
							},
							"apply": map[string]interface{}{
								"type":        "boolean",
								"description": "When true, apply all upgradable changes per service. Default false (dry-run).",
							},
						},
					},
				},
				map[string]interface{}{
					"name":        "workspace_check",
					"description": "Run the architecture check for services in a workspace (all by default, or a single service via the service param).",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"workspacePath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: path to go-arch.workspace.yaml. Default: discover upward.",
							},
							"service": map[string]interface{}{
								"type":        "string",
								"description": "Optional: check only this service. Default: all services.",
							},
						},
					},
				},
			},
		})
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendError(req.ID, -32602, "Invalid params", err.Error())
			return
		}
		handleToolCall(req.ID, params.Name, params.Arguments)
	default:
		sendError(req.ID, -32601, "Method not found", nil)
	}
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolCallResponse struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError"`
}

func handleToolCall(id interface{}, name string, arguments json.RawMessage) {
	switch name {
	case "new_project":
		var args struct {
			ProjectName          string `json:"projectName"`
			ModuleName           string `json:"moduleName"`
			Architecture         string `json:"architecture"`
			DBDriver             string `json:"dbDriver"`
			UseDocker            bool   `json:"useDocker"`
			UseObservability     bool   `json:"useObservability"`
			ObservabilityBackend string `json:"observabilityBackend"`
			UseGRPC              bool   `json:"useGRPC"`
			UseTemplHTMX         bool   `json:"useTemplHTMX"`
			Template             string `json:"template"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.DBDriver == "" {
			args.DBDriver = "None"
		}

		// P3: architecture is optional when template is present.
		if args.Template == "" && args.Architecture == "" {
			sendToolResult(id, "architecture is required when template is not set", true)
			return
		}

		var packInfo *packs.PackInfo
		if args.Template != "" {
			name, version, err := packs.ParseRef(args.Template)
			if err != nil {
				sendToolResult(id, fmt.Sprintf("Invalid pack ref: %v", err), true)
				return
			}
			if err := packs.ValidateSlug(name); err != nil {
				sendToolResult(id, fmt.Sprintf("Invalid pack name: %v", err), true)
				return
			}
			if version == "" {
				latest, lErr := packs.LatestInstalled(name)
				if lErr != nil {
					sendToolResult(id, fmt.Sprintf("Pack %q is not installed", name), true)
					return
				}
				version = latest
			}
			dir := packs.Path(name, version)
			m, mErr := packs.Load(dir)
			if mErr != nil {
				sendToolResult(id, fmt.Sprintf("Failed to load pack %q: %v", name, mErr), true)
				return
			}

			// G4: empty-templates check before scaffold
			templatesDir := filepath.Join(dir, "templates")
			entries, readErr := os.ReadDir(templatesDir)
			if readErr != nil || len(entries) == 0 {
				sendToolResult(id, fmt.Sprintf("Pack %q has no templates", name), true)
				return
			}

			pi := packs.PackInfo{Dir: dir, Manifest: m}
			packInfo = &pi
		}

		cfg := &ui.ProjectConfig{
			ProjectName:          args.ProjectName,
			ModuleName:           args.ModuleName,
			Architecture:         args.Architecture,
			DBDriver:             args.DBDriver,
			UseDocker:            args.UseDocker,
			UseObservability:     args.UseObservability,
			ObservabilityBackend: args.ObservabilityBackend,
			UseGRPC:              args.UseGRPC,
			UseTemplHTMX:         args.UseTemplHTMX,
		}
		if packInfo != nil {
			cfg.Template = packInfo.Manifest.Name
		}

		hooksCfg, hErr := hooks.Load(hooks.ResolveConfigPath())
		if hErr != nil {
			sendToolResult(id, fmt.Sprintf("Failed to load hooks config: %v", hErr), true)
			return
		}

		// Pack hooks: honor the sidecar's HooksEnabled flag set at install time.
		if packInfo != nil && len(packInfo.Manifest.Hooks) > 0 {
			sc, scErr := packs.ReadSidecar(packInfo.Dir)
			if scErr == nil && sc.HooksEnabled {
				for hookType, entries := range packInfo.Manifest.Hooks {
					hooksCfg.Hooks[hookType] = append(hooksCfg.Hooks[hookType], entries...)
				}
			}
		}
		runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)

		var scaffOpts []scaffold.ScaffoldOption
		scaffOpts = append(scaffOpts, scaffold.WithRunner(runner), scaffold.WithVersion(Version))
		if packInfo != nil {
			scaffOpts = append(scaffOpts, scaffold.WithPackInfo(*packInfo))
		}
		scaffolder := scaffold.NewScaffolder(cfg, scaffOpts...)
		if err := scaffolder.Execute(); err != nil {
			sendToolResult(id, fmt.Sprintf("Error building project: %v", err), true)
			return
		}
		sendToolResult(id, fmt.Sprintf("Successfully scaffolded project %s at directory ./%s", args.ProjectName, args.ProjectName), false)

	case "generate_component":
		var args struct {
			Type          string                 `json:"type"`
			Name          string                 `json:"name"`
			ProjectPath   string                 `json:"projectPath"`
			Route         string                 `json:"route"`
			GeneratorArgs map[string]interface{} `json:"generatorArgs"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
					sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
					return
				}
				defer func() { _ = os.Chdir(oldWd) }()
			}
		}

		viper.Reset()
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
		if err := viper.ReadInConfig(); err != nil {
			sendToolResult(id, fmt.Sprintf("Could not read .go-arch.yaml config. Are you in a go-arch project? Error: %v", err), true)
			return
		}

		cfg := &ui.ProjectConfig{
			ProjectName:  viper.GetString("project_name"),
			ModuleName:   viper.GetString("module_name"),
			Architecture: viper.GetString("architecture"),
			DBDriver:     viper.GetString("db_driver"),
			UseDocker:    viper.GetBool("use_docker"),
			UseTemplHTMX: viper.GetBool("use_templ_htmx"),
		}

		hooksCfg, hErr := hooks.Load(hooks.ResolveConfigPath())
		if hErr != nil {
			sendToolResult(id, fmt.Sprintf("Failed to load hooks config: %v", hErr), true)
			return
		}
		runner := hooks.NewRunner(hooksCfg, hooks.RealRunner{}, ui.Out)

		// --- Three-tier dispatch ---
		// Tier 1: pack generators (if project has a template).
		templateName := viper.GetString("template")
		packResolved := false
		if templateName != "" {
			packName, packVersion, parseErr := packs.ParseRef(templateName)
			if parseErr == nil {
				if packVersion == "" {
					latest, lErr := packs.LatestInstalled(packName)
					if lErr == nil {
						packVersion = latest
					}
				}
				if packVersion != "" {
					packDir := packs.Path(packName, packVersion)
					packManifest, mErr := packs.Load(packDir)
					if mErr == nil {
						packResolved = true
						if _, ok := packManifest.Generators[args.Type]; ok {
							cfg.Template = packName
							pi := packs.PackInfo{Dir: packDir, Manifest: packManifest}

							// Convert generatorArgs to map[string]any.
							genArgs := make(map[string]any)
							for k, v := range args.GeneratorArgs {
								genArgs[k] = v
							}

							scaffolder := scaffold.NewScaffolder(cfg,
								scaffold.WithRunner(runner),
								scaffold.WithPackInfo(pi),
							)
							if genErr := scaffolder.GeneratePackGenerator(args.Type, genArgs,
								scaffold.WithPromptErrorCode(generators.CodeMissingGeneratorArgument),
							); genErr != nil {
								sendToolResult(id, formatMCGeneratorError(genErr), true)
								return
							}
							sendToolResult(id, fmt.Sprintf("Generator '%s' (%s) from pack '%s' completed.", args.Name, args.Type, packName), false)
							return
						}
					}
				}
			}
		}

		// Tier 2 & 3: component types.
		// If template was set but the pack was NOT successfully resolved,
		// and the type is not a known component type, emit pack_not_installed.
		if templateName != "" && !packResolved && !isMCKnownComponentType(args.Type) {
			sendToolResult(id, fmt.Sprintf(
				"pack_not_installed: pack %q is not installed. Run 'go-arch template install' to install it.",
				templateName,
			), true)
			return
		}

		if !isMCKnownComponentType(args.Type) {
			msg := fmt.Sprintf("unknown_generator: unknown generator %q. Component types: service, repository, handler, crud, page, component.", args.Type)
			// If a pack is installed, include its available generators.
			if packResolved && templateName != "" {
				packName, packVersion, _ := packs.ParseRef(templateName)
				if packVersion == "" {
					latest, _ := packs.LatestInstalled(packName)
					if latest != "" {
						packVersion = latest
					}
				}
				if packVersion != "" {
					packDir := packs.Path(packName, packVersion)
					packManifest, mErr := packs.Load(packDir)
					if mErr == nil && len(packManifest.Generators) > 0 {
						names := make([]string, 0, len(packManifest.Generators))
						for n := range packManifest.Generators {
							names = append(names, n)
						}
						sort.Strings(names)
						msg = fmt.Sprintf("unknown_generator: unknown generator %q. Pack generators (%s): %s. Component types: service, repository, handler, crud, page, component.",
							args.Type, packName, strings.Join(names, ", "))
					}
				}
			}
			sendToolResult(id, msg, true)
			return
		}

		scaffolder := scaffold.NewScaffolder(cfg, scaffold.WithRunner(runner))
		var err error
		if args.Type == "crud" {
			err = scaffolder.GenerateCRUD(args.Name)
		} else {
			var opts []scaffold.GenerateOption
			if args.Route != "" {
				opts = append(opts, scaffold.WithRoute(args.Route))
			}
			err = scaffolder.GenerateComponent(args.Type, args.Name, opts...)
		}

		if err != nil {
			sendToolResult(id, fmt.Sprintf("Error generating component: %v", err), true)
			return
		}
		sendToolResult(id, fmt.Sprintf("Successfully generated %s component: %s", args.Type, args.Name), false)

	case "list_generators":
		var args struct {
			ProjectPath string `json:"projectPath"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
					sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
					return
				}
				defer func() { _ = os.Chdir(oldWd) }()
			}
		}

		viper.Reset()
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
		_ = viper.ReadInConfig() // best-effort; missing config handled per section

		// Build the response: component types + pack generators (if installed) +
		// builtin generators.
		type GeneratorInfo struct {
			Name        string `json:"name"`
			Source      string `json:"source"`
			Description string `json:"description,omitempty"`
		}
		var genList []GeneratorInfo

		// Component types (always available).
		for _, t := range []string{"service", "repository", "handler", "crud", "page", "component"} {
			genList = append(genList, GeneratorInfo{
				Name:   t,
				Source: "builtin-component",
			})
		}

		// Pack generators.
		templateName := viper.GetString("template")
		if templateName != "" {
			packName, packVersion, parseErr := packs.ParseRef(templateName)
			if parseErr == nil {
				if packVersion == "" {
					latest, lErr := packs.LatestInstalled(packName)
					if lErr == nil {
						packVersion = latest
					}
					if packVersion == "" {
						goto builtins
					}
				}
				packDir := packs.Path(packName, packVersion)
				packManifest, mErr := packs.Load(packDir)
				if mErr == nil {
					for name, gen := range packManifest.Generators {
						genList = append(genList, GeneratorInfo{
							Name:        name,
							Source:      fmt.Sprintf("pack:%s", packName),
							Description: gen.Description,
						})
					}
				}
			}
		}

	builtins:
		// Builtin generators.
		if len(generators.BuiltinRegistry) > 0 {
			for name := range generators.BuiltinRegistry {
				genList = append(genList, GeneratorInfo{
					Name:   name,
					Source: "builtin",
				})
			}
		}

		result, _ := json.MarshalIndent(map[string]interface{}{
			"generators": genList,
		}, "", "  ")
		sendToolResult(id, string(result), false)

	case "check_architecture":
		var args struct {
			ProjectPath string `json:"projectPath"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
					sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
					return
				}
				defer func() { _ = os.Chdir(oldWd) }()
			}
		}

		viper.Reset()
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
		if err := viper.ReadInConfig(); err != nil {
			sendToolResult(id, fmt.Sprintf("Could not read .go-arch.yaml config. Error: %v", err), true)
			return
		}

		cfg := &ui.ProjectConfig{
			ProjectName:  viper.GetString("project_name"),
			ModuleName:   viper.GetString("module_name"),
			Architecture: viper.GetString("architecture"),
		}

		v := validator.NewValidator(cfg)
		violations, err := v.Validate()
		if err != nil {
			sendToolResult(id, fmt.Sprintf("Error executing validator: %v", err), true)
			return
		}

		if len(violations) == 0 {
			sendToolResult(id, "Clean architecture! No violations found.", false)
			return
		}

		var b []byte
		b, err = json.MarshalIndent(violations, "", "  ")
		if err != nil {
			sendToolResult(id, fmt.Sprintf("Found %d violations, but failed to format them.", len(violations)), true)
			return
		}
		sendToolResult(id, string(b), false)

	case "serve_project":
		var args struct {
			ProjectPath string `json:"projectPath"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
					sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
					return
				}
				defer func() { _ = os.Chdir(oldWd) }()
			}
		}

		viper.Reset()
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
		if err := viper.ReadInConfig(); err != nil {
			sendToolResult(id, fmt.Sprintf("Could not read .go-arch.yaml config. Are you in a go-arch project? Error: %v", err), true)
			return
		}

		layout := viper.GetString("architecture")
		if layout == "" {
			sendToolResult(id, "No valid architecture configuration found. Are you in the root of a go-arch project?", true)
			return
		}

		mainPath := "cmd/api/main.go"
		if layout == "Minimalist" {
			mainPath = "main.go"
		}

		var command string
		var hotReload bool
		if _, err := exec.LookPath("air"); err == nil {
			command = "air"
			hotReload = true
		} else {
			command = "go run " + mainPath
		}

		result := map[string]interface{}{
			"architecture": layout,
			"mainPath":     mainPath,
			"command":      command,
			"hotReload":    hotReload,
			"note":         "MCP tools never start a long-running server. Run this command yourself in a terminal.",
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		sendToolResult(id, string(data), false)

	case "setup_environment":
		var args struct {
			Install bool `json:"install"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		goInstalled := false
		if _, err := exec.LookPath("go"); err == nil {
			goInstalled = true
		}
		airInstalled := false
		if _, err := exec.LookPath("air"); err == nil {
			airInstalled = true
		}

		airInstallCommand := "go install github.com/air-verse/air@latest"
		var goInstallCommand string
		switch runtime.GOOS {
		case "linux":
			goInstallCommand = "Download the official tarball from https://go.dev/dl and run: sudo tar -C /usr/local -xzf go<VERSION>.linux-" + runtime.GOARCH + ".tar.gz"
		case "windows":
			goInstallCommand = "Download and run the official MSI from https://go.dev/dl"
		default:
			goInstallCommand = "Download the official Go installer from https://go.dev/dl"
		}

		result := map[string]interface{}{
			"os":                runtime.GOOS,
			"arch":              runtime.GOARCH,
			"goInstalled":       goInstalled,
			"airInstalled":      airInstalled,
			"goInstallCommand":  goInstallCommand,
			"airInstallCommand": airInstallCommand,
		}

		if args.Install && !airInstalled {
			cmd := exec.Command("go", "install", "github.com/air-verse/air@latest")
			out, err := cmd.CombinedOutput()
			if err != nil {
				result["installStatus"] = "failed"
				result["installOutput"] = fmt.Sprintf("go install failed: %v — output: %s", err, string(out))
				result["installCommand"] = airInstallCommand
			} else {
				result["installStatus"] = "installed"
				result["installOutput"] = string(out)
				result["airInstalled"] = true
			}
		} else if args.Install && airInstalled {
			result["installStatus"] = "already_installed"
		}

		if !goInstalled {
			result["goInstallNote"] = "The Go toolchain itself is never installed by this tool (requires sudo and is a manual decision). " + goInstallCommand
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		sendToolResult(id, string(data), false)

	case "upgrade_project":
		var args struct {
			ProjectPath   string `json:"projectPath"`
			Service       string `json:"service"`
			WorkspacePath string `json:"workspacePath"`
			Apply         bool   `json:"apply"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		// Workspace service upgrade: chdir-free via WithRoot injection.
		if args.Service != "" {
			ws, err := resolveMCWorkspace(args.WorkspacePath)
			if err != nil {
				sendToolResult(id, toolResultError(err), true)
				return
			}
			handleMCWorkspaceUpgrade(id, ws, args.Service, args.Apply)
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				if chdirErr := os.Chdir(args.ProjectPath); chdirErr != nil {
					sendError(id, -32602, "Cannot change to project directory", chdirErr.Error())
					return
				}
				defer func() { _ = os.Chdir(oldWd) }()
			}
		}

		viper.Reset()
		viper.AddConfigPath(".")
		viper.SetConfigName(".go-arch")
		if err := viper.ReadInConfig(); err != nil {
			sendToolResult(id, fmt.Sprintf("Could not read .go-arch.yaml config. Error: %v", err), true)
			return
		}

		cfg := &ui.ProjectConfig{
			ProjectName:          viper.GetString("project_name"),
			ModuleName:           viper.GetString("module_name"),
			Architecture:         viper.GetString("architecture"),
			DBDriver:             viper.GetString("db_driver"),
			UseDocker:            viper.GetBool("use_docker"),
			UseObservability:     viper.GetBool("use_observability"),
			ObservabilityBackend: viper.GetString("observability_backend"),
			UseGRPC:              viper.GetBool("use_grpc"),
			UseTemplHTMX:         viper.GetBool("use_templ_htmx"),
		}

		plan, err := scaffold.Upgrade(cfg, scaffold.WithResolver(scaffold.DefaultResolver{}))
		if err != nil {
			sendToolResult(id, fmt.Sprintf("Upgrade failed: %v", err), true)
			return
		}

		if args.Apply {
			applied, applyErr := plan.Apply()
			if applyErr != nil {
				sendToolResult(id, fmt.Sprintf("Apply failed: %v", applyErr), true)
				return
			}
			_ = scaffold.WriteVersionField(".go-arch.yaml", Version)
			plan.AppliedCount = applied
		}

		result, _ := json.MarshalIndent(plan, "", "  ")
		sendToolResult(id, string(result), false)

	case "install_template":
		var args struct {
			Module     string `json:"module"`
			AllowHooks bool   `json:"allowHooks"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		handleInstallTemplate(id, args.Module, args.AllowHooks, packs.RealDownloader{})

	case "list_packs":
		packsList, err := packs.List()
		if err != nil {
			sendToolResult(id, fmt.Sprintf("Failed to list packs: %v", err), true)
			return
		}

		type PackInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		var out []PackInfo
		for _, p := range packsList {
			out = append(out, PackInfo{Name: p.Name, Version: p.Version})
		}
		if out == nil {
			out = []PackInfo{} // JSON [] not null
		}
		result, _ := json.MarshalIndent(out, "", "  ")
		sendToolResult(id, string(result), false)

	case "remove_pack":
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		if args.Name == "" {
			sendError(id, -32602, "Missing required argument", "name is required")
			return
		}
		handleRemovePack(id, args.Name)

	case "update_pack":
		var args struct {
			Name       string `json:"name"`
			AllowHooks bool   `json:"allowHooks"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		handleUpdatePack(id, args.Name, args.AllowHooks, packs.RealDownloader{})

	case "workspace_list":
		var args struct {
			WorkspacePath string `json:"workspacePath"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		ws, err := resolveMCWorkspace(args.WorkspacePath)
		if err != nil {
			sendToolResult(id, toolResultError(err), true)
			return
		}
		type svcInfo struct {
			Name     string `json:"name"`
			Path     string `json:"path"`
			Template string `json:"template,omitempty"`
		}
		var out []svcInfo
		for _, s := range ws.Services {
			out = append(out, svcInfo{Name: s.Name, Path: s.Path, Template: s.Template})
		}
		if out == nil {
			out = []svcInfo{}
		}
		result, _ := json.MarshalIndent(out, "", "  ")
		sendToolResult(id, string(result), false)

	case "workspace_upgrade":
		var args struct {
			WorkspacePath string `json:"workspacePath"`
			Service       string `json:"service"`
			Apply         bool   `json:"apply"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		ws, err := resolveMCWorkspace(args.WorkspacePath)
		if err != nil {
			sendToolResult(id, toolResultError(err), true)
			return
		}
		handleMCWorkspaceUpgrade(id, ws, args.Service, args.Apply)

	case "workspace_check":
		var args struct {
			WorkspacePath string `json:"workspacePath"`
			Service       string `json:"service"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}
		ws, err := resolveMCWorkspace(args.WorkspacePath)
		if err != nil {
			sendToolResult(id, toolResultError(err), true)
			return
		}
		handleMCWorkspaceCheck(id, ws, args.Service)

	default:
		sendError(id, -32601, "Tool not found", nil)
	}
}

// handleRemovePack removes an installed pack. Bare names resolve to the latest
// installed version (mirrors the CLI template remove command).
func handleRemovePack(id interface{}, ref string) {
	name, version, err := packs.ParseRef(ref)
	if err != nil {
		sendToolResult(id, fmt.Sprintf("Invalid pack reference %q: %v", ref, err), true)
		return
	}
	if version == "" {
		latest, latestErr := packs.LatestInstalled(name)
		if latestErr != nil {
			sendToolResult(id, fmt.Sprintf("Failed to resolve latest version: %v", latestErr), true)
			return
		}
		version = latest
	}
	if err := packs.Remove(name, version); err != nil {
		sendToolResult(id, fmt.Sprintf("Failed to remove pack: %v", err), true)
		return
	}
	sendToolResult(id, fmt.Sprintf("✅ Pack %q (v%s) removed.", name, version), false)
}

// handleUpdatePack updates an installed pack. The downloader is injectable for
// tests (FakeDownloader); production uses RealDownloader. allowHooks is the
// explicit agent decision — MCP is non-interactive (no trust prompt).
func handleUpdatePack(id interface{}, name string, allowHooks bool, dl packs.Downloader) {
	if name == "" {
		sendError(id, -32602, "Missing required argument", "name is required")
		return
	}
	confirm := func(packName string) (bool, error) {
		return allowHooks, nil
	}
	m, err := packs.Update(context.Background(), dl, name, confirm)
	if err != nil {
		sendToolResult(id, fmt.Sprintf("Failed to update pack: %v", err), true)
		return
	}
	status := "updated with hooks disabled"
	if allowHooks {
		status = "updated with hooks enabled"
	}
	sendToolResult(id, fmt.Sprintf("✅ Pack %q (v%s) %s.", m.Name, m.Version, status), false)
}

// handleInstallTemplate installs a template pack via MCP. The downloader is
// injectable so tests can avoid the network (FakeDownloader); production uses
// RealDownloader. MCP is non-interactive: allowHooks is the explicit agent
// decision; without it, hooks/generators that run commands stay disabled
// (equivalent to declining the CLI trust prompt).
func handleInstallTemplate(id interface{}, moduleRef string, allowHooks bool, dl packs.Downloader) {
	if moduleRef == "" {
		sendError(id, -32602, "Missing required argument", "module is required")
		return
	}

	module, version, err := packs.ParseRef(moduleRef)
	if err != nil {
		sendToolResult(id, fmt.Sprintf("Invalid pack reference %q: %v", moduleRef, err), true)
		return
	}
	if version == "" {
		version = "latest"
	}

	confirm := func(packName string) (bool, error) {
		return allowHooks, nil
	}

	m, err := packs.Install(context.Background(), dl, module, version, confirm)
	if err != nil {
		sendToolResult(id, fmt.Sprintf("Pack install failed: %v", err), true)
		return
	}

	status := "installed with hooks disabled"
	if allowHooks {
		status = "installed with hooks enabled"
	}
	result := fmt.Sprintf("✅ Pack %q (v%s) %s.", m.Name, m.Version, status)
	sendToolResult(id, result, false)
}

func sendToolResult(id interface{}, text string, isError bool) {
	sendResponse(id, ToolCallResponse{
		Content: []TextContent{
			{
				Type: "text",
				Text: text,
			},
		},
		IsError: isError,
	})
}

// isMCKnownComponentType returns true if t is a built-in component type.
func isMCKnownComponentType(t string) bool {
	switch t {
	case "service", "repository", "handler", "crud", "page", "component":
		return true
	}
	return false
}

// formatMCGeneratorError formats a generator error for MCP tool results,
// prepending the oops error code when available.
func formatMCGeneratorError(err error) string {
	if err == nil {
		return "unknown generator error"
	}
	var oErr oops.OopsError
	if errors.As(err, &oErr) {
		if code, ok := oErr.Code().(string); ok && code != "" {
			return fmt.Sprintf("%s: %v", code, err)
		}
	}
	return fmt.Sprintf("Error executing pack generator: %v", err)
}

// ---------------------------------------------------------------------------
// Workspace MCP helpers
// ---------------------------------------------------------------------------

// resolveMCWorkspace returns the workspace for a request: an explicit
// workspacePath param wins, otherwise upward discovery from the MCP server's
// CWD. This mirrors the CLI's resolveWorkspace but lives in mcp to avoid the
// cmd→mcp import cycle.
func resolveMCWorkspace(workspacePath string) (*workspace.Workspace, error) {
	if workspacePath != "" {
		return workspace.Load(workspacePath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, oops.Code("workspace_invalid").Wrapf(err, "cannot resolve working directory")
	}
	path, err := workspace.Discover(cwd)
	if err != nil {
		return nil, err
	}
	return workspace.Load(path)
}

// findMCService returns the named service from the workspace.
func findMCService(ws *workspace.Workspace, name string) (*workspace.Service, error) {
	svc, ok := ws.Find(name)
	if !ok {
		return nil, oops.
			Code("service_not_found").
			Errorf("service %q not found in workspace", name)
	}
	return svc, nil
}

// toolResultError formats an error as the structured tool-result body
// {error: {code, message}}. Business errors flow INSIDE the content JSON,
// not as JSON-RPC -326xx errors.
func toolResultError(err error) string {
	code := ""
	var oErr oops.OopsError
	if errors.As(err, &oErr) {
		if c, ok := oErr.Code().(string); ok {
			code = c
		}
	}
	type errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	body := map[string]interface{}{
		"error": errBody{Code: code, Message: err.Error()},
	}
	data, _ := json.MarshalIndent(body, "", "  ")
	return string(data)
}

// upgradeMCService upgrades one service chdir-free via WithRoot injection.
// On missing manifest (legacy): returns {status:"skipped", error:{code:
// "service_no_manifest"}} — the batch continues. NOTE: the CLI FAILS here
// (upgradeOneService returns service_no_manifest → non-zero exit); the MCP
// spec DELIBERATELY diverges — a missing manifest is a non-fatal skip so the
// batch continues (workspace-no-manifest requirement).
func upgradeMCService(ws *workspace.Workspace, svc *workspace.Service, apply bool) (map[string]any, error) {
	root := ws.ResolvePath(svc)
	if _, err := os.Stat(root); err != nil || !isDir(root) {
		return nil, oops.
			Code("service_path_missing").
			Errorf("service %q path %s does not exist", svc.Name, root)
	}

	viper.Reset()
	viper.SetConfigFile(filepath.Join(root, ".go-arch.yaml"))
	_ = viper.ReadInConfig() // best-effort: missing config = legacy service

	projectName := viper.GetString("project_name")
	if projectName == "" {
		return map[string]any{
			"name":   svc.Name,
			"status": "skipped",
			"error": map[string]string{
				"code":    "service_no_manifest",
				"message": fmt.Sprintf("service %q has no manifest", svc.Name),
			},
		}, nil
	}

	cfg := &ui.ProjectConfig{
		ProjectName:  projectName,
		ModuleName:   viper.GetString("module_name"),
		Architecture: viper.GetString("architecture"),
		DBDriver:     viper.GetString("db_driver"),
		UseDocker:    viper.GetBool("use_docker"),
		UseTemplHTMX: viper.GetBool("use_templ_htmx"),
	}

	plan, err := scaffold.Upgrade(cfg,
		scaffold.WithRoot(root),
		scaffold.WithResolver(scaffold.DefaultResolver{}),
	)
	if err != nil {
		return nil, oops.
			Code("workspace_upgrade_failed").
			Wrapf(err, "service %q upgrade classification failed", svc.Name)
	}

	entry := map[string]any{
		"name":          svc.Name,
		"status":        "success",
		"upgradable":    plan.CountBy(scaffold.ClassUpgradable),
		"protected":     plan.CountBy(scaffold.ClassProtected),
		"absent":        plan.CountBy(scaffold.ClassAbsent),
		"files_changed": 0,
	}

	if apply {
		if plan.CountBy(scaffold.ClassUpgradable) == 0 {
			return entry, nil
		}
		applied, applyErr := plan.Apply()
		if applyErr != nil {
			return nil, oops.
				Code("workspace_upgrade_failed").
				Wrapf(applyErr, "service %q apply failed", svc.Name)
		}
		_ = scaffold.WriteVersionField(filepath.Join(root, ".go-arch.yaml"), Version)
		entry["files_changed"] = applied
	}

	return entry, nil
}

// checkMCService runs the architecture check for one service (chdir+defer —
// the validator walks "internal" relative to CWD). On missing manifest:
// returns {status:"failed", error:{code:"service_no_manifest"}} — mirrors
// cmd/check.go which errors on empty project_name.
func checkMCService(ws *workspace.Workspace, svc *workspace.Service) (map[string]any, error) {
	root := ws.ResolvePath(svc)
	if _, err := os.Stat(root); err != nil || !isDir(root) {
		return nil, oops.
			Code("service_path_missing").
			Errorf("service %q path %s does not exist", svc.Name, root)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		return nil, oops.Code("workspace_invalid").Wrap(err)
	}
	if err := os.Chdir(root); err != nil {
		return nil, oops.Code("workspace_invalid").Wrapf(err, "cannot enter %s", root)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	viper.Reset()
	viper.AddConfigPath(".")
	viper.SetConfigName(".go-arch")
	_ = viper.ReadInConfig() // best-effort

	projectName := viper.GetString("project_name")
	if projectName == "" {
		return map[string]any{
			"name":   svc.Name,
			"status": "failed",
			"error": map[string]string{
				"code":    "service_no_manifest",
				"message": fmt.Sprintf("service %q has no manifest", svc.Name),
			},
		}, nil
	}

	cfg := &ui.ProjectConfig{
		ProjectName:  projectName,
		ModuleName:   viper.GetString("module_name"),
		Architecture: viper.GetString("architecture"),
	}

	v := validator.NewValidator(cfg)
	violations, err := v.Validate()
	if err != nil {
		return nil, oops.
			Code("workspace_check_failed").
			Wrapf(err, "service %q check failed", svc.Name)
	}

	return map[string]any{
		"name":       svc.Name,
		"status":     "success",
		"violations": len(violations),
	}, nil
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// handleMCWorkspaceUpgrade runs the upgrade over all services (or one) and
// sends a structured per-service result. Continue-on-error.
func handleMCWorkspaceUpgrade(id interface{}, ws *workspace.Workspace, serviceName string, apply bool) {
	services := ws.Services
	if serviceName != "" {
		svc, err := findMCService(ws, serviceName)
		if err != nil {
			sendToolResult(id, toolResultError(err), true)
			return
		}
		services = []workspace.Service{*svc}
	}

	results := []map[string]any{}
	failed := 0
	for _, svc := range services {
		entry, err := upgradeMCService(ws, &svc, apply)
		if err != nil {
			failed++
			results = append(results, map[string]any{
				"name":   svc.Name,
				"status": "failed",
				"error":  toolResultErrorBody(err),
			})
			continue
		}
		if entry["status"] == "failed" {
			failed++
		}
		results = append(results, entry)
	}

	status := "ok"
	if failed > 0 && len(results) > failed {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}

	resp := map[string]any{
		"status":   status,
		"services": results,
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	sendToolResult(id, string(data), failed > 0)
}

// handleMCWorkspaceCheck runs the architecture check over all services (or
// one) and sends a structured per-service result. Continue-on-error.
func handleMCWorkspaceCheck(id interface{}, ws *workspace.Workspace, serviceName string) {
	services := ws.Services
	if serviceName != "" {
		svc, err := findMCService(ws, serviceName)
		if err != nil {
			sendToolResult(id, toolResultError(err), true)
			return
		}
		services = []workspace.Service{*svc}
	}

	results := []map[string]any{}
	failed := 0
	for _, svc := range services {
		entry, err := checkMCService(ws, &svc)
		if err != nil {
			failed++
			results = append(results, map[string]any{
				"name":   svc.Name,
				"status": "failed",
				"error":  toolResultErrorBody(err),
			})
			continue
		}
		if entry["status"] == "failed" {
			failed++
		}
		results = append(results, entry)
	}

	status := "ok"
	if failed > 0 && len(results) > failed {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}

	resp := map[string]any{
		"status":   status,
		"services": results,
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	sendToolResult(id, string(data), failed > 0)
}

// toolResultErrorBody returns the {code, message} map form of an error.
func toolResultErrorBody(err error) map[string]string {
	code := ""
	var oErr oops.OopsError
	if errors.As(err, &oErr) {
		if c, ok := oErr.Code().(string); ok {
			code = c
		}
	}
	return map[string]string{"code": code, "message": err.Error()}
}
