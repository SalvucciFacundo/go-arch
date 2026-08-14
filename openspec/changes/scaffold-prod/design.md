# Design: scaffold-prod (P2 production-ready scaffolding)

**status**: success
**next_recommended**: tasks

## Technical Approach

Three sequenced work units inside ONE change, all additive templates + `createCommonFiles` wiring:
1. **Typed config** — new `internal/config` template (stdlib `Load()`, fail-fast on missing `DATABASE_URL` for DB projects).
2. **Subcommand main** — every `main.tmpl` gains `switch os.Args[1]` (default = `server`, plus `migrate`/`version`, unknown → exit 2); web main uses `cfg.ServerPort`.
3. **Migrations runner** — new `internal/dbmigrate` template (`//go:embed migrations/*.sql`, `Up(driverName, dsn)` idempotent via `schema_migrations`) generated when `DBDriver` is PostgreSQL or MySQL.
Plus: upgrade-injection of the new packages into marker'd old projects (routes.go precedent), and the compose/Dockerfile/env bug fixes (driver-correct DATABASE_URL/volume, Minimalist main path, POSTGRES_* only for PostgreSQL).

**Determinism refinement (flagged)**: `config.tmpl` renders `generated_at: {{ now }}` — NON-deterministic, breaks upgrade byte-stable re-render. This design REMOVES `generated_at` (replaced by static `scaffold_prod_v1: true` marker). Verified safe: `.go-arch.yaml` is ADR-8 exempt from upgrade re-render (upgrade.go:93-95); no test asserts `generated_at`; configFromViper never reads it.

**Marker contract (blocker b resolution)**: each `main.tmpl` renders CONDITIONALLY on the marker:
```
{{ if .ScaffoldProdV1 }}<dispatch + config/dbmigrate imports>{{ else }}<legacy flat content>{{ end }}
```
- No-marker old project → main re-renders to byte-identical legacy → `up_to_date` (upgrade.go:239-242), nothing changes, "legacy preserved" ✓ (spec R5 scenario 2).
- Marker'd project with packages deleted → dispatch main + injection recreates `internal/config` + `internal/dbmigrate`; go.mod already carries the pins → compiles ✓.
- **Refinement note**: spec R5 scenario 1 ("no-marker project re-renders to dispatch + injects") is NOT implementable under the marker gate — no-marker projects keep legacy mains by design. Flagged explicitly.

## Architecture Decisions

| Decision | Options | Chosen | Rationale |
|---|---|---|---|
| Config package | `internal/config` stdlib vs viper | **stdlib `os.Getenv` + validation** | go.mod report-only on upgrade; viper would break re-render compile |
| Subcommand dispatch | stdlib switch vs Cobra | **stdlib switch** | Same constraint |
| Migrations driver | pgx/v5 + mysql vs stdlib-only | **pgx/v5, go-sql-driver/mysql** | Driver deps only in NEW scaffolds (report-only safe) |
| Marker field | ProjectConfig field vs render data | **`ScaffoldProdV1 bool mapstructure:"scaffold_prod_v1"` on ProjectConfig** | Render data can't flow into configFromViper; template hardcodes `scaffold_prod_v1: true` (RunWizard never sets the field, so `{{ .ScaffoldProdV1 }}` would emit false for new projects) |
| Marker gate | gate injection on marker | **conditional main rendering + marker-gated injection** | Coherent contract (blocker b) |
| schema_migrations DDL | TEXT vs VARCHAR | **`version VARCHAR(255) PRIMARY KEY`** | MySQL rejects TEXT in keys without length; VARCHAR works on both (blocker c) |
| Migration tx | bare exec vs tx | **wrap exec+record in a tx** | Failed migration leaves schema_migrations unrecorded otherwise (D8) |
| ServerPort type | int vs string | **string** (spec R1 supersedes proposal's strconv.Atoi — refinement note) | Spec is authoritative |
| Compose DATABASE_URL | per driver | **Postgres/MySQL prefix, no Mongo** | Fixes hardcoded postgres bug |
| Compose volume | per driver | **pg:/var/lib/postgresql/data, mysql:/var/lib/mysql** | Fixes postgres-volume-for-all bug |
| Dockerfile main path | conditional | **Minimalist `./main.go`, else `./cmd/api/main.go`** | Fixes Minimalist+Docker build bug |
| env.tmpl POSTGRES_* | only PostgreSQL | **conditional on DBDriver == PostgreSQL** | Fixes leak into MySQL/None projects |

## Template Files

### 1. `internal/pkg/template/templates/common/config_go.tmpl` (NEW)

```go
package config

import (
	"fmt"
	"os"
)

// Config holds runtime configuration read from the environment.
type Config struct {
	ServerPort  string
	AppEnv      string
	DatabaseURL string
}

// Load reads configuration from the environment.
{{ if ne .DBDriver "None" }}
// It fails fast when DATABASE_URL is missing — copy .env.example to .env first.
{{ end }}
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:  envOr("SERVER_PORT", "8080"),
		AppEnv:      envOr("APP_ENV", "development"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
{{ if ne .DBDriver "None" }}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required (see .env.example)")
	}
{{ end }}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

### 2. `internal/pkg/template/templates/common/dbmigrate_go.tmpl` (NEW, when DBDriver == PostgreSQL || MySQL)

```go
// Package dbmigrate applies embedded SQL migrations idempotently.
package dbmigrate

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
)

