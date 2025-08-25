package server

import (
	"context"
	"errors"
	"time"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"github.com/leelachesszero/lczero-server/internal/models"

	"database/sql"

	"github.com/leelachesszero/lczero-server/internal/db/queries"
)

// validateToken checks if the provided token string exists and is active in the database.
func (s *TaskServiceImpl) validateToken(token string) (*models.AuthToken, error) {
	if len(token) < 5 || token[:4] != "lc0-" {
		return nil, ErrInvalidTokenFormat
	}
	var tok models.AuthToken
	row := s.DB.QueryRow(`SELECT id, created_at, updated_at, user_id, token, last_used_at, issued_reason, client_version, client_host, gpu_type, gpuid, status FROM auth_tokens WHERE token = $1`, token)
	err := row.Scan(
		&tok.ID, &tok.CreatedAt, &tok.UpdatedAt, &tok.UserID, &tok.Token, &tok.LastUsedAt, &tok.IssuedReason,
		&tok.ClientVersion, &tok.ClientHost, &tok.GPUType, &tok.GPUID,
	)
	if err != nil {
		return nil, err
	}

	// If the token is found, update its last used timestamp
	now := time.Now()
	tok.LastUsedAt = &now
	_, err = s.DB.Exec(`UPDATE auth_tokens SET last_used_at = $1 WHERE id = $2`, now, tok.ID)
	if err != nil {
		return nil, err
	}

	return &tok, nil
}

// ErrInvalidTokenFormat is returned when a token does not start with "lc0-".
var ErrInvalidTokenFormat = errors.New("invalid token format")

// TaskServiceServer defines the server interface for TaskService.
type TaskServiceServer interface {
	GetNextTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error)
	ReportProgress(ctx context.Context, req *pb.ProgressReport) (*pb.ProgressResponse, error)
}

// TaskServiceImpl provides TaskService backed by DB.
type TaskServiceImpl struct {
	pb.UnimplementedTaskServiceServer
	DB *sql.DB
}

// updateClientInfo updates audit fields for the given token using client info from the request.
func (s *TaskServiceImpl) updateClientInfo(tok *models.AuthToken, clientInfo *pb.ClientInfo) {
	now := time.Now()
	_, _ = s.DB.Exec(
		`UPDATE auth_tokens SET last_used_at = $1, client_host = $2, client_version = $3, gpu_type = $4, gpuid = $5 WHERE id = $6`,
		now,
		clientInfo.GetHostname(),
		clientInfo.GetVersion(),
		clientInfo.GetGpuType(),
		clientInfo.GetGpuId(),
		tok.ID,
	)
}

// NewTaskService constructs the TaskServiceImpl.
func NewTaskService(dbConn *sql.DB) *TaskServiceImpl {
	return &TaskServiceImpl{DB: dbConn}
}

/*
GetNextTask fetches next available task for the client/token

TODO for this function:
1. Determine what tasks the user is eligible for based on their token and client info
  - Validate engine version
  - Check user supported task types

2. Determine NPS on each task type (Depends on network, might be hard. Maybe use a known network)
  - Potentially store this NPS in a hardware db.

3. Compute workload ratios (mostly for training runs)

4. Assign user to previous task (if it exists), if it doesn't mess with ratios too much

5. Size, SHA, and URL for all resources.

6. Task ID generation

7. Correctly handle engine parameters.
*/
func (s *TaskServiceImpl) GetNextTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	// 1) Validate token and update audit info
	tok, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.updateClientInfo(tok, req.GetClientInfo())

	// 2) Choose the lowest-id active TrainingRun (do NOT default to zero)
	tr, err := queries.FetchActiveTrainingTask(s.DB)
	if err != nil {
		return nil, err
	}

	// Load best network for this run
	net, err := queries.FetchNetworkByID(s.DB, tr.BestNetworkID)
	if err != nil {
		return nil, err
	}

	// Compute deterministic slice for match assignment
	tokenStr := req.GetToken()
	slice := 1
	if len(tokenStr) > 0 {
		var acc int
		for i := 0; i < len(tokenStr); i++ {
			acc += int(tokenStr[i])
		}
		slice = (acc%3 + 1)
	}

	// Try match task first
	resp, err := s.getNextMatchTask(ctx, tok, *tr, now, req, slice)
	if err == nil && resp != nil {
		return resp, nil
	}

	// Fallback to training task
	return s.getNextTrainingTask(ctx, tok, *tr, *net, now, req)
}

