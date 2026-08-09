# Proposal: Hooks (roadmap item 5)

**Status**: success

## Intent

Add lifecycle hooks (`pre-new`, `post-new`, `pre-generate`, `post-generate`) to `.go-arch.yaml` so generated projects run their own tooling (`gofmt`, `go mod tidy`, `git init`). These are the extension points the plugin system (roadmap item 4) will formalize.

## Scope

**In**: `hooks:` config (hybrid), `internal/pkg/hooks/` runner, scaffold-layer fire sites in `Execute`/`GenerateComponent`/`GenerateCRUD`, env contract, `config.tmpl` example, `docs/hooks.md`.

**Out**: plugin system, hook chaining/priority, interactive prompts, `*-upgrade` hooks, schema validation beyond type checks.

## Approach

**Config (hybrid)** — strings via shell, objects argv-direct:
```yaml
hooks:
  post-generate:
    - gofmt -w .
    - command: go
      args: [mod, tidy]
      timeout: 60s
```

**Decisions** (exploration open questions resolved):
1. **Config**: hybrid — terse one-liners + object control.
2. **Wiring**: scaffold-layer runner. MCP parity free (same scaffold methods).
3. **`post-new`**: included (symmetric with `pre-new`, home for `git init`).
4. **Failure**: fatal, stop-on-first. `ignore_failure: true` continues.
5. **Shell**: strings → `sh -c` / `cmd /c` (runtime.GOOS). Objects → argv-direct.
6. **Timeout**: 30s default, `timeout:` overrides, `0` disables.
7. **Env**: `PROJECT_NAME`, `PROJECT_PATH` (abs), `ARCHITECTURE`, `HOOK_TYPE`; process env inherited.
8. **MCP**: hooks run; no prompts; output via `ui.Out` (stderr); stdin closed.
9. **Upgrade**: fires NO hooks (ADR-8 protects `.go-arch.yaml`).
10. **Output**: HARD rule — all hook output via `ui.Out`; `os.Stdout` fails the runner.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/hooks/` | New | Runner, config types, env builder. |
| `scaffold/scaffold.go` | Modified | 4 fire sites via `*hooks.Runner`. |
| `cmd/new.go`, `cmd/generate.go` | Modified | Build runner, inject. |
| `config.tmpl` | Modified | Commented `hooks:` example. |
| `docs/hooks.md` | New | Reference + trust warning. |
| `mcp/server.go` | Unchanged | Parity via scaffold wiring. |

## Risks & Mitigations

- **Config-as-code trust** → docs + `new` wizard warning (npm/goreleaser model).
- **Shell divergence** → objects bypass shell; strings unix-preferring.
- **MCP stdout corruption** → runner rejects `os.Stdout`; integration test asserts.
- **Partial state on `post-*` failure** → non-atomic, documented; `ignore_failure` for best-effort.

## Rollback Plan

Drop Runner calls + package. User `.go-arch.yaml` with `hooks:` is ignored by older versions (viper skips unknown keys) — no migration.

## Success Criteria

- [ ] `new`/`generate` fire pre/post hooks; `post-new` CWD = new project dir.
- [ ] MCP fires same hooks; output on stderr only.
- [ ] `upgrade` fires NO hooks.
- [ ] Timeouts kill runaways; non-zero exit fails unless ignored.
- [ ] Runner has faked `CommandRunner`; `go test ./...` green.

## Edge Cases

Missing key / empty list → no-op. Unknown type → `Code("unknown_hook_type")`. Command missing → `Code("hook_command_not_found")`. Non-zero exit → `Code("hook_failed")`. `post-*` failure after writes → non-atomic.

**Open Questions**: none — all 10 exploration questions resolved.

**Next Recommended**: `sdd-spec`.
