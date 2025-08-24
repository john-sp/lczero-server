-- schema.sql
CREATE TABLE users ( -- Read only DB, legacy table that stores HTTP version of user credentials. Only used to migrate existing credentials to tokens.
  id BIGSERIAL PRIMARY KEY,
  username TEXT,
  password TEXT,
  assigned_training_run_id BIGINT
);

CREATE TABLE clients ( -- Read only DB, legacy table that stores HTTP version of client information. Only used to migrate existing clients to tokens.
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  client_name TEXT,
  last_version BIGINT,
  last_engine_version TEXT,
  last_game_at TIMESTAMPTZ,
  gpu_name TEXT
);
CREATE INDEX idx_clients_user_id ON clients(user_id);


CREATE TABLE networks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  training_run_id BIGINT,
  network_number BIGINT,
  sha TEXT,
  path TEXT,
  layers INTEGER,
  filters INTEGER,
  games_played INTEGER,
  elo DOUBLE PRECISION,
  anchor BOOLEAN,
  elo_set BOOLEAN
);

CREATE TABLE training_runs (
  id BIGSERIAL PRIMARY KEY,
  best_network_id BIGINT REFERENCES networks(id),
  description TEXT,
  train_parameters TEXT,
  match_parameters TEXT,
  train_book TEXT,
  match_book TEXT,
  active BOOLEAN,
  last_network BIGINT,
  last_game BIGINT,
  permission_expr TEXT,
  multi_net_mode BOOLEAN
);


CREATE TABLE matches (
  id BIGSERIAL PRIMARY KEY,
  training_run_id BIGINT,
  parameters TEXT,
  candidate_id BIGINT REFERENCES networks(id),
  current_best_id BIGINT REFERENCES networks(id),
  games_created INTEGER,
  wins INTEGER,
  losses INTEGER,
  draws INTEGER,
  game_cap INTEGER,
  done BOOLEAN,
  passed BOOLEAN,
  test_only BOOLEAN,
  special_params BOOLEAN,
  target_slice INTEGER
);

CREATE TABLE match_games (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  user_id BIGINT,
  match_id BIGINT REFERENCES matches(id),
  version BIGINT,
  pgn TEXT,
  result INTEGER,
  done BOOLEAN,
  flip BOOLEAN,
  engine_version TEXT
);

CREATE TABLE training_games (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ,
  user_id BIGINT REFERENCES users(id),
  client_id BIGINT REFERENCES clients(id),
  training_run_id BIGINT REFERENCES training_runs(id),
  network_id BIGINT REFERENCES networks(id),
  game_number BIGINT,
  version BIGINT,
  compacted BOOLEAN,
  engine_version TEXT,
  resign_fp_threshold DOUBLE PRECISION
);
CREATE INDEX idx_training_games_created_at ON training_games(created_at);
CREATE INDEX idx_training_games_user_id ON training_games(user_id);
CREATE INDEX idx_training_games_client_id ON training_games(client_id);
CREATE INDEX idx_training_games_network_id ON training_games(network_id);

CREATE TABLE server_data (
  id BIGSERIAL PRIMARY KEY,
  training_pgn_uploaded INTEGER
);

-- New tables, can be modified as needed

CREATE TABLE auth_tokens (
  id BIGSERIAL PRIMARY KEY,
  token TEXT UNIQUE,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  user_id BIGINT REFERENCES users(id), -- Potentially null for anonymous tokens
    -- Potentially break the foreign key to connect to django's auth system
  last_used_at TIMESTAMPTZ,
  issued_reason TEXT, -- e.g., "anonymous", "migrated_credentials", "django_auth"
  client_version TEXT,
  client_host TEXT,
  gpu_type TEXT,
  gpu_id INTEGER
);
CREATE INDEX idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX idx_auth_tokens_last_used_at ON auth_tokens(last_used_at);

-- Book table
CREATE TABLE books (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  sha256 TEXT UNIQUE NOT NULL,
  url TEXT,
  size_bytes BIGINT,
  format TEXT
);

-- Task table (base for all high-level tasks)
CREATE TABLE tasks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  task_type TEXT, -- e.g., "TRAINING", "SPRT", "TUNE"
  status TEXT,
  description TEXT
);

-- TrainingTask table
CREATE TABLE training_tasks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  task_id BIGINT UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
  training_run_id BIGINT REFERENCES training_runs(id),
  train_book_id BIGINT REFERENCES books(id),
  match_book_id BIGINT REFERENCES books(id),
  train_parameters TEXT,
  match_parameters TEXT,
  best_network_id BIGINT REFERENCES networks(id)
);

-- MatchTask table
CREATE TABLE match_tasks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  task_id BIGINT UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
  training_task_id BIGINT REFERENCES training_tasks(id),
  candidate_network_id BIGINT REFERENCES networks(id),
  current_best_network_id BIGINT REFERENCES networks(id),
  games_created INTEGER,
  wins INTEGER,
  losses INTEGER,
  draws INTEGER,
  done BOOLEAN,
  passed BOOLEAN
);

-- SprtTask table
CREATE TABLE sprt_tasks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  task_id BIGINT UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
  baseline_network_id BIGINT REFERENCES networks(id),
  baseline_params_args TEXT,
  baseline_params_uci_options TEXT,
  candidate_network_id BIGINT REFERENCES networks(id),
  candidate_params_args TEXT,
  candidate_params_uci_options TEXT,
  opening_book_id BIGINT REFERENCES books(id),
  time_control_type VARCHAR(32),
  base_time_seconds DOUBLE PRECISION,
  increment_seconds DOUBLE PRECISION,
  nodes_per_move BIGINT
);

-- See https://github.com/LeelaChessZero/OpenBench/blob/master/OpenBench/models.py for better table definitions. Must decide what is needed. At minimum, the following tables should be considered:
-- Result (Most importantly, the wins, losses, draws, games (WDL all added up, maybe dont count, compute at run time), crashes, timeouts)

-- TuneTask table
CREATE TABLE tune_tasks (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  task_id BIGINT UNIQUE REFERENCES tasks(id) ON DELETE CASCADE,
  build_repo_url TEXT,
  build_commit_hash TEXT,
  build_params TEXT,
  tune_network_id BIGINT REFERENCES networks(id),
  opening_book_id BIGINT REFERENCES books(id),
  games_per_param_set INTEGER,
  time_control_type VARCHAR(32),
  base_time_seconds DOUBLE PRECISION,
  increment_seconds DOUBLE PRECISION,
  nodes_per_move BIGINT
);

-- TuneParamSet table
CREATE TABLE tune_param_sets (
  id BIGSERIAL PRIMARY KEY,
  tune_task_id BIGINT REFERENCES tune_tasks(id),
  param_set_id TEXT,
  params_args TEXT,
  params_uci_options TEXT
);

-- Notes: Tuning tasks will store data into Redis for processing externally.

