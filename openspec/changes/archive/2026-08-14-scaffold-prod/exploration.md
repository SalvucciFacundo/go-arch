# Exploration: `scaffold-prod` (P2 production-ready scaffolding)

## Status

**Feasible.** All three features are additive scaffold templates + small `scaffold.go` wiring. They only affect NEW scaffolds (plus a deliberate, bounded upgrade path), so already-generated projects cannot break unless we change the shared `main.tmpl` templates — which we must do carefully (see Q6 and Risks).

## Executive Summary

The scaffolder currently renders DB-driven env/compose configuration but generates **zero DB Go code**: `db_driver` only flows into `.go-arch.yaml`, `.env.example`, and `docker-compose.yaml` (config.tmpl:13, env.tmpl:7-18, docker-compose.yaml.tmpl:11-61); `go.mod.tmpl` has no driver deps and no migrations or runner are emitted (the portfolio user built the whole stack by hand). The four `main.tmpl` files are flat mains — only `web/main.tmpl` even starts an HTTP server, and it reads `os.Getenv("SERVER_PORT")` inline (web/main.tmpl:50-53). There is no `internal/config` package anywhere in the templates. All three features slot into `createCommonFiles` (scaffold.go:352-410) as new `createFile` calls and new templates under `internal/pkg/template/templates/common/`, registered by path string through the embedded FS (engine.go:26-27, 108-139).

**Scope shape — ONE change, three work units.** Typed config is the foundation (no dependencies), the subcommand main consumes it, and migrations consumes both (subcommand `migrate` + `cfg.DatabaseURL`). Splitting into three changes would re-touch the same `main.tmpl` files three times, multiplying upgrade churn and review overhead for zero isolation benefit. Estimated ~1,100-1,600 authored lines → **exceeds the 400-line review budget**; the tasks phase MUST plan chained PRs or an explicit size exception.

**Critical constraint discovered:** `go.mod` is report-only on upgrade (upgrade.go:145-163, 390-391). Any new import referenced by a MODIFIED `main.tmpl` must be compileable in OLD projects too, or the upgrade re-render breaks them. This drives the design: **stdlib-only subcommand dispatch** and an upgrade "inject missing scaffold-owned files" mechanism (extending the existing routes.go precedent, upgrade.go:259-273, 123-134).

## Findings

### 1. How the scaffold generates main.go today

| Arch | File | Template | Call site |
|---|---|---|---|
| Minimalist | `main.go` | `minimalist/main.tmpl` | scaffold.go:300-308 |
| Standard | `cmd/api/main.go` | `standard/main.tmpl` | scaffold.go:310-329 |
| Hexagonal | `cmd/api/main.go` | `hexagonal/main.tmpl` | scaffold.go:331-350 |
| Web (any arch) | `cmd/api/main.go` (or `main.go` if Minimalist) | `web/main.tmpl` | scaffold.go:246-298 (target at 293-297) |

- All four mains are flat. `standard/main.tmpl:16-41` and `hexagonal/main.tmpl:19-45` are stubs (print + optional telemetry/grpc, `select{}` to stay alive); they do NOT start an HTTP server. `minimalist/main.tmpl:12-23` is a hello world.
- Only `web/main.tmpl:42-57` starts an HTTP server (`http.ListenAndServe`), reading `os.Getenv("SERVER_PORT")` inline with a `8080` fallback (web/main.tmpl:50-53). This is the exact inline-`os.Getenv` that typed config replaces.
- **No arg parsing or subcommands exist anywhere** — no `create-admin`, no `migrate`, no `server`.
- Template selection is a hardcoded `switch` on `s.config.Architecture` (scaffold.go:164-173) → `scaffoldMinimalist/Standard/Hexagonal`, each creating its main only when `!UseTemplHTMX` (scaffold.go:302, 323, 344), then delegating to `createCommonFiles`.
- **Entry-point inconsistency (pre-existing bug):** `Dockerfile.tmpl:16` hardcodes `go build -o main ./cmd/api/main.go`, so Minimalist projects (main at `main.go`) with `use_docker: true` produce a broken Docker build. The web scaffold for Minimalist also emits `main.go` (scaffold.go:294-296) — same break. The subcommand restructure is the natural place to fix this with a per-architecture build path conditional.

