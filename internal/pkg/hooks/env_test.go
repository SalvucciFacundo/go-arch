package hooks

import (
	"strings"
	"testing"
)

// assertEnv checks that env slice contains KEY=VALUE exactly once.
// Fails if the key is missing or has the wrong value.
func assertEnv(t *testing.T, env []string, key, want string) {
	t.Helper()
	prefix := key + "="
	found := false
	for _, e := range env {
		if e == prefix+want {
			found = true
			break
		}
		if strings.HasPrefix(e, prefix) {
			t.Errorf("env[%s]: got %q, want %q", key, e[len(prefix):], want)
			return
		}
	}
	if !found {
		t.Errorf("env[%s]: not found, want %s=%s", key, key, want)
	}
}

func TestBuildEnv_Precedence_ParentIsBase(t *testing.T) {
	// parent env is the base layer — per-entry overrides everything.
	parent := []string{"FOO=parent", "PROJECT_NAME=parent_val"}
	ctx := EnvContext{
		ProjectName: "myproj",
		ProjectPath: "/abs/path",
		Arch:        "Standard",
		HookType:    PostGenerate,
	}
	perEntry := map[string]string{
		"PROJECT_NAME": "override",
		"CUSTOM":       "custom_val",
	}

	result := BuildEnv(parent, ctx, perEntry)

	// parent value inherited when not overridden
	assertEnv(t, result, "FOO", "parent")
	// per-entry wins over standard (which would have set "myproj")
	assertEnv(t, result, "PROJECT_NAME", "override")
	// per-entry key is present
	assertEnv(t, result, "CUSTOM", "custom_val")
}

func TestBuildEnv_AllFourStandardVars(t *testing.T) {
	parent := []string{"PATH=/usr/bin"}
	ctx := EnvContext{
		ProjectName: "myapp",
		ProjectPath: "/home/user/myapp",
		Arch:        "Hexagonal",
		HookType:    PreNew,
	}

	result := BuildEnv(parent, ctx, nil)

	assertEnv(t, result, "PROJECT_NAME", "myapp")
	assertEnv(t, result, "PROJECT_PATH", "/home/user/myapp")
	assertEnv(t, result, "ARCHITECTURE", "Hexagonal")
	assertEnv(t, result, "HOOK_TYPE", "pre-new")
	// PATH inherited from parent
	assertEnv(t, result, "PATH", "/usr/bin")
}

func TestBuildEnv_HOOK_TYPE_MatchesFiredType(t *testing.T) {
	tests := []struct {
		name     string
		hookType Type
		expected string
	}{
		{"pre-new", PreNew, "pre-new"},
		{"post-new", PostNew, "post-new"},
		{"pre-generate", PreGenerate, "pre-generate"},
		{"post-generate", PostGenerate, "post-generate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := EnvContext{
				ProjectName: "test",
				ProjectPath: "/tmp/test",
				Arch:        "Minimalist",
				HookType:    tt.hookType,
			}
			result := BuildEnv(nil, ctx, nil)
			assertEnv(t, result, "HOOK_TYPE", tt.expected)
		})
	}
}

func TestBuildEnv_PROJECT_PATH_IsAbsolute(t *testing.T) {
	// PROJECT_PATH is set to ctx.ProjectPath as-is. The caller is
	// responsible for passing an absolute path.
	ctx := EnvContext{
		ProjectName: "myproj",
		ProjectPath: "/absolute/path/to/myproj",
		Arch:        "Standard",
		HookType:    PostNew,
	}
	result := BuildEnv(nil, ctx, nil)
	assertEnv(t, result, "PROJECT_PATH", "/absolute/path/to/myproj")
}

func TestBuildEnv_PerEntryOverridesStandard(t *testing.T) {
	ctx := EnvContext{
		ProjectName: "myproj",
		ProjectPath: "/abs/myproj",
		Arch:        "Standard",
		HookType:    PreNew,
	}
	perEntry := map[string]string{
		"PROJECT_NAME": "custom-name",
		"ARCHITECTURE": "custom-arch",
	}

	result := BuildEnv(nil, ctx, perEntry)

	assertEnv(t, result, "PROJECT_NAME", "custom-name")
	assertEnv(t, result, "ARCHITECTURE", "custom-arch")
	// non-overridden standard vars still present
	assertEnv(t, result, "PROJECT_PATH", "/abs/myproj")
	assertEnv(t, result, "HOOK_TYPE", "pre-new")
}

