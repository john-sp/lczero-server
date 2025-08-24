# LCZero Server Database Schema

Human-readable reference generated from `schema.sql` to speed up iterative schema improvements. This is a working document, not final. Fields without `NOT NULL` in DDL are nullable by default.

Conventions:
- PK = Primary Key, FK = Foreign Key, NN = NOT NULL, UQ = UNIQUE
- Types are PostgreSQL types as defined in `schema.sql`

---

## Legacy (read-only) tables
These exist to support migration from the old HTTP-based system. Treat as read-only in the new stack and plan for deprecation after migration.

### users
- Purpose: Legacy user credentials; used only to migrate to token-based auth.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- username (TEXT)
	- password (TEXT)
	- assigned_training_run_id (BIGINT)

### clients
- Purpose: Legacy client records; used only to migrate to token-based client identity.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- user_id (BIGINT, FK -> users.id)
	- client_name (TEXT)
	- last_version (BIGINT)
	- last_engine_version (TEXT)
	- last_game_at (TIMESTAMPTZ)
	- gpu_name (TEXT)

---

## Legacy training tables

### networks
- Purpose: Neural network artifacts produced by training; candidates in matches.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- training_run_id (BIGINT)
	- network_number (BIGINT)
	- sha (TEXT)
	- path (TEXT)
	- layers (INTEGER)
	- filters (INTEGER)
	- games_played (INTEGER)
	- elo (DOUBLE PRECISION)
	- anchor (BOOLEAN)
	- elo_set (BOOLEAN)

### training_runs
- Purpose: Top-level container grouping networks, matches, books, and parameters.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- best_network_id (BIGINT, FK -> networks.id)
	- description (TEXT)
	- train_parameters (TEXT)
	- match_parameters (TEXT)
	- train_book (TEXT)
	- match_book (TEXT)
	- active (BOOLEAN)
	- last_network (BIGINT)
	- last_game (BIGINT)
	- permission_expr (TEXT)
	- multi_net_mode (BOOLEAN)

### matches
- Purpose: Candidate-vs-best matches within a training run.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- training_run_id (BIGINT)
	- parameters (TEXT)
	- candidate_id (BIGINT, FK -> networks.id)
	- current_best_id (BIGINT, FK -> networks.id)
	- games_created (INTEGER)
	- wins (INTEGER)
	- losses (INTEGER)
	- draws (INTEGER)
	- game_cap (INTEGER)
	- done (BOOLEAN)
	- passed (BOOLEAN)
	- test_only (BOOLEAN)
	- special_params (BOOLEAN)
	- target_slice (INTEGER)

### match_games
- Purpose: Individual games belonging to a match.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- user_id (BIGINT)
	- match_id (BIGINT, FK -> matches.id)
	- version (BIGINT)
	- pgn (TEXT)
	- result (INTEGER)
	- done (BOOLEAN)
	- flip (BOOLEAN)
	- engine_version (TEXT)

### training_games
- Purpose: Self-play training games used to train networks.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ)
	- user_id (BIGINT, FK -> users.id)
	- client_id (BIGINT, FK -> clients.id)
	- training_run_id (BIGINT, FK -> training_runs.id)
	- network_id (BIGINT, FK -> networks.id)
	- game_number (BIGINT)
	- version (BIGINT)
	- compacted (BOOLEAN)
	- engine_version (TEXT)
	- resign_fp_threshold (DOUBLE PRECISION)
- Indexes:
	- idx_training_games_created_at (created_at)
	- idx_training_games_user_id (user_id)
	- idx_training_games_client_id (client_id)
	- idx_training_games_network_id (network_id)

### server_data
- Purpose: Honestly, no idea what is in here.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- training_pgn_uploaded (INTEGER)

---

## Authentication and identity

### auth_tokens
- Purpose: Token-based authentication for users and clients.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- token (TEXT, UQ)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- user_id (BIGINT, FK -> users.id) — may be NULL for anonymous tokens
	- last_used_at (TIMESTAMPTZ)
	- issued_reason (TEXT) — e.g., "anonymous", "migrated_credentials", "django_auth"
	- client_version (TEXT)
	- client_host (TEXT)
	- gpu_type (TEXT)
	- gpu_id (INTEGER)
- Indexes:
	- idx_auth_tokens_user_id (user_id)
	- idx_auth_tokens_last_used_at (last_used_at)
- Notes:
	- Comment hints at possibly breaking FK to connect to Django auth; decide on auth source of truth.
	- Consider expirable/rotating tokens, and `revoked_at` field.

---

## Books

### books
- Purpose: Opening books referenced by tasks/training.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- sha256 (TEXT, UQ, NN)
	- url (TEXT)
	- size_bytes (BIGINT)
	- format (TEXT)
