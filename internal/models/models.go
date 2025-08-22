package models

import (
	"time"
)

type User struct { // Read only, here for migration from old system
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	Username              string
	Password              string
	AssignedTrainingRunID uint
}

type Client struct { // Read only, here for migration from old system
	ID uint

	User              User
	UserID            uint `gorm:"index"`
	ClientName        string
	LastVersion       uint
	LastEngineVersion string
	LastGameAt        time.Time
	GpuName           string
}

type TrainingRun struct {
	ID uint

	BestNetwork   Network `gorm:"foreignkey:BestNetworkID;association_foreignkey:ID"`
	BestNetworkID uint
	Matches       []Match

	Description     string
	TrainParameters string
	MatchParameters string
	TrainBook       string
	MatchBook       string
	Active          bool
	LastNetwork     uint
	LastGame        uint
	PermissionExpr  string // Expression defined whether user is allowed to use this instance.
	MultiNetMode    bool
}

type Network struct {
	ID        uint
	CreatedAt time.Time

	TrainingRunID uint
	// Scoped to training run
	NetworkNumber uint

	Sha  string
	Path string

	Layers  int
	Filters int

	// Cached here, as expensive to do COUNT(*) on Postgresql
	GamesPlayed int

	Elo    float64
	Anchor bool
	EloSet bool
}

type Match struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	TrainingRunID uint
	Parameters    string

	Candidate     Network
	CandidateID   uint
	CurrentBest   Network
	CurrentBestID uint

	GamesCreated int

	Wins   int
	Losses int
	Draws  int

	GameCap int
	Done    bool
	Passed  bool

	// If true, this is not a promotion match
	TestOnly bool
	// If true, match is unusual so shouldn't be used for elo.
	SpecialParams bool

	TargetSlice int
}

type MatchGame struct {
	ID        uint64
	CreatedAt time.Time

	User    User
	UserID  uint
	Match   Match
	MatchID uint

	Version uint
	Pgn     string
	Result  int
	Done    bool
	Flip    bool

	EngineVersion string
}

type TrainingGame struct {
	ID        uint64
	CreatedAt time.Time

	User          User
	UserID        uint
	Client        Client
	ClientID      uint
	TrainingRun   TrainingRun
	TrainingRunID uint
	Network       Network
	NetworkID     uint

	// Scoped to training run.
	GameNumber uint

	Version   uint
	Compacted bool

	EngineVersion string

	ResignFPThreshold float64
}

type ServerData struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	TrainingPgnUploaded int
}

// ============================================================================
// gRPC v1 Auth and Task models (non-destructive additions)
// ============================================================================

// Token issuance reason for traceability.
const (
	TokenReasonAnonymous = "anonymous"
	TokenReasonMigrated  = "migrated_credentials"
	TokenReasonManual    = "manual"
)

// Task types aligned with proto TaskType
const (
	TaskTypeUnspecified = "UNSPECIFIED"
	TaskTypeTraining    = "TRAINING"
	TaskTypeMatch       = "MATCH"
	TaskTypeSprt        = "SPRT"
	TaskTypeTuning      = "TUNING"
)

// Task status to support heartbeat/cancellation
const (
	TaskStatusActive    = "ACTIVE"
	TaskStatusCancelled = "CANCELLED"
	TaskStatusPending   = "PENDING"
	TaskStatusDone      = "DONE"
)

// AuthToken stores bearer tokens for both migrated and anonymous users.
type AuthToken struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Null for anonymous tokens
	UserID *uint

	// Random, unique token string (e.g., 32-64 bytes base64/hex)
	Token string

	LastUsedAt *time.Time

	IssuedReason string // one of TokenReason*

	// Optional minimal client info for auditing/limiting
	ClientVersion string
	ClientHost    string
	GPUType       string
	GPUID         *int32
}

// TaskAssignment represents the assignment of a user (via AuthToken) to a specific task instance (TRAINING, MATCH, SPRT, TUNING, etc.).
// Each row tracks a user's active work on a task, including assignment and progress status.
type TaskAssignment struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// External stable identifier returned to clients
	TaskID string

	// Type of task (TRAINING, MATCH, SPRT, TUNING)
	TaskType string

	// Assignment
	AssignedToken   AuthToken
	AssignedTokenID *uint // nullable until assigned
	AssignedAt      *time.Time
	LastHeartbeatAt *time.Time
	Status          string // ACTIVE, CANCELLED, PENDING, DONE
	CancelledAt     *time.Time
	CompletedAt     *time.Time
}

