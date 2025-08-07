package server

import (
	"context"
	"time"

	pb "github.com/leelachesszero/lczero-server/api/v1"

	"github.com/leelachesszero/lczero-server/internal/models"

	"gorm.io/gorm"
)

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

// NewTaskService constructs the TaskServiceImpl.
func NewTaskService(dbConn *gorm.DB) *TaskServiceImpl {
	return &TaskServiceImpl{DB: dbConn}
}

// GetNextTask fetches next available task for the client/token.
func (s *TaskServiceImpl) GetNextTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	// 1) Validate token and update audit info
	var tok models.AuthToken
	if err := s.DB.Where("token = ?", req.Token).First(&tok).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.DB.Model(&tok).Updates(map[string]interface{}{
		"last_used_at":   &now,
		"client_host":    req.GetClientInfo().GetHostname(),
		"client_version": req.GetClientInfo().GetVersion(),
		"gpu_type":       req.GetClientInfo().GetGpuType(),
		"gpuid":          func() *int32 { v := req.GetClientInfo().GetGpuId(); return &v }(),
	}).Error; err != nil {
		// If schema doesn't include these columns yet, ignore errors for optional fields
		_ = err
	}

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

	// Attempt MATCH assignment first, mirroring legacy nextGame behavior.
	// Compute a deterministic slice key from token string when possible.
	// Fallback to 1..3 slices cycling.
	tokenStr := req.GetToken()
	slice := 1
	if len(tokenStr) > 0 {
		var acc int
		for i := 0; i < len(tokenStr); i++ {
			acc += int(tokenStr[i])
		}
		slice = (acc%3 + 1)
	}
	// Find next pending match for this training run and slice.
	var pendingMatch models.Match
	matchQuery := s.DB.
		Preload("Candidate").
		Preload("CurrentBest").
		Where("done = ? AND training_run_id = ? AND (target_slice = 0 OR target_slice = ?)", false, tr.ID, slice).
		Order("id ASC")
	if err := matchQuery.First(&pendingMatch).Error; err == nil {
		// Allocate a MatchGame row to track assignment-like behavior as legacy server did.
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
		if err := s.DB.Model(mg).Update("flip", flip).Error; err != nil {
			// Non-fatal; continue even if flip update fails
			_ = err
		}

		// Build Match task response payload.
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

		// Generate task id and persist GrpcTask
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

	// No match available; fall back to training task.
	// Build proto resources per api/v1/lczero.proto
	networkRes := &pb.ResourceSpec{
		Sha256:    net.Sha,
		Url:       "", // Optional: fill with CDN/path when available
		SizeBytes: 0,  // Unknown size currently
		Type:      pb.ResourceType_NETWORK,
		Format:    "", // Unknown, not required
	}
	openingBookRes := &pb.ResourceSpec{
		Sha256:    "", // Unknown; provide URL only if configured
		Url:       tr.TrainBook,
		SizeBytes: 0, // Unknown size currently
		Type:      pb.ResourceType_BOOK,
		Format:    "pgn",
	}

	// Engine build and params placeholders (can be extended later)
	engineCfg := &pb.EngineConfiguration{
		Build:   &pb.BuildSpec{}, // No specific repo/commit for now
		Network: networkRes,
		Params: &pb.EngineParams{
			Args:       []string{tr.TrainParameters},
			UciOptions: map[string]string{},
		},
	}

	// Compose TrainingTask
	trainingTask := &pb.TrainingTask{
		Engine:      engineCfg,
		OpeningBook: openingBookRes,
	}

	// Generate a stable external task id; placeholder for now
	taskID := time.Now().UTC().Format("20060102T150405.000000000")

	// Persist GrpcTask record
	grpcTask := &models.GrpcTask{
		TaskID:   taskID,
		TaskType: models.TaskTypeTraining,
		// Optionally serialize payload; left empty until a serializer is decided
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

	// Wrap into TaskResponse per proto
	resp := &pb.TaskResponse{
		TaskId: taskID,
		Task: &pb.TaskResponse_Training{
			Training: trainingTask,
		},
	}
	return resp, nil
}

// ReportProgress updates heartbeat and checks cancellation. TODO: Does not save games that are uploaded
// Find a way to remove tasks that are never completed, missing a number of heart beats
func (s *TaskServiceImpl) ReportProgress(ctx context.Context, req *pb.ProgressReport) (*pb.ProgressResponse, error) {
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