func TestBuildEnv_NilParentNilPerEntry(t *testing.T) {
	ctx := EnvContext{
		ProjectName: "test",
		ProjectPath: "/tmp/test",
		Arch:        "Minimalist",
		HookType:    PostGenerate,
	}
	result := BuildEnv(nil, ctx, nil)

	if len(result) != 4 {
		t.Errorf("expected exactly 4 standard vars, got %d: %v", len(result), result)
	}
}

func TestBuildEnv_EmptyParent_WithPerEntry(t *testing.T) {
	parent := []string{}
	ctx := EnvContext{
		ProjectName: "test",
		ProjectPath: "/tmp/test",
		Arch:        "Minimalist",
		HookType:    PreNew,
	}
	perEntry := map[string]string{"FOO": "bar"}

	result := BuildEnv(parent, ctx, perEntry)

	assertEnv(t, result, "PROJECT_NAME", "test")
	assertEnv(t, result, "FOO", "bar")
	if len(result) != 5 {
		t.Errorf("expected 5 vars (4 standard + 1 custom), got %d: %v", len(result), result)
	}
}

func TestBuildEnv_ParentEnvInherited_WithoutClobber(t *testing.T) {
	// Multiple parent vars preserved alongside standard vars.
	parent := []string{"USER=kuno", "HOME=/home/kuno", "SHELL=/bin/zsh"}
	ctx := EnvContext{
		ProjectName: "myproj",
		ProjectPath: "/tmp/myproj",
		Arch:        "Minimalist",
		HookType:    PostNew,
	}

	result := BuildEnv(parent, ctx, nil)

	assertEnv(t, result, "USER", "kuno")
	assertEnv(t, result, "HOME", "/home/kuno")
	assertEnv(t, result, "SHELL", "/bin/zsh")
	assertEnv(t, result, "PROJECT_NAME", "myproj")
	if len(result) != 7 {
		t.Errorf("expected 7 vars (3 parent + 4 standard), got %d", len(result))
	}
}

func TestBuildEnv_PerEntryOverridesParent(t *testing.T) {
	parent := []string{"FOO=parent_value"}
	ctx := EnvContext{
		ProjectName: "test",
		ProjectPath: "/tmp/test",
		Arch:        "Standard",
		HookType:    PreNew,
	}
	perEntry := map[string]string{"FOO": "hook_value"}

	result := BuildEnv(parent, ctx, perEntry)
	assertEnv(t, result, "FOO", "hook_value")
}

func TestBuildEnv_StandardOverridesParent(t *testing.T) {
	// Standard vars override same-named parent vars.
	parent := []string{"PROJECT_NAME=from_parent", "ARCHITECTURE=from_parent"}
	ctx := EnvContext{
		ProjectName: "from_ctx",
		ProjectPath: "/tmp/test",
		Arch:        "Hexagonal",
		HookType:    PreNew,
	}

	result := BuildEnv(parent, ctx, nil)

	assertEnv(t, result, "PROJECT_NAME", "from_ctx")
	assertEnv(t, result, "ARCHITECTURE", "Hexagonal")
}

// TestBuildEnv_PackEnvVarsPresent verifies task 4.5: when PackName and
// PackVersion are set, PACK_NAME and PACK_VERSION appear in the env.
func TestBuildEnv_PackEnvVarsPresent(t *testing.T) {
	ctx := EnvContext{
		ProjectName: "myapp",
		ProjectPath: "/tmp/myapp",
		Arch:        "Standard",
		HookType:    PostNew,
		PackName:    "express",
		PackVersion: "1.0.0",
	}

	result := BuildEnv(nil, ctx, nil)

	assertEnv(t, result, "PACK_NAME", "express")
	assertEnv(t, result, "PACK_VERSION", "1.0.0")
	// Standard vars still present
	assertEnv(t, result, "PROJECT_NAME", "myapp")
	assertEnv(t, result, "HOOK_TYPE", "post-new")
}

// TestBuildEnv_PackEnvVarsAbsent verifies task 4.5: when PackName and
// PackVersion are empty, PACK_NAME and PACK_VERSION are absent.
func TestBuildEnv_PackEnvVarsAbsent(t *testing.T) {
	ctx := EnvContext{
		ProjectName: "myapp",
		ProjectPath: "/tmp/myapp",
		Arch:        "Standard",
		HookType:    PostNew,
	}

	result := BuildEnv(nil, ctx, nil)

	// PACK_NAME and PACK_VERSION must NOT be present
	for _, e := range result {
		if strings.HasPrefix(e, "PACK_NAME=") || strings.HasPrefix(e, "PACK_VERSION=") {
			t.Errorf("expected PACK_NAME/PACK_VERSION to be absent when empty, but found: %s", e)
		}
	}
}
