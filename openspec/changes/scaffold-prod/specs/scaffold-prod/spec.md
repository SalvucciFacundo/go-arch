# Delta for scaffold-prod

## ADDED Requirements

### Requirement: Typed Config Package (typed-config)

Every scaffolded project MUST generate an `internal/config` package with a `Config` struct containing `ServerPort string`, `AppEnv string`, and `DatabaseURL string`. The package MUST provide a stdlib-only `Load() (*Config, error)` that reads `SERVER_PORT` (default `"8080"`), `APP_ENV` (default `"development"`), and `DATABASE_URL` from the environment. When the project was generated with a database driver other than `None`, `Load()` MUST fail fast with an error naming the missing variable and pointing to `.env.example` if `DATABASE_URL` is unset. All generated code MUST be deterministic (no timestamps, no randomness) so upgrade re-render is byte-stable.

#### Scenario: Default config loads

- GIVEN a generated project with no environment variables set
- WHEN `config.Load()` runs
- THEN it returns `Config{ServerPort: "8080", AppEnv: "development", DatabaseURL: ""}` with no error

#### Scenario: Env vars override defaults

- GIVEN `SERVER_PORT=9000` and `APP_ENV=production` in the environment
- WHEN `config.Load()` runs
- THEN it returns `ServerPort: "9000"` and `AppEnv: "production"`

#### Scenario: Missing DATABASE_URL fails fast for DB projects

- GIVEN a project generated with `db_driver: PostgreSQL`
- AND `DATABASE_URL` is not set
- WHEN `config.Load()` runs
- THEN it returns an error naming `DATABASE_URL`
- AND the error message points to `.env.example`

#### Scenario: No database means no fail-fast

- GIVEN a project generated with `db_driver: None`
- AND `DATABASE_URL` is not set
- WHEN `config.Load()` runs
- THEN it returns a config with empty `DatabaseURL` and no error

### Requirement: Subcommand Dispatch in Main (subcommand-dispatch)

Every generated `main.go` / `cmd/api/main.go` MUST dispatch on `os.Args[1]` with a stdlib `switch`. The default (no argument) MUST run the HTTP server — preserving the Dockerfile `CMD ["./main"]` contract. The `server` subcommand MUST run the same HTTP server logic explicitly. The `migrate` subcommand MUST run the migrations runner (only when the project has a database). The `version` subcommand MUST print a version string. Any unknown subcommand MUST print a usage hint and exit with code 2. The generated code MUST use only the standard library (no Cobra) so `go.mod` stays report-only-safe on upgrade.

#### Scenario: No argument runs the server

- GIVEN a generated project
- WHEN it is executed as `./main`
- THEN the HTTP server starts on the configured port

#### Scenario: Explicit server subcommand

- GIVEN a generated project
- WHEN it is executed as `./main server`
- THEN the HTTP server starts on the configured port

#### Scenario: Version subcommand

- GIVEN a generated project
- WHEN it is executed as `./main version`
- THEN it prints a version string and exits 0

#### Scenario: Unknown subcommand exits 2

- GIVEN a generated project
- WHEN it is executed as `./main frobnicate`
- THEN it prints a usage hint listing valid subcommands
- AND it exits with code 2

#### Scenario: Migrate subcommand with database

- GIVEN a project generated with `db_driver: PostgreSQL`
- WHEN it is executed as `./main migrate`
- THEN the migrations runner executes pending migrations

### Requirement: Migrations Runner (migrations-runner)

When a project is generated with a database driver other than `None`, the scaffold MUST generate an `internal/dbmigrate` package that embeds migration SQL files via `//go:embed migrations/*.sql` (SQL files MUST live inside the runner package directory — go:embed cannot traverse above it). The package MUST provide `Up(driverName, dsn string) error` using `database/sql` and a `schema_migrations` tracking table so re-running is idempotent (already-applied migrations are skipped). The runner MUST support PostgreSQL (via `pgx/v5` stdlib driver) and MySQL (via `go-sql-driver/mysql`). MongoDB projects MUST NOT generate a runner; instead the README MUST note that migrations are not scaffolded for MongoDB.

#### Scenario: Migrations embedded and idempotent

- GIVEN a PostgreSQL project with `internal/dbmigrate/migrations/0001_init.sql`
- WHEN `dbmigrate.Up("pgx", dsn)` runs twice
- THEN both runs succeed
- AND the second run skips the already-applied migration (schema_migrations records it)

#### Scenario: MySQL project has runner

