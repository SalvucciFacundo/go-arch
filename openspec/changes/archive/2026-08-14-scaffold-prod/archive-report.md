# Archive Report — scaffold-prod (P2 production-ready scaffolding)

**Status**: ARCHIVED
**Change**: scaffold-prod
**Merged**: PR #52→#55 (slices) + PR #56 (tracker) → main (commit d42e04f)
**Date**: 2026-08-14

## Executive Summary

Closed the P2 scaffolding features found in real-world use (portfolio-go). Every new scaffolded project is production-ready: stdlib typed config, subcommand-aware main, zero-dep SQL migrations runner, plus compose/Dockerfile/env driver-consistency bug fixes. Determinism fix (removed `generated_at: {{ now }}` → static `scaffold_prod_v1` marker). Verify: 9 packages green, scaffold matrix + upgrade injection tests, live smoke via MCP new_project.

## What Ships

- **Typed config** (`internal/config`): stdlib `Load()` with defaults (8080/development), fail-fast on missing `DATABASE_URL` for DB projects (points to .env.example).
- **Subcommand main**: `switch os.Args[1]` — default = `server` (preserves `CMD ["./main"]`), `migrate`, `version`, unknown → exit 2. Conditional on the marker: `{{ if .ScaffoldProdV1 }}` dispatch + config `{{ else }}` legacy `{{ end }}` — no-marker projects keep byte-identical legacy mains.
- **Migrations runner** (`internal/dbmigrate`): `//go:embed migrations/*.sql`, `Up(driverName, dsn)` idempotent via `schema_migrations` (VARCHAR(255) PK — MySQL-safe), tx-wrapped exec+record, per-driver blank import (pgx stdlib / mysql), `?` placeholder. Generated for PostgreSQL|MySQL only; MongoDB gets env note; None gets nothing.
- **Driver pins**: pgx v5.7.1 (PostgreSQL), go-sql-driver/mysql v1.8.1 (MySQL) — new scaffolds only (go.mod report-only on upgrade).
- **Upgrade injection**: marker'd projects that lose config/dbmigrate get them re-injected (routes.go precedent + `FileAction.TemplatePath` so injected files re-render from their own template; migrations SQL injected with the runner — empty go:embed is a compile error).
- **Compose/Dockerfile/env fixes**: DATABASE_URL per driver, volume per driver (pg/mysql/mongo paths), Minimalist Dockerfile build path (`./main.go` vs `./cmd/api/main.go`), `POSTGRES_*` only for PostgreSQL.
- **Determinism**: `generated_at: {{ now }}` removed (broke upgrade byte-stable re-render) → replaced by static `scaffold_prod_v1: true`.

## Verification Summary

- Full suite green (`go test ./...`, 9 packages), vet + gofmt clean.
- Scaffold matrix: TestScaffoldProd_DispatchInMains (3 arches × dispatch/subcommands/exit-2/config import), TestScaffoldProd_DBMigrateMatrix (4 drivers × runner presence + VARCHAR PK + real SQL statement), TestUpgrade_InjectsScaffoldProd (marker injects 3 files + Apply writes; no-marker nothing), TestEngine_DeterministicConfig (no generated_at, byte-identical re-render).
- Smoke (MCP new_project, PostgreSQL+web): config.go + dbmigrate/migrate.go + migrations/0001_init.sql + dispatch main + marker + pgx pin all generated; runMigrate calls `dbmigrate.Up("pgx", cfg.DatabaseURL)`.
- SDD: explore → propose (8 decisions) → spec (6 req / 23 scenarios) → design (fresh-context validator: 4 blockers + 8 findings — hexagonal main missing, marker contract self-contradiction, MySQL DDL TEXT PK, injection path incomplete) → tasks (23) → apply (4 slices).

## Follow-Ups (non-blocking)

- MongoDB migrations runner — relational-only by design.
- `create-admin` / app-specific subcommands — out of scaffold scope (user adds them per project).
- Down migrations — Up-only.
- The `?` placeholder works for both pgx stdlib and MySQL — validated by inspection; a MySQL live smoke is recommended before the next release (PostgreSQL smoke only this cycle).

## Artifacts

| Artifact | Path |
|----------|------|
| Proposal | `openspec/changes/archive/2026-08-14-scaffold-prod/proposal.md` |
| Exploration | `openspec/changes/archive/2026-08-14-scaffold-prod/exploration.md` |
| Design | `openspec/changes/archive/2026-08-14-scaffold-prod/design.md` |
| Tasks | `openspec/changes/archive/2026-08-14-scaffold-prod/tasks.md` |
| Spec (delta) | `openspec/changes/archive/2026-08-14-scaffold-prod/specs/scaffold-prod/spec.md` |
| Spec (synced) | `openspec/specs/scaffold-prod/spec.md` (6 requirements, byte-identical) |

## Delivery Note

Receipt-driven review disabled at clone scope (user decision after escalating upstream #2743). Delivery under ordinary policy — CI gates (test/lint) are the authority. No review receipt exists; none fabricated.