// TODO: getNextMatchTask and getNextTrainingTask are both almost direct copies from HTTP version. They should be rewritten.

// getNextMatchTask tries to allocate a match task for the given training run and slice.
func (s *TaskServiceImpl) getNextMatchTask(
	ctx context.Context,
	tok *models.AuthToken,
	tr models.TrainingTask,
	now time.Time,
	req *pb.TaskRequest,
	slice int,
) (*pb.TaskResponse, error) {
	// TODO: I think this is wrong, look over again. It is almost an exact copy from the HTTP version.

	// NOTE: target slice no longer exists in MatchTask. Rework required
	pendingMatchPtr, err := queries.FetchPendingMatch(s.DB, tr.ID, slice)
	if err == nil && pendingMatchPtr != nil {
		pendingMatch := *pendingMatchPtr
		mg := &models.MatchGame{
			UserID: func() uint {
				if tok.UserID != nil {
					return *tok.UserID
				}
				return 0
			}(),
			MatchID: pendingMatch.ID,
			Done:    false,
		}
		mg.ID, err = queries.InsertMatchGame(s.DB, mg.UserID, mg.MatchID, mg.Done)
		if err != nil {
			return nil, err
		}
		flip := (mg.ID & 1) == 1
		_ = queries.UpdateMatchGameFlip(s.DB, mg.ID, flip)

		// Fetch Candidate and CurrentBest networks for resource specs
		candidateSha, _ := queries.FetchNetworkSha(s.DB, pendingMatch.CandidateID)
		currentBestSha, _ := queries.FetchNetworkSha(s.DB, pendingMatch.CurrentBestID)

		baselineNetRes := &pb.ResourceSpec{
			Sha256:    currentBestSha,
			Url:       "",
			SizeBytes: 0,
			Type:      pb.ResourceType_NETWORK,
			Format:    "",
		}
		candidateNetRes := &pb.ResourceSpec{
			Sha256:    candidateSha,
			Url:       "",
			SizeBytes: 0,
			Type:      pb.ResourceType_NETWORK,
			Format:    "",
		}
		// Fetch MatchBook info
		matchBookSha, matchBookURL, matchBookSize, _ := queries.FetchBookByID(s.DB, tr.MatchBookID)
		matchBook := &pb.ResourceSpec{
			Sha256:    matchBookSha,
			Url:       matchBookURL,
			SizeBytes: matchBookSize,
			Type:      pb.ResourceType_BOOK,
			Format:    "pgn",
		}
		engineParams := &pb.EngineParams{
			Args:       []string{tr.MatchParameters},
			UciOptions: map[string]string{},
		}
		matchTask := &pb.MatchTask{
			Baseline: &pb.EngineConfiguration{
				Build:   &pb.BuildSpec{},
				Network: baselineNetRes,
				Params:  engineParams,
			},
			Candidate: &pb.EngineConfiguration{
				Build:   &pb.BuildSpec{},
				Network: candidateNetRes,
				Params:  engineParams,
			},
			OpeningBook: matchBook,
		}

		taskID := time.Now().UTC().Format("20060102T150405.000000000")
		grpcTaskID, err := queries.InsertTaskAssignment(
			s.DB,
			taskID,
			models.TaskTypeMatch,
			tok.ID,
			now,
			now,
			models.TaskStatusActive,
		)
		if err != nil {
			return nil, err
		}

		resp := &pb.TaskResponse{
			TaskId: taskID,
			Task: &pb.TaskResponse_Match{
				Match: matchTask,
			},
		}
		_ = grpcTaskID // suppress unused warning
		return resp, nil
	}
	return nil, nil
}