- GIVEN a project generated with `db_driver: MySQL`
- WHEN the scaffold runs
- THEN `internal/dbmigrate` exists with the MySQL driver blank import

#### Scenario: MongoDB has no runner

- GIVEN a project generated with `db_driver: MongoDB`
- WHEN the scaffold runs
- THEN no `internal/dbmigrate` package is generated
- AND the README notes migrations are not scaffolded for MongoDB

#### Scenario: No database has no runner

- GIVEN a project generated with `db_driver: None`
- WHEN the scaffold runs
- THEN no `internal/dbmigrate` package is generated

### Requirement: Upgrade Injection of New Packages (upgrade-injection)

The `upgrade` command MUST extend its routes.go absent→create precedent: during the post-classification loop, when a re-rendered main imports `internal/config` or `internal/dbmigrate` and the corresponding package directory is absent on disk, the CLI MUST render and write those scaffold-owned files with `origin: scaffold` so the upgraded project still compiles. This injection MUST be gated by a `.go-arch.yaml` marker `scaffold_prod_v1: true` written when the feature was scaffolded. Projects without the marker MUST NOT receive injected packages (they keep their legacy mains).

#### Scenario: Old project upgrades to subcommand main

- GIVEN a pre-feature project (no `scaffold_prod_v1` marker) with an old flat main
- WHEN `go-arch upgrade` runs
- THEN the main is re-rendered to the subcommand version
- AND `internal/config` is written with `origin: scaffold` because it is absent
- AND the project still compiles

#### Scenario: Marker gates injection

- GIVEN a project without the `scaffold_prod_v1` marker
- WHEN `go-arch upgrade` runs
- THEN no new packages are injected
- AND the legacy main is preserved

#### Scenario: New scaffolds always have the marker

- GIVEN a newly scaffolded project
- THEN `.go-arch.yaml` contains `scaffold_prod_v1: true`
- AND `internal/config` exists from the start

### Requirement: Compose and Dockerfile Driver Fixes (compose-dockerfile-fixes)

The generated `docker-compose.yaml` MUST build the `DATABASE_URL` from the actual driver (not hardcoded PostgreSQL) and MUST mount the database volume at the driver-correct path (PostgreSQL `/var/lib/postgresql/data`, MySQL `/var/lib/mysql`). The generated `Dockerfile` MUST build the correct main path per architecture: `./main.go` for Minimalist, `./cmd/api/main.go` for Standard and Hexagonal. The generated `.env.example` MUST NOT leak `POSTGRES_USER`/`POSTGRES_PASSWORD` into projects without a PostgreSQL database.

#### Scenario: Minimalist Dockerfile builds main.go

- GIVEN a Minimalist project generated with `use_docker: true`
- WHEN the Dockerfile is generated
- THEN the build command is `go build -o main ./main.go`

#### Scenario: Standard Dockerfile builds cmd/api

- GIVEN a Standard project generated with `use_docker: true`
- WHEN the Dockerfile is generated
- THEN the build command is `go build -o main ./cmd/api/main.go`

#### Scenario: MySQL compose URL and volume

- GIVEN a project generated with `db_driver: MySQL`
- WHEN docker-compose.yaml is generated
- THEN `DATABASE_URL` uses the MySQL driver prefix
- AND the volume mounts at `/var/lib/mysql`

#### Scenario: No PostgreSQL means no POSTGRES_* leak

- GIVEN a project generated with `db_driver: None` or `db_driver: MySQL`
- WHEN `.env.example` is generated
- THEN it does not contain `POSTGRES_USER` or `POSTGRES_PASSWORD`

### Requirement: Driver Dependencies (driver-pins)

New scaffolds generated with a database driver MUST pin the driver library in `go.mod`: `github.com/jackc/pgx/v5` for PostgreSQL and `github.com/go-sql-driver/mysql` for MySQL. This applies to NEW scaffolds only — `go.mod` remains report-only on upgrade, so existing projects are not modified.

#### Scenario: PostgreSQL scaffold pins pgx

- GIVEN a new project generated with `db_driver: PostgreSQL`
- WHEN go.mod is generated
- THEN it requires `github.com/jackc/pgx/v5`

#### Scenario: MySQL scaffold pins mysql driver

- GIVEN a new project generated with `db_driver: MySQL`
- WHEN go.mod is generated
- THEN it requires `github.com/go-sql-driver/mysql`

#### Scenario: No database has no driver dep

- GIVEN a new project generated with `db_driver: None`
- WHEN go.mod is generated
- THEN no DB driver dependency is present