### 2. How db_driver flows

- Source of truth: `ui.ProjectConfig.DBDriver` (`mapstructure:"db_driver"`, ui/prompts.go:13); wizard options `PostgreSQL | MySQL | MongoDB | None`, default None (ui/prompts.go:54-63); MCP `new_project` defaults empty → `"None"` (server.go:443-444) and passes through (server.go:495).
- Written to `.go-arch.yaml` as `db_driver: {{ .DBDriver }}` (config.tmpl:13).
- `env.tmpl:7-18`: `{{ if ne .DBDriver "None" }}` block emits `DATABASE_URL` per driver — `postgres://user:password@localhost:5432/<name>` | `mysql://...:3306/<name>` | `mongodb://localhost:27017/<name>` (env.tmpl:12) — plus `POSTGRES_USER`/`POSTGRES_PASSWORD` (env.tmpl:16-17). **Wart:** POSTGRES_* vars are emitted for MySQL/Mongo too (they're inside the `ne None` branch, not the PostgreSQL branch).
- `docker-compose.yaml.tmpl:11-61`: `db` service per driver (image/healthcheck/ports) — BUT line 12 hardcodes `DATABASE_URL: postgres://...@db:5432/...` in the `api.environment` regardless of driver, and line 59 hardcodes the postgres data volume path `/var/lib/postgresql/data` for all drivers. **Two real bugs for MySQL/Mongo compose flows.**
- `go.mod.tmpl:1-27`: **zero DB dependencies.** Only conditional blocks exist for observability, gRPC, templ. No pgx / go-sql-driver / mongo driver regardless of DBDriver.
- Generated DB Go code: **none.** `grep DBDriver` over templates hits only config.tmpl, crud_repository.tmpl (which just prints `"Saving X to <DBDriver> database..."`, crud_repository.tmpl:18-38) and docker-compose. `repository.tmpl:14-18` has a `// db connection` stub. So today `db_driver` = env + compose + a YAML field, nothing executable.

### 3. Existing patterns to reuse

- **dbmigrate pattern (from portfolio-go feedback):** NOT in this repo (no `dbmigrate`/`migrations` anywhere outside archived docs). The described pattern — `go:embed migrations/*.sql` + idempotent runner tracking applied migrations in a `schema_migrations` table — is the canonical zero-dep Go approach and is what we replicate. Constraint: `go:embed` cannot traverse above the package dir (`..` forbidden), so embedded SQL must live inside the package: `internal/dbmigrate/migrations/*.sql`.
- **Typed config pattern:** no `internal/config` exists in templates. The go-arch CLI itself uses Viper (cmd/root.go), but scaffolded projects carry no deps by default (go.mod.tmpl), so the scaffolded `internal/config` should be **stdlib-only** (`os.Getenv` + `strconv`), mirroring how web/main.tmpl already reads `SERVER_PORT`. `hooks/config.go` shows the codebase's own viper+yaml validation style but is CLI-side, not a template.
- **Subcommand pattern:** the CLI's own Cobra tree (cmd/root.go, cmd/version.go, ...) is the in-repo reference, and golang-cli skill prescribes Cobra+Viper. But adding Cobra to every scaffolded project means new go.mod deps — which collide with the upgrade report-only constraint (Q6). For v1 the scaffolded main should use a stdlib `switch os.Args[1]`; Cobra can be a later opt-in.
- **MCP generators / packs:** `internal/pkg/generators` + pack recipes can generate files, but they are per-pack and not part of the built-in scaffold; the built-in path for these features is plain templates + `createFile`, not generators. A future `go-arch generate migration` could reuse the generators framework, but that is out of scope here.

### 4. Config/env conventions

- `env.tmpl` currently defines: `SERVER_PORT=8080`, `APP_ENV=development`, `DATABASE_URL` (per driver), `POSTGRES_USER`/`POSTGRES_PASSWORD` (compose provisioning).
- `APP_ENV` is defined but consumed nowhere in any template — typed config is its first consumer.
- Minimal `internal/config` (stdlib-only, template-conditional on DBDriver):
  - `Config{ ServerPort int; AppEnv string; DatabaseURL string }`; `Load() (*Config, error)` with defaults (8080/development), `strconv.Atoi` validation for `SERVER_PORT`, and (when `{{ if ne .DBDriver "None" }}`) a hard requirement on `DATABASE_URL`.
- Driver connection-string shapes: pgx expects `postgres://` (via `pgx/v5/stdlib`), go-sql-driver/mysql expects `mysql://` or `user:pass@tcp(...)`, mongo driver expects `mongodb://`. Since the scaffold generates no DB connection code today, `internal/config` only needs to carry `DATABASE_URL` generically; driver-specific `sql.Open` belongs to the dbmigrate runner (SQL drivers only — see Q7 open question on Mongo).

### 5. Scaffold structure — where new files slot in

- New **always-on** files (typed config) → `createCommonFiles` (scaffold.go:352-410), as new `createFile("internal/config/config.go", "common/config_go.tmpl", nil)` alongside go.mod/.go-arch.yaml (scaffold.go:354-364).
- New **conditional** files (migrations + runner) → same function, following the existing `if s.config.UseDocker { ... }` pattern (scaffold.go:372-379): `if s.config.DBDriver == "PostgreSQL" || s.config.DBDriver == "MySQL" { ... }`.
- Templates are registered by **path string** in `createFile` calls; the embedded FS is `//go:embed all:templates/*` (engine.go:26-27) and resolution is local → global → pack → embedded (engine.go:108-139). New templates go under `internal/pkg/template/templates/common/` and are referenced as `"common/<name>.tmpl"`. No registry file to update.
- Template data is `*ui.ProjectConfig` (createFile passes `s.config` when data is nil, scaffold.go:214-216), so new templates can use `{{ .DBDriver }}`, `{{ .Architecture }}`, `{{ .ModuleName }}`, and the existing funcs `lower/plural/title/now` (engine.go:228-243).
- `scaffoldWeb` (scaffold.go:249-298) overrides the main target per arch — the subcommand rework must keep that dispatch consistent (including the Minimalist `main.go` case).

### 6. Backward compat (manifest / fingerprint / upgrade)

- Upgrade iterates ONLY `m.Files` (manifest entries) (upgrade.go:92). Files not in the manifest are never touched — new scaffold files therefore never appear in old projects **by default**.
- PROTECTED (never overwritten): user-modified disk content (upgrade.go:224-228), generator-origin entries (upgrade.go:102-111), pack entries whose pack is not installed (upgrade.go:192-197).
- `go.mod` is always report-only — never written (upgrade.go:145-163, Apply skip at 390-391).
- The ONLY "create missing file" path today is `internal/router/routes.go` (upgrade.go:123-134 absent→rerender; post-loop injection at 259-273; legacy path 594-608). New scaffold-owned files (internal/config, internal/dbmigrate, migrations, and the `migrate` subcommand's needs) must reuse this mechanism if old projects are to receive them.
- **Impact:** if we keep the same `main.tmpl` paths and rework their content, unmodified old projects' `main.go`/`cmd/api/main.go` become `ClassUpgradable` and get rewritten on `go-arch upgrade`. Safety rules derived:
  1. The rewritten main MUST compile in old projects → new imports must be stdlib-only OR upgrade must inject the new packages too.
  2. Since `go.mod` is report-only, the rewritten main MUST NOT import a new third-party package (e.g. cobra) — old projects would not have the dep. **This rules out Cobra in the shared main for v1.**
- Fingerprint implications: new files get manifest entries at scaffold time via `recordManifest` (scaffold.go:103-126, 222) with `OriginScaffold` — future upgrades can re-render them. New templates must render deterministically (the `now` func already breaks byte-identity for `.go-arch.yaml`; avoid it in the new files, following the routes.go precedent).

### 7. Scope fit — one change or three?

- **Typed config**: affects ALL projects (every arch). Pure addition; zero backward-compat risk for old projects as long as mains don't import it until injection exists. Foundation.
- **Subcommands**: affects ALL projects and all four mains + Dockerfile build path. Highest risk surface (upgrade re-render of mains), needs the injection mechanism.
- **Migrations**: only meaningful when `DBDriver != None` (PostgreSQL/MySQL); independent files (new dirs + runner + go.mod deps for new scaffolds only). Lowest risk.
- One change vs three: they share ONE template surface (main.tmpl × 4) and ONE config surface (env). Three sequential changes would re-edit the same mains three times (config wiring, then dispatch, then migrate subcommand) → triple upgrade churn and review overhead. **Recommendation: ONE change `scaffold-prod` with three sequenced work units** (config → subcommands → migrations), each independently verifiable, delivered via chained PRs (budget exceeds 400 lines).
- Estimated changed lines: templates ~600-800 (config_go ~45; 4 mains ~40-60 each; dbmigrate ~90; 0001_init.sql ~20; go.mod +~10; Dockerfile/compose fixes +~15), scaffold.go +~60, upgrade.go +~40, tests +~300-500 → **~1,100-1,600 authored lines. Review budget: HIGH.**

## Scope Recommendation

**ONE change** (`scaffold-prod`), three work units: (1) typed `internal/config` (stdlib, driver-conditional `DATABASE_URL` requirement), (2) subcommand main (stdlib dispatch, default = server so `CMD ["./main"]` keeps working), (3) migrations runner for PostgreSQL/MySQL (`internal/dbmigrate` + embedded `migrations/*.sql`, `migrate` subcommand). Bundle the two related compose bugs (DATABASE_URL hardcoded to postgres at docker-compose.yaml.tmpl:12; volume path hardcoded to postgres at :59) and the Dockerfile main-path bug (Dockerfile.tmpl:16 vs Minimalist `main.go`) — they sit exactly in the templates this change touches.

## Template Design Sketches

### A. Typed config — `common/config_go.tmpl` → `internal/config/config.go`

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration, read from environment variables.
type Config struct {
	ServerPort int
	AppEnv     string
	DatabaseURL string // empty when db_driver is None
}

// Load reads configuration from the environment, applying defaults.
func Load() (*Config, error) {
	cfg := &Config{ServerPort: 8080, AppEnv: "development"}

	if v := os.Getenv("SERVER_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("SERVER_PORT must be an integer: %q", v)
		}
		cfg.ServerPort = port
	}
	if v := os.Getenv("APP_ENV"); v != "" {
		cfg.AppEnv = v
	}
	{{ if ne .DBDriver "None" }}
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (see .env.example)")
	}
	{{ end }}
	return cfg, nil
}
```

### B. Subcommand main — shared dispatch, per-arch server (stdlib only)

```go
func main() {
	if len(os.Args) < 2 {
		runServer() // default keeps `CMD ["./main"]` and `go run .` working
		return
	}
	switch os.Args[1] {
	case "server":
		runServer()
	case "migrate":
		{{ if or (eq .DBDriver "PostgreSQL") (eq .DBDriver "MySQL") }}
		if err := dbmigrate.Up(driver, cfg.DatabaseURL); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		{{ else }}
		fmt.Fprintln(os.Stderr, "migrations not enabled (db_driver is None)")
		os.Exit(1)
		{{ end }}
	case "version":
		fmt.Println("{{ .ProjectName }} dev")
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
```

- Config loads in `runServer()`/`migrate` via `config.Load()`; `web/main.tmpl:50-53` inline `os.Getenv("SERVER_PORT")` is replaced by `cfg.ServerPort`.
- New-import rule for upgrade safety: `internal/config` and `internal/dbmigrate` are NEW packages — old projects will only compile if upgrade injects them (recommended, see Q6b) OR the main stays config-free for old projects (fallback).
- Dockerfile fix: `{{ if eq .Architecture "Minimalist" }}go build -o main .{{ else }}go build -o main ./cmd/api/main.go{{ end }}` (Dockerfile.tmpl:16).

### C. Migrations runner — `common/dbmigrate_go.tmpl` → `internal/dbmigrate/migrate.go` + `common/migration_sql.tmpl` → `internal/dbmigrate/migrations/0001_init.sql`

```go
package dbmigrate

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up applies pending embedded migrations idempotently. Each migration runs
// once; applied files are tracked in schema_migrations.
func Up(driverName, dsn string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		filename TEXT PRIMARY KEY, applied_at TIMESTAMP DEFAULT NOW())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		// skip when already applied; exec within tx; record
	}
	return nil
}
```

- Driver wiring in go.mod.tmpl (NEW scaffolds only — safe, since go.mod is report-only for old ones):
  - PostgreSQL: `github.com/jackc/pgx/v5` (+ `pgx/v5/stdlib` driver name `pgx`)
  - MySQL: `github.com/go-sql-driver/mysql` (driver name `mysql`)
- MongoDB: no SQL migrations (schemaless) — skip runner; README note instead.
- `0001_init.sql` content is a commented placeholder teaching `CREATE TABLE IF NOT EXISTS` idempotency.

## Open Questions (proposal phase must decide)

1. **Subcommand mechanism**: stdlib `switch os.Args[1]` (recommended — no deps, upgrade-safe) vs Cobra (idiomatic, but breaks the upgrade constraint while go.mod is report-only). Could Cobra be an opt-in later?
2. **Old-project upgrade strategy**: (a) extend the routes.go precedent (upgrade.go:123-134, 259-273) so upgrade CREATES missing scaffold-owned files (internal/config, internal/dbmigrate, migrations) when the new main references them — recommended; or (b) keep the new main config-free so old projects get dispatch only, no new imports. (a) delivers full value but grows upgrade.go and its tests.
3. **MongoDB migrations**: skip entirely (schemaless) vs generate a collection-seeding pattern. Recommend skip + README note.
4. **Driver versions to pin** in go.mod.tmpl (pgx/v5 latest minor, go-sql-driver/mysql v1.8.x) — verify against current releases during spec.
5. **Command names**: keep default = server (backward-compatible with Dockerfile `CMD ["./main"]`); explicit `server` subcommand as alias; `migrate`; `version`. Should `create-admin` be in scope? Recommend NO — it is domain-specific; the pattern enables users to add it.
6. **go:embed placement**: `internal/dbmigrate/migrations/*.sql` (recommended — `..` in embed patterns is forbidden) vs a root-level `migrations` package.
7. **Rollback**: Up-only for P2 vs Up+Down. Recommend Up-only; Down later.
8. **Fail-fast on missing DATABASE_URL** in `config.Load()` for DB projects vs warn-and-continue. Recommend fail-fast.

## Risks

- **Upgrade re-render breakage (highest):** modifying the shared main.tmpl paths makes old unmodified mains ClassUpgradable; any new non-stdlib import breaks those builds because go.mod is report-only. Mitigation: stdlib dispatch + inject-missing-files mechanism; verify with an upgrade test on a pre-change fixture project.
- **Scaffold test churn:** `TestScaffolder_Layouts` (scaffold_test.go:17) and web-main content assertions (scaffold_test.go:372) assert exact file sets/content; new files + main rework require updating several tests and adding driver-matrix cases.
- **Compose/env consistency:** POSTGRES_* vars leaking to MySQL/Mongo env.tmpl:16-17 and the hardcoded postgres DATABASE_URL/volume (docker-compose.yaml.tmpl:12, 59) must be fixed in the same pass or the feature ships with broken MySQL/Mongo compose flows.
- **Minimalist+Docker entry-point bug** (Dockerfile.tmpl:16) becomes user-visible once migrations/subcommands make DB projects more common — fix in scope.
- **Scope creep:** Mongo seeding, Cobra adoption, Down migrations, and create-admin are adjacent ideas — keep them OUT to protect the review budget.
- **400-line budget:** ~1,100-1,600 authored lines → HIGH. tasks phase must chain PRs (config → subcommands → migrations) or request an explicit size exception.

## Ready for Proposal

**Yes.** The proposal phase should decide the open questions (especially #1 subcommand mechanism and #2 upgrade injection) and lock the three work units with their chained PR slices.

## Key Learnings

1. `db_driver` currently drives only env, compose, and .go-arch.yaml — no Go code, deps, or migrations are generated.
2. `go.mod` is report-only on upgrade, so new main-template imports must be stdlib-only unless upgrade injects the new packages.
3. The routes.go absent→create path in upgrade.go is the precedent for injecting new scaffold-owned files into old projects.
4. go:embed cannot traverse above its package directory, so migration SQL must live inside the runner package.
5. Dockerfile.tmpl hardcodes `./cmd/api/main.go`, breaking Minimalist projects with use_docker enabled.