// getNextTrainingTask allocates a training task for the given training run.
func (s *TaskServiceImpl) getNextTrainingTask(
	ctx context.Context,
	tok *models.AuthToken,
	tr models.TrainingTask,
	net models.Network,
	now time.Time,
	req *pb.TaskRequest,
) (*pb.TaskResponse, error) {
	networkRes := &pb.ResourceSpec{
		Sha256:    net.Sha,
		Url:       "",
		SizeBytes: 0,
		Type:      pb.ResourceType_NETWORK,
		Format:    "",
	}
	openingBookRes := &pb.ResourceSpec{
		Sha256:    tr.TrainBook.Sha256,
		Url:       tr.TrainBook.URL,
		SizeBytes: tr.TrainBook.SizeBytes,
		Type:      pb.ResourceType_BOOK,
		Format:    "pgn",
	}
	engineCfg := &pb.EngineConfiguration{
		Build:   &pb.BuildSpec{},
		Network: networkRes,
		Params: &pb.EngineParams{
			Args:       []string{tr.TrainParameters},
			UciOptions: map[string]string{},
		},
	}
	trainingTask := &pb.TrainingTask{
		Engine:       engineCfg,
		OpeningBook:  openingBookRes,
		NodesPerMove: 8000, // TODO: Get from table
	}
	taskID := time.Now().UTC().Format("20060102T150405.000000000")
	grpcTaskID, err := queries.InsertTaskAssignment(
		s.DB,
		taskID,
		models.TaskTypeTraining,
		tok.ID,
		now,
		now,
		models.TaskStatusActive,
	)
	if err != nil {
		return nil, err
	}
	_ = grpcTaskID // suppress unused warning
	resp := &pb.TaskResponse{
		TaskId: taskID,
		Task: &pb.TaskResponse_Training{
			Training: trainingTask,
		},
	}
	return resp, nil
}

/*
ReportProgress updates heartbeat and checks cancellation.

TODO for this function:

## Training/Match Heartbeats
- Save games that are uploaded

## SPRT Heartbeats
- Record Wins/Losses/Draws (Pentanomial requires LL/LD/DD/DW/WW results)
-

## Tuning Heartbeats

## All of them
1. Handle training and match cases
2. Save games that are uploaded
  - Verify versions before save

3. Handle SPRT and tuning progress
4. Correctly update task status based on progress
5. Remove tasks that are never completed, missing a number of heart beats
6. Handle cancellation logic (Stop iff task is finished, find some way to report new networks required)
7. Handle crashes and logging
*/
func (s *TaskServiceImpl) ReportProgress(ctx context.Context, req *pb.ProgressReport) (*pb.ProgressResponse, error) {

	_, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	var task models.TaskAssignment
	rowTask := s.DB.QueryRow(`SELECT id, created_at, updated_at, task_id, task_type, assigned_token_id, assigned_at, last_heartbeat_at, status, cancelled_at, completed_at FROM task_assignments WHERE task_id = $1`, req.TaskId)
	err = rowTask.Scan(
		&task.ID, &task.CreatedAt, &task.UpdatedAt, &task.TaskID, &task.TaskType, &task.AssignedTokenID, &task.AssignedAt, &task.LastHeartbeatAt, &task.Status, &task.CancelledAt, &task.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	task.LastHeartbeatAt = &now

	switch req.GetProgress().(type) {
	case *pb.ProgressReport_Training:
		// TODO: Handle training progress
	case *pb.ProgressReport_Match:
		// TODO: Handle match progress
	case *pb.ProgressReport_Sprt:
		// TODO: Handle SPRT progress
	case *pb.ProgressReport_Tuning:
		// TODO: Handle tuning progress
	}

	err = queries.UpdateTaskAssignmentHeartbeat(s.DB, task.ID, now)
	if err != nil {
		return nil, err
	}

	status := pb.ProgressResponse_ACTIVE
	if task.Status == models.TaskStatusCancelled {
		status = pb.ProgressResponse_CANCELLED
	}
	return &pb.ProgressResponse{Status: status}, nil
}
