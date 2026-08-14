# Proposal: scaffold-prod

## Intent

Scaffolded projects ship no Go code for `db_driver`, no typed config, and flat mains with inline `os.Getenv`. This change makes new projects production-ready: a stdlib `internal/config` with fail-fast validation, a subcommand-aware `main`, and a zero-dep SQL migrations runner for Postgres/MySQL — while fixing three latent compose/Dockerfile bugs that break DB and Minimalist flows.

## Scope

### In Scope
- Typed `internal/config` (stdlib; driver-conditional `DATABASE_URL` fail-fast).
- Subcommand `main` (stdlib dispatch: default `server`, `migrate`, `version`; exit 2 unknown).
- SQL migrations runner (`internal/dbmigrate` + `go:embed migrations/*.sql`, Up-only).
- Upgrade-inject mechanism extending the `routes.go` precedent (upgrade.go:123-134, 259-273) so old projects receive `internal/config` and `internal/dbmigrate` when the rewritten main references them.
- Bundled bug fixes: `docker-compose.yaml.tmpl` DATABASE_URL hardcoded to postgres (line 12) + postgres-only volume path (line 59); `Dockerfile.tmpl` main path for Minimalist (line 16); `env.tmpl` `POSTGRES_*` leak into MySQL/Mongo (lines 16-17).

### Out of Scope
- `create-admin` — domain-specific; not scaffold-able.
- Cobra/Viper in scaffolded projects — breaks upgrade constraint (go.mod report-only).
- MongoDB migrations — schemaless.
- Down migrations — P2 is Up-only.
- New `go-arch` CLI commands or generators.

## Capabilities

### New Capabilities
- `typed-config`: `internal/config/config.go` template — `Config{ServerPort, AppEnv, DatabaseURL}`, stdlib `Load()`, defaults (8080, "development"), fail-fast on missing `DATABASE_URL` when `db_driver != None`.
- `subcommand-dispatch`: rewrite of all four `main.tmpl` variants to stdlib `switch os.Args[1]`; `server` is default (preserves `CMD ["./main"]`); `runServer()` hosts existing HTTP-start logic.
- `migrations-runner`: `internal/dbmigrate/migrate.go` + embedded `migrations/0001_init.sql` (idempotent placeholder), driver pin in `go.mod.tmpl` (pgx/v5, go-sql-driver/mysql), `migrate` subcommand calls `dbmigrate.Up`.

### Modified Capabilities
- `cli`: scaffold file set expands; `createCommonFiles` wires config/migrations; env and compose templates lose the POSTGRES_* leak and hardcoded-postgres compose bugs.
- `upgrade-project`: add "inject missing scaffold-owned files" path — when the re-rendered main imports `internal/config` or `internal/dbmigrate` and those packages are absent on disk, create them (mirrors upgrade.go:123-134, 259-273).

## Approach

**Config shape.** `Config{ServerPort int, AppEnv string, DatabaseURL string}`. `Load()` reads `os.Getenv`, applies defaults, parses `SERVER_PORT` via `strconv.Atoi`, and — only when `{{ if ne .DBDriver "None" }}` — requires `DATABASE_URL` non-empty. Stdlib only.

**Subcommand dispatch.** Every `main.tmpl`: `len(os.Args) < 2 → runServer(); else switch os.Args[1] { server → runServer, migrate → dbmigrate.Up or stderr-exit-1 if no DB, version → print, default → stderr-exit-2 }`. Default-to-server preserves Dockerfile `CMD` and `go run .`. `web/main.tmpl` inline `os.Getenv("SERVER_PORT")` replaced by `cfg.ServerPort`.

**Migrations runner.** `internal/dbmigrate` owns `//go:embed migrations/*.sql` (embed cannot traverse `..`). `Up(driverName, dsn)` opens `database/sql`, creates `schema_migrations` if missing, sorts embedded filenames, skips applied, executes each in a tx, records. `0001_init.sql` ships as a commented idempotency-teaching placeholder. Driver blank import is chosen per `db_driver`; runner itself stays `database/sql`-only.

**Upgrade injection.** Add a post-classify loop in `Upgrade`: for each manifest entry whose re-rendered content imports `internal/config` or `internal/dbmigrate`, if that package is absent on disk, render+write it with `origin: scaffold` and a fresh manifest entry. Extends, not replaces, the routes.go precedent.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/pkg/template/templates/common/config_go.tmpl` | New | typed config template |
| `internal/pkg/template/templates/common/dbmigrate_go.tmpl` | New | migrations runner template |
| `internal/pkg/template/templates/common/migration_sql.tmpl` | New | `0001_init.sql` template |
| `templates/{minimalist,standard,hexagonal,web}/main.tmpl` | Modified | subcommand dispatch + config load |
| `templates/common/{env,docker-compose.yaml,Dockerfile,go.mod}.tmpl` | Modified | POSTGRES_* leak, compose DATABASE_URL+volume, Dockerfile Minimalist path, driver pin |
| `internal/pkg/scaffold/scaffold.go` | Modified | `createCommonFiles` wires new files |
| `internal/pkg/scaffold/upgrade.go` | Modified | inject-missing-files loop |
| `scaffold_test.go`, `engine_test.go` | Modified | new files + driver matrix + upgrade injection |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Upgrade re-render of old mains fails to compile | High | stdlib-only dispatch; inject-missing-files covers new packages; fixture-based upgrade test |
| Review budget overflow (~1,100–1,600 lines) | High | tasks phase chains 3 PRs (config → subcommands → migrations) |
| Test churn on exact file-set assertions | Medium | update `TestScaffolder_Layouts` + web-main content asserts alongside each PR |
| `go:embed` misplacement breaks build | Low | template places SQL under `internal/dbmigrate/migrations/`; spec asserts path |

## Rollback Plan

Each PR in the chain reverts independently. The inject-missing-files loop is gated on an opt-in marker in `.go-arch.yaml` (`scaffold_prod_v1: true`) added by PR1 — disabling the flag skips injection and the old main shape can be restored by a one-shot patch. `internal/config` and `internal/dbmigrate` are purely additive and never overwrite user files (PROTECTED class in `upgrade-project` spec).

## Dependencies

- Pin `github.com/jackc/pgx/v5` (latest at spec time) and `github.com/go-sql-driver/mysql` ≥ v1.8 in `go.mod.tmpl` for new scaffolds only.
- Spec phase verifies pgx/mysql release versions; design phase locks exact minor.

## Success Criteria

- [ ] `go-arch new` with any arch + Postgres/MySQL emits `internal/config/config.go`, `internal/dbmigrate/migrate.go`, `internal/dbmigrate/migrations/0001_init.sql`, and a working `go.mod` with the driver.
- [ ] Generated project builds with `go build ./...` and runs both `./main` (default server) and `./main migrate`.
- [ ] `go-arch upgrade` on a pre-change fixture injects the new packages and the rewritten main compiles.
- [ ] `go test ./...` passes with driver-matrix coverage (Postgres/MySQL/None/Mongo).
- [ ] No new template uses `now` or randomness; re-render of unchanged inputs is byte-identical.