// ============================================================================
// New Task Hierarchy
// ============================================================================

type Book struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	Sha256    string
	URL       string
	SizeBytes int64
	Format    string
}

// Task is the base table for all high-level tasks (training, match, sprt, tune, etc.)
type Task struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Type of the task (TRAINING, SPRT, TUNE, etc.) (MATCH task is a part of TRAINING)
	TaskType string

	// Status (ACTIVE, CANCELLED, PENDING, DONE, etc.)
	Status string

	// Optional: Human-readable description
	Description string

	// Optional: extensibility fields should be added explicitly as needed.
}

// TrainingTask represents a training process (one per network training cycle)
type TrainingTask struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Foreign key to base Task
	TaskID uint
	Task   Task

	// TrainingRun this task is associated with (if any)
	TrainingRunID *uint
	TrainingRun   *TrainingRun

	// All match tasks for this training cycle
	MatchTasks []MatchTask

	TrainBook   *Book
	TrainBookID uint
	MatchBook   *Book
	MatchBookID uint

	// Network produced by this training task
	OutputNetworks []Network
	BestNetwork    Network
	BestNetworkID  uint

	TrainParameters string // Maybe add UCI options here?
	MatchParameters string
}

// MatchTask represents a match process (promotion, evaluation, etc.)
type MatchTask struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Foreign key to base Task
	TaskID uint
	Task   Task

	// Parent training task
	TrainingTaskID uint
	TrainingTask   TrainingTask

	// Candidate and current best networks
	CandidateNetworkID   *uint
	CandidateNetwork     *Network
	CurrentBestNetworkID *uint
	CurrentBestNetwork   *Network

	// Game results, etc. (optional, for denormalization)
	GamesCreated int
	Wins         int
	Losses       int
	Draws        int
	Done         bool
	Passed       bool

	// Book and Engine parameters to use from training task
	// NOTE: Missing target slice from original MatchTask
}

// SprtTask represents a sequential probability ratio test task
type SprtTask struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Foreign key to base Task
	TaskID uint
	Task   Task

	// Baseline EngineConfiguration fields
	// Baseline BuildSpec fields
	BaselineNetwork          *Network
	BaselineNetworkID        uint
	BaselineParamsArgs       string // JSON-encoded []string or separate table if needed
	BaselineParamsUciOptions string // JSON-encoded map[string]string or separate table if needed

	// Candidate EngineConfiguration fields
	// Candidate BuildSpec fields
	CandidateNetwork          *Network
	CandidateNetworkID        uint
	CandidateParamsArgs       string
	CandidateParamsUciOptions string

	OpeningBook   *Book
	OpeningBookID uint

	// Time control
	TimeControlType  string  // "time_based" or "nodes_per_move"
	BaseTimeSeconds  float64 // Only if time_based
	IncrementSeconds float64 // Only if time_based
	NodesPerMove     int64   // Only if nodes_per_move
}

// TuneTask represents a tuning task (hyperparameter search, etc.)
type TuneTask struct {
	ID        uint
	CreatedAt time.Time
	UpdatedAt time.Time

	// Foreign key to base Task
	TaskID uint
	Task   Task

	// BuildSpec fields
	BuildRepoURL    string
	BuildCommitHash string
	BuildParams     string // JSON-encoded map[string]string or separate table if needed

	TuneNetwork   Network
	TuneNetworkID uint

	OpeningBook   *Book
	OpeningBookID uint

	GamesPerParamSet int32

	// TODO: Should this be a separate table? or JSON-encoded?
	TuneParamSets []TuneParamSet

	// Time control
	TimeControlType  string  // "time_based" or "nodes_per_move"
	BaseTimeSeconds  float64 // Only if time_based
	IncrementSeconds float64 // Only if time_based
	NodesPerMove     int64   // Only if nodes_per_move
}

type TuneParamSet struct {
	ID               uint
	TuneTaskID       uint
	ParamSetID       string // Unique ID for this parameter configuration
	ParamsArgs       string // JSON-encoded []string or separate table if needed
	ParamsUciOptions string // JSON-encoded map[string]string or separate table if needed
}

/* Should we add a table for build versions, could be automatically build during releases,
*  and added by devs for PRs on front-end?
*
*  With the table version, it could also track the if this tasks needs that version to be minimum,
*  maximum (regression tests), or exact (PRs).
 */
