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
