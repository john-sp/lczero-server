AI written Readme. Update this more as needed.

# LCZero gRPC Training Server

Rewrite of the LCZero training server using gRPC. It interacts with https://dev.lczero.org and removes all website components from the legacy HTTP server, adding richer tasking (training, match, SPRT, tuning).

- Language: Go 1.23
- RPC: gRPC + Protocol Buffers (see `api/v1/lczero.proto`)
- DB: PostgreSQL (schema in `schema.sql`; human guide in `schema.md`)
- Entry point: `cmd/main.go`

Our old training setup was a pain: starting a new run meant hand-editing the database, fiddling with YAML, and clobbering whatever was in "Run 0." We also had a separate [OpenBench](https://github.com/LeelaChessZero/OpenBench/) instance for [SPRT](https://www.chessprogramming.org/Match_Statistics#SPRT) and tuning hyperparameters was done individually via [Chess Tuning Tools](https://chess-tuning-tools.readthedocs.io/en/latest/) (let’s be real, it was mostly devs running this and posting results on Discord).

This rewrite aims to fix all that—bringing everything (training, matches, tuning, and SPRT) into one unified, distributed system that’s actually flexible and easy to experiment with. It is taking heavy inspiration from OpenBench, just expanded to fit our need. 

## Architecture
- Services: `AuthService` (token issuance), `TaskService` (task assignment + data collection)
- Packages:
	- `internal/config`: loads `serverconfig.json`
	- `internal/db`: Postgres connection
	- `internal/server`: gRPC services (`auth_service.go`, `task_service.go`)
	- `internal/models`, `internal/db/queries`: model structs and SQL helpers
	- `api/v1`: protobuf (`.proto` + generated `.pb.go`)

## Prerequisites
- Go 1.23.x
- PostgreSQL 13+ (14+ recommended)
- protoc (only if re-generating protobuf code)

## Configuration
Copy and edit `serverconfig.json`.

Relevant fields (mapped by `internal/config/config.go`):
- `database.host|user|dbname|password`
- `webserver.address` (e.g., `":9830"`)
- Client/engine version gates and URLs for artifacts

## Database setup
Theoretically, the database should be setup from the https://dev.lczero.org/ but here is a basic setup instructions.  

1) Create DB and user, then apply schema:

```powershell
# Example (adjust to your environment)
psql -h localhost -U postgres -c "CREATE ROLE lc0 WITH LOGIN PASSWORD 'lc0pass';"
psql -h localhost -U postgres -c "CREATE DATABASE lc0 OWNER lc0;"
psql -h localhost -U lc0 -d lc0 -f .\schema.sql
```

2) Optional: read `schema.md` for a human-friendly schema guide and improvement notes.

## Regenerate protobuf (optional)
```powershell
# Requires protoc and the Go plugins installed
protoc --go_out=. --go-grpc_out=. api/v1/lczero.proto
```

## Notes / roadmap
- Legacy `users`/`clients` are read-only (migration only). See `schema.md`.
- Several FKs and NOT NULLs are intentionally loose while iterating; see improvement list in `schema.md`.
- Task assignment logic is evolving (see TODOs in `internal/server/task_service.go`).


