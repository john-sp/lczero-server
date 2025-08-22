// Package queries contains SQL query templates for task-related operations.
package queries

import (
	"database/sql"
	"time"

	"github.com/leelachesszero/lczero-server/internal/models"
)

// FetchActiveTrainingTask returns the first active training task.
func FetchActiveTrainingTask(db *sql.DB) (*models.TrainingTask, error) {
	row := db.QueryRow(`
SELECT id, task_id, training_run_id, train_book_id, match_book_id, best_network_id, train_parameters, match_parameters
FROM training_tasks
WHERE active = true
ORDER BY id ASC
LIMIT 1`)
	var tr models.TrainingTask
	err := row.Scan(&tr.ID, &tr.TaskID, &tr.TrainingRunID, &tr.TrainBookID, &tr.MatchBookID, &tr.BestNetworkID, &tr.TrainParameters, &tr.MatchParameters)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

// FetchNetworkByID returns a network by its ID.
func FetchNetworkByID(db *sql.DB, id uint) (*models.Network, error) {
	row := db.QueryRow(`
	SELECT id, created_at, training_run_id, network_number, sha, path, layers, filters, games_played, elo, anchor, eloset 
	FROM networks
	WHERE id = $1`, id)
	var net models.Network
	err := row.Scan(&net.ID, &net.CreatedAt, &net.TrainingRunID, &net.NetworkNumber, &net.Sha, &net.Path, &net.Layers, &net.Filters, &net.GamesPlayed, &net.Elo, &net.Anchor, &net.EloSet)
	if err != nil {
		return nil, err
	}
	return &net, nil
}

// FetchPendingMatch returns the first pending match for a training run and slice.
func FetchPendingMatch(db *sql.DB, trainingRunID uint, slice int) (*models.Match, error) {
	row := db.QueryRow(
		`SELECT id, created_at, training_run_id, candidate_id, current_best_id, games_created, wins, losses, draws, game_cap, done, passed, test_only, special_params, target_slice 
		FROM matches 
		WHERE done = false AND training_run_id = $1 AND (target_slice = 0 OR target_slice = $2) 
		ORDER BY id ASC LIMIT 1`,
		trainingRunID, slice,
	)
	var m models.Match
	err := row.Scan(&m.ID, &m.CreatedAt, &m.TrainingRunID, &m.CandidateID, &m.CurrentBestID, &m.GamesCreated, &m.Wins, &m.Losses, &m.Draws, &m.GameCap, &m.Done, &m.Passed, &m.TestOnly, &m.SpecialParams, &m.TargetSlice)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// InsertMatchGame inserts a new match game and returns its ID.
func InsertMatchGame(db *sql.DB, userID, matchID uint, done bool) (uint64, error) {
	var id uint64
	err := db.QueryRow(
		`INSERT INTO match_games (user_id, match_id, done) 
		VALUES ($1, $2, $3) 
		RETURNING id`,
		userID, matchID, done,
	).Scan(&id)
	return id, err
}

// UpdateMatchGameFlip sets the flip value for a match game.
func UpdateMatchGameFlip(db *sql.DB, id uint64, flip bool) error {
	_, err := db.Exec(`UPDATE match_games 
	SET flip = $1 
	WHERE id = $2`, flip, id)
	return err
}

// FetchNetworkSha returns the SHA for a network by ID.
func FetchNetworkSha(db *sql.DB, id uint) (string, error) {
	var sha string
	err := db.QueryRow(`SELECT sha FROM networks WHERE id = $1`, id).Scan(&sha)
	return sha, err
}

// FetchBookByID returns book details by ID.
func FetchBookByID(db *sql.DB, id uint) (sha string, url string, size int64, err error) {
	err = db.QueryRow(`SELECT sha256, url, size_bytes FROM books WHERE id = $1`, id).Scan(&sha, &url, &size)
	return
}

// InsertTaskAssignment inserts a new task assignment and returns its ID.
func InsertTaskAssignment(db *sql.DB, taskID string, taskType string, assignedTokenID uint, assignedAt, lastHeartbeatAt time.Time, status string) (uint, error) {
	var id uint
	err := db.QueryRow(
		`INSERT INTO task_assignments (task_id, task_type, assigned_token_id, assigned_at, last_heartbeat_at, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		taskID, taskType, assignedTokenID, assignedAt, lastHeartbeatAt, status,
	).Scan(&id)
	return id, err
}

// FetchTaskAssignmentByTaskID returns a task assignment by task_id.
func FetchTaskAssignmentByTaskID(db *sql.DB, taskID string) (*models.TaskAssignment, error) {
	row := db.QueryRow(
		`SELECT id, created_at, updated_at, task_id, task_type, assigned_token_id, assigned_at, last_heartbeat_at, status, cancelled_at, completed_at 
		FROM task_assignments 
		WHERE task_id = $1`, taskID)
	var t models.TaskAssignment
	err := row.Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.TaskID, &t.TaskType, &t.AssignedTokenID, &t.AssignedAt, &t.LastHeartbeatAt, &t.Status, &t.CancelledAt, &t.CompletedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTaskAssignmentHeartbeat updates the last_heartbeat_at for a task assignment.
func UpdateTaskAssignmentHeartbeat(db *sql.DB, id uint, now time.Time) error {
	_, err := db.Exec(`UPDATE task_assignments SET last_heartbeat_at = $1 WHERE id = $2`, now, id)
	return err
}

// UpdateAllTokensLastUsedAt sets last_used_at to now for all tokens.
func UpdateAllTokensLastUsedAt(db *sql.DB) error {
	_, err := db.Exec(`UPDATE auth_tokens SET last_used_at = NOW()`)
	return err
}
