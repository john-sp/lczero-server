package server

import (
	"context"
	"time"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"github.com/leelachesszero/lczero-server/internal/models"

	"gorm.io/gorm"
)

// validateToken checks if the provided token string exists and is active in the database.
func (s *TaskServiceImpl) validateToken(token string) (*models.AuthToken, error) {
	if len(token) < 5 || token[:4] != "lc0-" {
		return nil, ErrInvalidTokenFormat
	}
	var tok models.AuthToken
	if err := s.DB.Where("token = ? AND active = ?", token, true).First(&tok).Error; err != nil {
		return nil, err
	}

	// If the token is found, update its last used timestamp
	now := time.Now()
	tok.LastUsedAt = &now
	if err := s.DB.Save(&tok).Error; err != nil {
		return nil, err
	}

	return &tok, nil
}

// ErrInvalidTokenFormat is returned when a token does not start with "lc0-".
var ErrInvalidTokenFormat = gorm.ErrInvalidTransaction // Use a recognizable error, or define your own

// TaskServiceServer defines the server interface for TaskService.
type TaskServiceServer interface {
	GetNextTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error)
	ReportProgress(ctx context.Context, req *pb.ProgressReport) (*pb.ProgressResponse, error)
}

// TaskServiceImpl provides TaskService backed by DB.
type TaskServiceImpl struct {
	pb.UnimplementedTaskServiceServer
	DB *gorm.DB
}

// updateClientInfo updates audit fields for the given token using client info from the request.
func (s *TaskServiceImpl) updateClientInfo(tok *models.AuthToken, clientInfo *pb.ClientInfo) {
	now := time.Now()
	_ = s.DB.Model(tok).Updates(map[string]interface{}{
		"last_used_at":   &now,
		"client_host":    clientInfo.GetHostname(),
		"client_version": clientInfo.GetVersion(),
		"gpu_type":       clientInfo.GetGpuType(),
		"gpuid":          func() *int32 { v := clientInfo.GetGpuId(); return &v }(),
	}).Error
}

// NewTaskService constructs the TaskServiceImpl.
func NewTaskService(dbConn *gorm.DB) *TaskServiceImpl {
	return &TaskServiceImpl{DB: dbConn}
}

/*
GetNextTask fetches next available task for the client/token

TODO for this function:
1. Validate engine version for each task
2. Handle task assignment logic (Other Task types)
3. Size, SHA, and URL for all resources
4. Task ID generation
5. Correctly handle engine parameters
6.
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
	var tr models.TrainingRun
	if err := s.DB.Where("active = ?", true).Order("id ASC").First(&tr).Error; err != nil {
		return nil, err
	}

	// Load best network for this run
	var net models.Network
	if err := s.DB.Where("id = ?", tr.BestNetworkID).First(&net).Error; err != nil {
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
	resp, err := s.getNextMatchTask(ctx, tok, tr, now, req, slice)
	if err == nil && resp != nil {
		return resp, nil
	}

	// Fallback to training task
	return s.getNextTrainingTask(ctx, tok, tr, net, now, req)
}

// getNextMatchTask tries to allocate a match task for the given training run and slice.
func (s *TaskServiceImpl) getNextMatchTask(
	ctx context.Context,
	tok *models.AuthToken,
	tr models.TrainingRun,
	now time.Time,
	req *pb.TaskRequest,
	slice int,
) (*pb.TaskResponse, error) {
	var pendingMatch models.Match
	matchQuery := s.DB.
		Preload("Candidate").
		Preload("CurrentBest").
		Where("done = ? AND training_run_id = ? AND (target_slice = 0 OR target_slice = ?)", false, tr.ID, slice).
		Order("id ASC")
	if err := matchQuery.First(&pendingMatch).Error; err == nil {
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
		if err := s.DB.Create(mg).Error; err != nil {
			return nil, err
		}
		flip := (mg.ID & 1) == 1
		_ = s.DB.Model(mg).Update("flip", flip).Error

		baselineNetRes := &pb.ResourceSpec{
			Sha256:    pendingMatch.CurrentBest.Sha,
			Url:       "",
			SizeBytes: 0,
			Type:      pb.ResourceType_NETWORK,
			Format:    "",
		}
		candidateNetRes := &pb.ResourceSpec{
			Sha256:    pendingMatch.Candidate.Sha,
			Url:       "",
			SizeBytes: 0,
			Type:      pb.ResourceType_NETWORK,
			Format:    "",
		}
		matchBook := &pb.ResourceSpec{
			Sha256:    "",
			Url:       tr.MatchBook,
			SizeBytes: 0,
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
		grpcTask := &models.GrpcTask{
			TaskID:      taskID,
			TaskType:    models.TaskTypeMatch,
			PayloadJSON: "",
			AssignedTokenID: func() *uint {
				if tok.ID == 0 {
					return nil
				}
				v := tok.ID
				return &v
			}(),
			AssignedAt:      &now,
			LastHeartbeatAt: &now,
			Status:          models.TaskStatusActive,
		}
		if err := s.DB.Create(grpcTask).Error; err != nil {
			return nil, err
		}

		resp := &pb.TaskResponse{
			TaskId: taskID,
			Task: &pb.TaskResponse_Match{
				Match: matchTask,
			},
		}
		return resp, nil
	}
	return nil, nil
}

// getNextTrainingTask allocates a training task for the given training run.
func (s *TaskServiceImpl) getNextTrainingTask(
	ctx context.Context,
	tok *models.AuthToken,
	tr models.TrainingRun,
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
		Sha256:    "",
		Url:       tr.TrainBook,
		SizeBytes: 0,
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
		Engine:      engineCfg,
		OpeningBook: openingBookRes,
	}
	taskID := time.Now().UTC().Format("20060102T150405.000000000")
	grpcTask := &models.GrpcTask{
		TaskID:      taskID,
		TaskType:    models.TaskTypeTraining,
		PayloadJSON: "",
		AssignedTokenID: func() *uint {
			if tok.ID == 0 {
				return nil
			}
			v := tok.ID
			return &v
		}(),
		AssignedAt:      &now,
		LastHeartbeatAt: &now,
		Status:          models.TaskStatusActive,
	}
	if err := s.DB.Create(grpcTask).Error; err != nil {
		return nil, err
	}
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
1. Handle training and match cases
2. Save games that are uploaded
  - Verify versions before save

3. Handle SPRT and tuning progress
4. Correctly update task status based on progress
5. Remove tasks that are never completed, missing a number of heart beats
6. Handle cancellation logic
7. Handle crashes and logging
*/
func (s *TaskServiceImpl) ReportProgress(ctx context.Context, req *pb.ProgressReport) (*pb.ProgressResponse, error) {

	_, err := s.validateToken(req.Token)
	if err != nil {
		return nil, err
	}

	var task models.GrpcTask
	if err := s.DB.Where("task_id = ?", req.TaskId).First(&task).Error; err != nil {
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

	if err := s.DB.Save(&task).Error; err != nil {
		return nil, err
	}

	status := pb.ProgressResponse_ACTIVE
	if task.Status == models.TaskStatusCancelled {
		status = pb.ProgressResponse_CANCELLED
	}
	return &pb.ProgressResponse{Status: status}, nil
}