{{ if eq .DBDriver "PostgreSQL" }}
_ "github.com/jackc/pgx/v5/stdlib" // postgres driver
{{ else if eq .DBDriver "MySQL" }}
_ "github.com/go-sql-driver/mysql" // mysql driver
{{ end }}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Up applies all pending migrations in filename order, each in a transaction.
func Up(driverName, dsn string) error {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
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
		if applied[name] {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := applyOne(db, name, data); err != nil {
			return err
		}
		fmt.Printf("applied migration %s\n", name)
	}
	return nil
}

func applyOne(db *sql.DB, name string, data []byte) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(string(data)); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	// The pgx stdlib driver accepts ? and translates to $n; MySQL uses ? natively.
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	return tx.Commit()
}
```

### 3. `internal/pkg/template/templates/common/migration_sql.tmpl` (NEW, example 0001_init.sql — MUST be a real executable statement)

```sql
-- 0001_init.sql — initial schema.
-- Migrations are applied in filename order; add your tables here.

CREATE TABLE IF NOT EXISTS app_meta (
    key   VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL
);
```

### 4. `internal/pkg/template/templates/common/main_dispatch.tmpl` (NEW — shared dispatch snippet)

```go
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			runServer()
			return
		case "migrate":
			if err := runMigrate(); err != nil {
				log.Fatalf("migrate: %v", err)
			}
			return
		case "version":
			fmt.Println("{{ .ProjectName }} dev")
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\nusage: %s [server|migrate|version]\n", os.Args[1], os.Args[0])
			os.Exit(2)
		}
	}
	runServer()
}
```

### 5. Modified mains (ALL FOUR — blocker a fix)

- **minimalist/main.tmpl**: conditional dispatch (`{{ if .ScaffoldProdV1 }}` dispatch + config `{{ else }}` legacy `{{ end }}`); `runServer()` starts HTTP on `cfg.ServerPort`. Uses `./main.go` path (Dockerfile fix).
- **standard/main.tmpl**: currently print + telemetry/gRPC + `select{}` stub → conditional dispatch; `runServer()` starts HTTP on `cfg.ServerPort`, keeps the gRPC goroutine, uses `select{}` to block (web pattern at web/main.tmpl:31-40, 58-60).
- **hexagonal/main.tmpl**: SAME treatment as standard (distinct file — scaffold.go:345 creates `hexagonal/main.tmpl`, NOT shared with standard). `runServer()` starts HTTP + gRPC goroutine + `select{}`.
- **web/main.tmpl**: inline `os.Getenv("SERVER_PORT")` → `cfg := config.Load()` + `cfg.ServerPort`; conditional dispatch; keeps static file serving + gRPC goroutine.

### 6. Modified common templates

- **config.tmpl**: remove `generated_at: {{ now }}`, add `scaffold_prod_v1: true` (deterministic).
- **env.tmpl**: `POSTGRES_USER`/`POSTGRES_PASSWORD` only when DBDriver == PostgreSQL. For MongoDB, add a comment: "Migrations are not scaffolded for MongoDB projects."
- **go.mod.tmpl**: add pinned `require` blocks — `github.com/jackc/pgx/v5 v5.7.1` (PostgreSQL), `github.com/go-sql-driver/mysql v1.8.1` (MySQL) (D6).
- **docker-compose.yaml.tmpl**: DATABASE_URL per driver; volume per driver.
- **Dockerfile.tmpl**: build path conditional (`./main.go` Minimalist vs `./cmd/api/main.go`).

## Scaffold Wiring (scaffold.go)

- `createCommonFiles` (scaffold.go:352-410): always create `internal/config/config.go` (config_go.tmpl); when `DBDriver == PostgreSQL || MySQL`: create `internal/dbmigrate/migrate.go` (dbmigrate_go.tmpl — D7: filename is migrate.go per proposal) + `internal/dbmigrate/migrations/0001_init.sql` (migration_sql.tmpl).
- `config.tmpl` hardcodes `scaffold_prod_v1: true` (deterministic, no ProjectConfig render needed).
- `ProjectConfig` gains `ScaffoldProdV1 bool mapstructure:"scaffold_prod_v1"` — used ONLY by the upgrade-side read (configFromViper → viper.GetBool("scaffold_prod_v1")), never rendered.
- The 4 mains are wired per architecture (scaffold.go:303, 324, 345, web branch) — only template content changes.

## Upgrade Injection (upgrade.go)

Mirror the routes.go absent→create precedent (upgrade.go:259-273) in the post-classify loop:

```
after classification loop:
  if cfg.ScaffoldProdV1 {            // marker from .go-arch.yaml (viper.GetBool)
    for each injectable package (internal/config, internal/dbmigrate, internal/dbmigrate/migrations/0001_init.sql):
      if file absent on disk AND the re-rendered main imports it:
        render via renderInjectedPackage(engine, cfg, tmpl)   // follows renderRoutesRegistry (upgrade.go:281-292), NOT renderEntry
        append FileAction{Path, Classification: ClassUpgradable, RerenderHash, RerenderBytes, TemplatePath: <the actual tmpl>}
  }
