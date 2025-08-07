package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model

	Username              string
	Password              string
	AssignedTrainingRunID uint
}

type Client struct {
	gorm.Model

	User              User
	UserID            uint `gorm:"index"`
	ClientName        string
	LastVersion       uint
	LastEngineVersion string
	LastGameAt        time.Time
	GpuName           string
}

type TrainingRun struct {
	gorm.Model

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
	ID        uint `gorm:"primary_key"`
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
	gorm.Model

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
	ID        uint64 `gorm:"primary_key"`
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
	ID        uint64    `gorm:"primary_key"`
	CreatedAt time.Time `gorm:"index"`

	User          User
	UserID        uint `gorm:"index"`
	Client        Client
	ClientID      uint `gorm:"index"`
	TrainingRun   TrainingRun
	TrainingRunID uint
	Network       Network
	NetworkID     uint `gorm:"index"`

	// Scoped to training run.
	GameNumber uint

	Version   uint
	Compacted bool

	EngineVersion string

	ResignFPThreshold float64
}

type ServerData struct {
	gorm.Model

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
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// Null for anonymous tokens
	UserID *uint `gorm:"index"`

	// Random, unique token string (e.g., 32-64 bytes base64/hex)
	Token string `gorm:"unique_index"`

	LastUsedAt *time.Time `gorm:"index"`

	IssuedReason string // one of TokenReason*

	// Optional minimal client info for auditing/limiting
	ClientVersion string
	ClientHost    string
	GPUType       string
	GPUID         *int32
}

// GrpcTask represents a single unit assigned to a client via gRPC API.
type GrpcTask struct {
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time

	// External stable identifier returned to clients
	TaskID string `gorm:"unique_index"`

	// Type of task (TRAINING, MATCH, SPRT, TUNING)
	TaskType string `gorm:"index"`

	// JSON payload with the full task definition serialized from proto
	// This avoids destructive schema upgrades as the proto evolves.
	PayloadJSON string `gorm:"type:text"`

	// Downloaded/engine build resources may be referenced via PayloadJSON fields.

	// Assignment
	AssignedToken   AuthToken
	AssignedTokenID *uint      `gorm:"index"` // nullable until assigned
	AssignedAt      *time.Time `gorm:"index"`
	LastHeartbeatAt *time.Time `gorm:"index"`
	Status          string     `gorm:"index"` // ACTIVE, CANCELLED, PENDING, DONE
	CancelledAt     *time.Time `gorm:"index"`
	CompletedAt     *time.Time `gorm:"index"`
}
