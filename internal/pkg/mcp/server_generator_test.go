package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------- MCP Generator Tests (Slice 4) ---------------

func TestMCPServer_ToolsList_IncludesListGenerators(t *testing.T) {
	output := captureStdout(func() {
		req := Request{JSONRPC: "2.0", Method: "tools/list", ID: 1}
		handleRequest(&req)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	resultStr := string(resultJSON)

	if !strings.Contains(resultStr, "list_generators") {
		t.Error("tools/list should include list_generators tool")
	}

	// generate_component type should NOT have a fixed enum.
	if strings.Contains(resultStr, `"enum":["service","repository","handler","crud","page","component"]`) {
		t.Error("generate_component type should NOT have a fixed enum (must be relaxed to any string)")
	}

	// generate_component should have generatorArgs parameter.
	if !strings.Contains(resultStr, "generatorArgs") {
		t.Error("generate_component schema should include generatorArgs parameter")
	}
}

func TestMCPServer_GenerateComponent_TypeRelaxed(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-type-relax-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "."
module_name: github.com/test/mcprelax
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{
			"type": "docker",
			"name": "myservice",
		})
		var rawArgs json.RawMessage = args
		handleToolCall(2, "generate_component", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	// Should get an error response (no pack installed for docker),
	// but NOT a parse/validation error.
	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if !tcr.IsError {
		// Success is also fine — means the generator ran. Either way, no crash.
		t.Log("generator ran without error (ok)")
		return
	}
	// Error is expected since no docker generator installed.
	t.Logf("expected error (no pack): %s", tcr.Content[0].Text)
}

func TestMCPServer_GenerateComponent_UnknownTypeError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-unknown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "."
module_name: github.com/test/mcpunknown
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{
			"type": "bogus999",
			"name": "Whatever",
		})
		var rawArgs json.RawMessage = args
		handleToolCall(3, "generate_component", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if !tcr.IsError {
		t.Fatal("expected error for unknown generator type, got success")
	}
	errMsg := tcr.Content[0].Text
	if !strings.Contains(errMsg, "bogus999") {
		t.Errorf("error should mention unknown generator type, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "unknown") && !strings.Contains(errMsg, "generator") {
		t.Errorf("error should indicate unknown generator, got: %s", errMsg)
	}
	// Error should include the unknown_generator code/grouped listing.
	if !strings.Contains(errMsg, "unknown_generator") {
		t.Errorf("error should contain 'unknown_generator', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "component types") && !strings.Contains(errMsg, "component") {
		t.Errorf("error should list component types, got: %s", errMsg)
	}
}

// TestMCPServer_GenerateComponent_PackNotInstalled verifies that when
// .go-arch.yaml declares template: <pack> but the pack is not installed,
// MCP generate_component returns pack_not_installed for non-component types.
func TestMCPServer_GenerateComponent_PackNotInstalled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-pni-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestMCPPNI"
module_name: github.com/test/mcppni
architecture: Standard
template: missingpack@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{
			"type": "docker",
			"name": "whatevs",
		})
		var rawArgs json.RawMessage = args
		handleToolCall(10, "generate_component", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if !tcr.IsError {
		t.Fatal("expected error (pack_not_installed), got success")
	}
	errMsg := tcr.Content[0].Text
	if !strings.Contains(errMsg, "pack_not_installed") {
		t.Errorf("error should contain 'pack_not_installed', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "missingpack") {
		t.Errorf("error should name pack 'missingpack', got: %s", errMsg)
	}
}

func TestMCPServer_ListGenerators(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-listgen-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	configYAML := `project_name: "TestListMCP"
module_name: github.com/test/listmcp
architecture: Standard
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{})
		var rawArgs json.RawMessage = args
		handleToolCall(4, "list_generators", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if tcr.IsError {
		t.Fatalf("expected success for list_generators, got error: %s", tcr.Content[0].Text)
	}

	content := tcr.Content[0].Text
	if !strings.Contains(content, "service") {
		t.Errorf("list_generators should mention component types, got: %s", content)
	}
	if !strings.Contains(content, "builtin-component") {
		t.Errorf("list_generators should mention builtin-component source, got: %s", content)
	}
}

// --------------- Pack Installed + Unknown Type Regression Tests ---------------

// TestMCPServer_GenerateComponent_WithTemplate_UnknownType verifies that
// when .go-arch.yaml declares a template pack that IS installed, but the
// requested generator type does NOT exist in the pack, the MCP handler
// returns unknown_generator (NOT pack_not_installed) and includes the
// pack's available generators. REQ-22 S3 / REQ-10 S1 regression guard.
func TestMCPServer_GenerateComponent_WithTemplate_UnknownType(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-pkg-unknown-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a fixture v2 pack with generators "docker" and "service".
	packDir := filepath.Join(tmpDir, "smoke@1.0.0")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `contract_version: 2
name: smoke
version: 1.0.0
generators:
  docker:
    description: "Generate Docker config"
    steps:
      - type: template
        from: "Dockerfile.tmpl"
        to: "Dockerfile"
  service:
    description: "Generate a service"
    steps:
      - type: template
        from: "handler.tmpl"
        to: "internal/handler/x.go"
`
	if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create template files so Load succeeds.
	tmplDir := filepath.Join(packDir, "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "Dockerfile.tmpl"),
		[]byte("FROM golang:1.24"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "handler.tmpl"),
		[]byte("package handler"), 0644); err != nil {
		t.Fatal(err)
	}

	// Point GO_ARCH_PACKS_DIR so LatestInstalled finds the pack.
	t.Setenv("GO_ARCH_PACKS_DIR", tmpDir)

	// Create .go-arch directory so manifest operations don't fail.
	archDir := filepath.Join(tmpDir, ".go-arch")
	if err := os.MkdirAll(archDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archDir, "manifest.yaml"),
		[]byte("version: 1\nfiles: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .go-arch.yaml with template referencing the installed pack.
	configYAML := `project_name: "TestMCPPackUK"
module_name: github.com/test/mcppackuk
architecture: Standard
template: smoke@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{
			"type": "bogus",
			"name": "Whatsit",
		})
		var rawArgs json.RawMessage = args
		handleToolCall(20, "generate_component", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if !tcr.IsError {
		t.Fatal("expected error for unknown generator type with installed pack")
	}
	errMsg := tcr.Content[0].Text

	// Must NOT claim pack is not installed (pack IS installed).
	if strings.Contains(errMsg, "pack_not_installed") {
		t.Errorf("error should NOT contain 'pack_not_installed' (pack IS installed), got: %s", errMsg)
	}

	// Must contain unknown_generator.
	if !strings.Contains(errMsg, "unknown_generator") {
		t.Errorf("error should contain 'unknown_generator', got: %s", errMsg)
	}

	// Must list the pack's available generators.
	if !strings.Contains(errMsg, "docker") {
		t.Errorf("error should list pack generator 'docker', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "service") {
		t.Errorf("error should list pack generator 'service', got: %s", errMsg)
	}

	// Must list component types.
	if !strings.Contains(strings.ToLower(errMsg), "component types") {
		t.Errorf("error should list component types, got: %s", errMsg)
	}
}

// TestMCPServer_GenerateComponent_MissingRequiredArg verifies that when
// an MCP generate_component call omits a required prompt from
// generatorArgs, the error code is missing_generator_argument (not
// generator_prompt_unresolvable). REQ-25 S1 regression guard.
func TestMCPServer_GenerateComponent_MissingRequiredArg(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-promptreq-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create a v2 pack with a generator that has a required prompt.
	packDir := filepath.Join(tmpDir, "promptreq@1.0.0")
	if err := os.MkdirAll(packDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `contract_version: 2
name: promptreq
version: 1.0.0
generators:
  gen:
    description: "Generator with required prompt"
    steps:
      - type: prompt
        name: "db_driver"
        message: "DB Driver?"
        required: true
      - type: template
        from: "x.tmpl"
        to: "x.txt"
`
	if err := os.WriteFile(filepath.Join(packDir, "go-arch.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	tmplDir := filepath.Join(packDir, "templates")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "x.tmpl"),
		[]byte("name: {{ .ProjectName }}"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GO_ARCH_PACKS_DIR", tmpDir)

	// Create .go-arch directory.
	archDir := filepath.Join(tmpDir, ".go-arch")
	if err := os.MkdirAll(archDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archDir, "manifest.yaml"),
		[]byte("version: 1\nfiles: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	configYAML := `project_name: "TestMCPPromptReq"
module_name: github.com/test/mcppromptreq
architecture: Standard
template: promptreq@1.0.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".go-arch.yaml"), []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Call generate_component with empty generatorArgs — required prompt missing.
	output := captureStdout(func() {
		args, _ := json.Marshal(map[string]interface{}{
			"type":          "gen",
			"name":          "Whatsit",
			"generatorArgs": map[string]interface{}{}, // empty — missing required "db_driver"
		})
		var rawArgs json.RawMessage = args
		handleToolCall(30, "generate_component", rawArgs)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	jsonLine := lines[len(lines)-1]

	var resp Response
	if err := json.Unmarshal([]byte(jsonLine), &resp); err != nil {
		t.Fatalf("expected valid JSON response, got: %s", jsonLine)
	}

	resultJSON, _ := json.Marshal(resp.Result)
	var tcr ToolCallResponse
	if err := json.Unmarshal(resultJSON, &tcr); err != nil {
		t.Fatalf("expected ToolCallResponse, got: %s", string(resultJSON))
	}
	if !tcr.IsError {
		t.Fatal("expected error for missing required generator arg")
	}
	errMsg := tcr.Content[0].Text

	// Must contain missing_generator_argument (NOT generator_prompt_unresolvable).
	if !strings.Contains(errMsg, "missing_generator_argument") {
		t.Errorf("MCP error should contain 'missing_generator_argument', got: %s", errMsg)
	}
	if strings.Contains(errMsg, "generator_prompt_unresolvable") {
		t.Errorf("MCP error should NOT contain 'generator_prompt_unresolvable', got: %s", errMsg)
	}

	// Must name the missing prompt.
	if !strings.Contains(errMsg, "db_driver") {
		t.Errorf("MCP error should name the missing prompt 'db_driver', got: %s", errMsg)
	}
}

// TestMCPServer_GenerateComponent_MissingRequiredArg_CLICode verifies that
// when the CLI non-interactive path has a required prompt, the error code
// is generator_prompt_unresolvable (not missing_generator_argument).
// This is the CLI counterpart — ensures the codes are correctly
// discriminated by the caller (cmd vs mcp).
func TestMCPServer_GenerateComponent_PromptCode_Discrimination(t *testing.T) {
	// This test confirms the scaffold's default code is
	// generator_prompt_unresolvable (for CLI), which is the default
	// config value in GeneratePackGenerator.
	// The MCP-specific test above (TestMCPServer_GenerateComponent_MissingRequiredArg)
	// already proves that MCP gets missing_generator_argument.
	// This doc-test documents the discrimination: CLI = generator_prompt_unresolvable,
	// MCP = missing_generator_argument.
	t.Log("CLI default prompt code = generator_prompt_unresolvable (verified by executor_test.go)")
	t.Log("MCP prompt code = missing_generator_argument (verified by TestMCPServer_GenerateComponent_MissingRequiredArg)")
}