```

- **FileAction.TemplatePath (blocker d/Gap1)**: add `TemplatePath string json:",omitempty"` to FileAction; Apply() (upgrade.go:418-424) seeds the manifest entry from it, falling back to `common/routes.tmpl` for the legacy routes.go case. This prevents injected config.go from getting a routes.tmpl path (which would re-render routes content over config.go next upgrade).
- **Migrations SQL injected with runner (blocker d/Gap2)**: `//go:embed migrations/*.sql` with zero files is a COMPILE ERROR — always inject `internal/dbmigrate/migrations/0001_init.sql` together with `migrate.go`.
- **Marker contract (blocker b)**: only marker'd projects reach injection; their go.mod already carries driver pins (D4 resolved).

## File Change Plan

| File | Action | What |
|------|--------|------|
| `internal/pkg/template/templates/common/config_go.tmpl` | Create | Typed config |
| `internal/pkg/template/templates/common/dbmigrate_go.tmpl` | Create | Migrations runner (go:embed, VARCHAR(255), tx) |
| `internal/pkg/template/templates/common/migration_sql.tmpl` | Create | 0001_init.sql — REAL executable statement |
| `internal/pkg/template/templates/common/main_dispatch.tmpl` | Create | Shared dispatch snippet |
| `internal/pkg/template/templates/{minimalist,standard,hexagonal,web}/main.tmpl` | Modify | Conditional dispatch + config integration (ALL FOUR) |
| `internal/pkg/template/templates/common/config.tmpl` | Modify | Remove `generated_at`, add `scaffold_prod_v1: true` |
| `internal/pkg/template/templates/common/env.tmpl` | Modify | POSTGRES_* only PostgreSQL; Mongo no-migrations comment |
| `internal/pkg/template/templates/common/go.mod.tmpl` | Modify | Driver pins (pgx v5.7.1, mysql v1.8.1) |
| `internal/pkg/template/templates/common/docker-compose.yaml.tmpl` | Modify | DATABASE_URL + volume per driver |
| `internal/pkg/template/templates/common/Dockerfile.tmpl` | Modify | Build path conditional |
| `internal/pkg/scaffold/scaffold.go` | Modify | createCommonFiles wiring + ProjectConfig.ScaffoldProdV1 field |
| `internal/pkg/scaffold/upgrade.go` | Modify | Injection loop + FileAction.TemplatePath + renderInjectedPackage |
| `internal/pkg/scaffold/upgrade_opts.go` | Modify | (if marker needs an option) |
| `internal/pkg/scaffold/scaffold_test.go` | Modify | Per-arch/driver file matrix (incl. hexagonal) |
| `internal/pkg/scaffold/upgrade_test.go` | Modify | Injection tests (marker'd fixture) |
| `internal/pkg/template/engine_test.go` | Modify | Determinism (no now in re-rendered files) |

## Testing Strategy

- **Scaffold matrix**: for each architecture × DBDriver × UseTemplHTMX — config.go always exists; dbmigrate only when DBDriver ∈ {PostgreSQL, MySQL}; no dbmigrate for None/MongoDB; marker present; ALL FOUR mains have dispatch (incl. hexagonal); Dockerfile path per arch; compose DATABASE_URL per driver; env no POSTGRES_* leak; Mongo env has no-migrations comment.
- **Upgrade injection**: MARKER'D fixture with packages deleted → upgrade → main re-renders to dispatch, internal/config + internal/dbmigrate + migrations/0001_init.sql injected (TemplatePath correct), project compiles; no-marker project → main re-renders to byte-identical legacy → up_to_date, nothing injected.
- **Determinism**: render each template twice → byte-identical (no `now` in re-rendered files).
- **Live smoke (verify)**: (1) PostgreSQL project → `./main migrate` against a temp DB → 0001_init applies; re-run → idempotent; `./main version`; unknown → exit 2. (2) **MySQL project → `./main migrate` against temp MySQL → applies (validates VARCHAR PK + real statement + `?` placeholder)** (blocker c). (3) Minimalist+Docker → Dockerfile builds `./main.go`.

## Key Risks

- **Upgrade re-render compile break** (HIGHEST): mitigated by conditional main rendering (marker) + marker-gated injection + fixture test.
- **go.mod report-only**: driver deps only in NEW scaffolds; injection only reaches marker'd projects (which have the pins).
- **MySQL dialect**: VARCHAR(255) PK + real 0001_init statement + `?` placeholder — validated by the MySQL live smoke.
- **Determinism**: `generated_at` removal is required (flagged refinement).
- **Scope**: 3 units + bug fixes → ~1100-1600 lines → chained PRs mandatory.

## Slice Boundaries (chained PRs)

| Slice | Content | Est. |
|-------|---------|------|
| S1 | Typed config template + wiring + marker + determinism (remove generated_at) + ProjectConfig field + tests | ~300 |
| S2 | Conditional subcommand dispatch in 4 mains + Dockerfile path fix + tests | ~400 |
| S3 | Migrations runner (migrate.go + migration_sql) + driver pins + compose/env fixes + tests | ~400 |
| S4 | Upgrade injection (FileAction.TemplatePath + renderInjectedPackage + marker read) + upgrade tests + docs | ~350 |

Tracker `feat/scaffold-prod`; slices → `feat/scaffold-prod-{1..4}`; only tracker merges to main.
