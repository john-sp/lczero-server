// Package queries contains SQL query templates for user-related operations.
package queries

const (
	SelectUserByUsername = `
SELECT id, username, password, assigned_training_run_id, created_at, updated_at, deleted_at
FROM users
WHERE username = $1
`
	InsertAuthToken = `
INSERT INTO auth_tokens (token, issued_reason, created_at, user_id)
VALUES ($1, $2, $3, $4)
RETURNING id
`
)
