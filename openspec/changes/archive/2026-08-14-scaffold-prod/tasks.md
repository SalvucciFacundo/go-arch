# Tasks: scaffold-prod (P2 production-ready scaffolding)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,450–1,650 (S1 ~300, S2 ~400, S3 ~400, S4 ~350) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 (feature-branch-chain) |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Slice 1 — Typed config + marker + determinism (PR 1 → tracker `feat/scaffold-prod`, ~300 lines)

- [ ] 1.1 RED `scaffold_test.go`: per-arch/driver matrix — config.go always exists; marker `scaffold_prod_v1: true` present in .go-arch.yaml; `generated_at` ABSENT
- [ ] 1.2 GREEN `config_go.tmpl` (NEW): `internal/config` — Config{ServerPort, AppEnv, DatabaseURL}, stdlib Load(), defaults 8080/development, fail-fast DATABASE_URL when DBDriver != None (points to .env.example)
- [ ] 1.3 GREEN `scaffold.go`: createCommonFiles always writes internal/config/config.go; ProjectConfig gains `ScaffoldProdV1 bool mapstructure:"scaffold_prod_v1"` (upgrade-side read only)
- [ ] 1.4 GREEN `config.tmpl`: remove `generated_at: {{ now }}`, add `scaffold_prod_v1: true`
- [ ] 1.5 Verify: scaffold matrix green + determinism (render config_go.tmpl twice → byte-identical)

### Slice 2 — Conditional subcommand dispatch in 4 mains + Dockerfile fix (PR 2 → `feat/scaffold-prod-1`, ~400 lines)

- [ ] 2.1 RED `scaffold_test.go`: ALL FOUR mains (minimalist, standard, hexagonal, web) contain the dispatch switch when marker set; legacy content when not
- [ ] 2.2 GREEN `main_dispatch.tmpl` (NEW): shared switch snippet — default=server, server/migrate/version, unknown → exit 2
- [ ] 2.3 GREEN `{minimalist,standard,hexagonal,web}/main.tmpl`: conditional `{{ if .ScaffoldProdV1 }}` dispatch + config `{{ else }}` legacy `{{ end }}`; runServer() starts HTTP on cfg.ServerPort; standard/hexagonal keep gRPC goroutine + select{}; web keeps static serving
- [ ] 2.4 GREEN `Dockerfile.tmpl`: build path conditional — Minimalist `./main.go`, else `./cmd/api/main.go`
- [ ] 2.5 Verify: matrix green + web main uses cfg.ServerPort

### Slice 3 — Migrations runner + driver pins + compose/env fixes (PR 3 → `feat/scaffold-prod-2`, ~400 lines)

- [ ] 3.1 RED `scaffold_test.go`: dbmigrate generated only for PostgreSQL|MySQL; NOT for None/MongoDB; migrate.go filename; migrations/0001_init.sql exists
- [ ] 3.2 GREEN `dbmigrate_go.tmpl` (NEW): go:embed migrations/*.sql, Up(driverName, dsn), schema_migrations VARCHAR(255) PRIMARY KEY, tx-wrapped exec+record, per-driver blank import (pgx stdlib / mysql), `?` placeholder
- [ ] 3.3 GREEN `migration_sql.tmpl` (NEW): 0001_init.sql with REAL executable statement (CREATE TABLE IF NOT EXISTS app_meta)
- [ ] 3.4 GREEN `scaffold.go`: createCommonFiles writes internal/dbmigrate/migrate.go + migrations/0001_init.sql when PostgreSQL|MySQL
- [ ] 3.5 GREEN `go.mod.tmpl`: pin pgx v5.7.1 (PostgreSQL), go-sql-driver/mysql v1.8.1 (MySQL)
- [ ] 3.6 GREEN `docker-compose.yaml.tmpl`: DATABASE_URL per driver; volume per driver (pg /var/lib/postgresql/data, mysql /var/lib/mysql)
- [ ] 3.7 GREEN `env.tmpl`: POSTGRES_* only PostgreSQL; Mongo no-migrations comment
- [ ] 3.8 Verify: matrix green + compose/env content assertions

### Slice 4 — Upgrade injection + docs (PR 4 → `feat/scaffold-prod-3`, ~350 lines)

- [ ] 4.1 RED `upgrade_test.go`: MARKER'D fixture with internal/config + internal/dbmigrate deleted → upgrade re-renders main to dispatch, injects config.go + migrate.go + migrations/0001_init.sql (FileAction.TemplatePath correct), project compiles; NO-marker fixture → main re-renders to byte-identical legacy → up_to_date, nothing injected
- [ ] 4.2 GREEN `upgrade.go`: injection loop (post-classify) gated on cfg.ScaffoldProdV1; `renderInjectedPackage(engine, cfg, tmpl)` helper following renderRoutesRegistry (NOT renderEntry)
- [ ] 4.3 GREEN `FileAction.TemplatePath string json:",omitempty"`: Apply() seeds manifest entry from it, fallback common/routes.tmpl for legacy routes.go
- [ ] 4.4 Verify: full suite + live smoke — PostgreSQL `./main migrate` (apply + idempotent + version + exit 2); MySQL `./main migrate` (validates VARCHAR PK + real statement); Minimalist Dockerfile path
- [ ] 4.5 Docs: README/COMMANDS mention subcommands + migrations; ROADMAP follow-up

## Commit Plan (conventional, tests with code)

- S1: `feat(scaffold): add typed config and scaffold_prod_v1 marker` + `test(scaffold): config presence and marker`
- S2: `feat(scaffold): subcommand dispatch in mains and Dockerfile path fix` + `test(scaffold): dispatch in all four mains`
- S3: `feat(scaffold): migrations runner, driver pins, compose and env fixes` + `test(scaffold): dbmigrate and driver matrix`
- S4: `feat(upgrade): inject config and migrations into marker'd projects` + `test(upgrade): injection and marker gate` + `docs: subcommands, migrations, ROADMAP`
