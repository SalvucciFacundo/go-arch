package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go-arch/internal/pkg/scaffold"
	"go-arch/internal/pkg/validator"
	"go-arch/internal/ui"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/viper"
)

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
								"description": "Clean architecture layout",
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
						},
						"required": []string{"projectName", "moduleName", "architecture"},
					},
				},
				map[string]interface{}{
					"name":        "generate_component",
					"description": "Generate standard components (service, repository, handler, crud, page, or component) for the project.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"type": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"service", "repository", "handler", "crud", "page", "component"},
								"description": "Type of the component to generate",
							},
							"name": map[string]interface{}{
								"type":        "string",
								"description": "Name of the entity or component (e.g. User, Product)",
							},
							"projectPath": map[string]interface{}{
								"type":        "string",
								"description": "Optional: Path to the project root containing .go-arch.yaml if not running in the current directory",
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
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.DBDriver == "" {
			args.DBDriver = "None"
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

		scaffolder := scaffold.NewScaffolder(cfg)
		if err := scaffolder.Execute(); err != nil {
			sendToolResult(id, fmt.Sprintf("Error building project: %v", err), true)
			return
		}
		sendToolResult(id, fmt.Sprintf("Successfully scaffolded project %s at directory ./%s", args.ProjectName, args.ProjectName), false)

	case "generate_component":
		var args struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			ProjectPath string `json:"projectPath"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			sendError(id, -32602, "Invalid tool arguments", err.Error())
			return
		}

		if args.ProjectPath != "" {
			oldWd, err := os.Getwd()
			if err == nil {
				defer os.Chdir(oldWd)
				os.Chdir(args.ProjectPath)
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

		scaffolder := scaffold.NewScaffolder(cfg)
		var err error
		if args.Type == "crud" {
			err = scaffolder.GenerateCRUD(args.Name)
		} else {
			err = scaffolder.GenerateComponent(args.Type, args.Name)
		}

		if err != nil {
			sendToolResult(id, fmt.Sprintf("Error generating component: %v", err), true)
			return
		}
		sendToolResult(id, fmt.Sprintf("Successfully generated %s component: %s", args.Type, args.Name), false)

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
				defer os.Chdir(oldWd)
				os.Chdir(args.ProjectPath)
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
				defer os.Chdir(oldWd)
				os.Chdir(args.ProjectPath)
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

	default:
		sendError(id, -32601, "Tool not found", nil)
	}
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