- Notes:
	- Consider NN on `url` if required; add content-addressed storage policy.

---

## Tasks subsystem
High-level orchestration for training, matches, SPRT, and tuning. `tasks` is the base table; specific task tables extend it 1:1 via `task_id` (UQ, FK, ON DELETE CASCADE).

### tasks
- Purpose: Polymorphic base for all high-level tasks.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- task_type (TEXT) — e.g., "TRAINING", "SPRT", "TUNE"
	- status (TEXT)
	- description (TEXT)
- Notes:
    - Consider priority system
	- Consider ENUMs for `task_type` and `status`, and indexes on (`task_type`, `status`).

### training_tasks
- Purpose: Configuration for an ongoing training run.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- task_id (BIGINT, UQ, FK -> tasks.id, ON DELETE CASCADE)
	- training_run_id (BIGINT, FK -> training_runs.id)
	- train_book_id (BIGINT, FK -> books.id)
	- match_book_id (BIGINT, FK -> books.id)
	- train_parameters (TEXT)
	- match_parameters (TEXT)
	- best_network_id (BIGINT, FK -> networks.id)
- Notes:
	- 

### match_tasks
- Purpose: Encodes a match job under a training task.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- task_id (BIGINT, UQ, FK -> tasks.id, ON DELETE CASCADE)
	- training_task_id (BIGINT, FK -> training_tasks.id)
	- candidate_network_id (BIGINT, FK -> networks.id)
	- current_best_network_id (BIGINT, FK -> networks.id)
	- games_created (INTEGER)
	- wins (INTEGER)
	- losses (INTEGER)
	- draws (INTEGER)
	- done (BOOLEAN)
	- passed (BOOLEAN)
- Notes:
	- Consider UQ on (`training_task_id`, `candidate_network_id`) to prevent duplicates.

### sprt_tasks
- Purpose: Sequential Probability Ratio Test matches between baseline and candidate.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- task_id (BIGINT, UQ, FK -> tasks.id, ON DELETE CASCADE)
	- baseline_network_id (BIGINT, FK -> networks.id)
	- baseline_params_args (TEXT)
	- baseline_params_uci_options (TEXT)
	- candidate_network_id (BIGINT, FK -> networks.id)
	- candidate_params_args (TEXT)
	- candidate_params_uci_options (TEXT)
	- opening_book_id (BIGINT, FK -> books.id)
	- time_control_type (VARCHAR(32))
	- base_time_seconds (DOUBLE PRECISION)
	- increment_seconds (DOUBLE PRECISION)
	- nodes_per_move (BIGINT)
- Notes:
    - See https://github.com/LeelaChessZero/OpenBench/blob/master/OpenBench/models.py for needed info. 
        - Fields to add in some way: 
            -wins
            - losses
            - draws
            - games (WDL all added up, maybe dont count, compute at run time)
            - crashes (Should never happen, but should track)
            - timeouts
        - Equivilant of Results (aka match_games ish from above, don't store PGNs)
	- Consider validating time control fields; possibly normalize time control into a separate type/table.
        - Openbench has a scaled time control system, maybe adopt it.
    - There are lots of networks that will be created by training, maybe find a way to mark networks as special so they are easier to select on website. Potentially include best network of each training run, plus dev uploads. (Maybe devs can only see their own uploads, plus Mod/Admin approved networks (Like BT4 and the like)).

### tune_tasks
- Purpose: Parameter tuning jobs for engines/builds.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- created_at (TIMESTAMPTZ, NN)
	- updated_at (TIMESTAMPTZ, NN)
	- task_id (BIGINT, UQ, FK -> tasks.id, ON DELETE CASCADE)
	- build_repo_url (TEXT)
	- build_commit_hash (TEXT)
	- build_params (TEXT)
	- tune_network_id (BIGINT, FK -> networks.id)
	- opening_book_id (BIGINT, FK -> books.id)
	- games_per_param_set (INTEGER)
	- time_control_type (VARCHAR(32))
	- base_time_seconds (DOUBLE PRECISION)
	- increment_seconds (DOUBLE PRECISION)
	- nodes_per_move (BIGINT)
- Notes:
    - find a way to store outputs
	- Build provenance fields may merit a separate `builds` table with UQ on (repo, commit) and reuse across tasks.

### tune_param_sets
- Purpose: Parameter sets evaluated within a tuning task.
- Columns:
	- id (BIGSERIAL, PK, NN)
	- tune_task_id (BIGINT, FK -> tune_tasks.id)
	- param_set_id (TEXT)
	- params_args (TEXT)
	- params_uci_options (TEXT)
- Notes:
	- Consider UQ on (`tune_task_id`, `param_set_id`).

