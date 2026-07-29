package store

import (
	"time"

	"YoudaoNoteLm/internal/agentharness/core"
)

// AgentRun 是 agent_runs 表的 GORM 模型。
type AgentRun struct {
	ID                      string          `gorm:"primaryKey;type:varchar(36)"`
	ParentRunID             *string         `gorm:"type:varchar(36);index"`
	AgentType               string          `gorm:"type:varchar(64);not null;index"`
	UserID                  uint            `gorm:"not null;index"`
	NotebookID              *uint           `gorm:"index"`
	ConversationID          *uint           `gorm:"index"`
	IdempotencyKey          string          `gorm:"type:varchar(191);not null;uniqueIndex:idx_user_idempotency,priority:1"`
	Sequence                uint64          `gorm:"not null;unsigned;uniqueIndex:idx_conv_sequence,priority:1"`
	InputKind               string          `gorm:"type:varchar(32);not null"`
	InputRef                string          `gorm:"type:varchar(256);not null"`
	InputHash               string          `gorm:"type:char(64);not null"`
	VersionSnapshotJSON     string          `gorm:"type:json;not null"`
	State                   core.RunState   `gorm:"type:varchar(32);not null;index"`
	DesiredState            core.DesiredState `gorm:"type:varchar(32);not null"`
	StateVersion            uint64          `gorm:"not null;unsigned"`
	Revision                uint64          `gorm:"not null;unsigned"`
	CurrentAttemptID        *string         `gorm:"type:varchar(36);index"`
	FencingToken            uint64          `gorm:"not null;unsigned"`
	PendingResumeCheckpointRef *string      `gorm:"type:varchar(256)"`
	RetryCount              *int
	MaxRetries              *int
	NextRetryAt             *time.Time      `gorm:"index"`
	LastErrorClass          *string         `gorm:"type:varchar(32)"`
	LastErrorCode           *string         `gorm:"type:varchar(64)"`
	CreatedAt               time.Time       `gorm:"not null"`
	StartedAt               *time.Time
	FinishedAt              *time.Time
	UpdatedAt               time.Time       `gorm:"not null"`
}

func (AgentRun) TableName() string { return "agent_runs" }

// AgentRunAttempt 是 agent_run_attempts 表的 GORM 模型。
type AgentRunAttempt struct {
	ID                  string              `gorm:"primaryKey;type:varchar(36)"`
	RunID               string              `gorm:"type:varchar(36);not null;index"`
	AttemptNumber       uint                `gorm:"not null"`
	WorkerID            *string             `gorm:"type:varchar(128)"`
	FencingToken        uint64              `gorm:"not null;unsigned"`
	ResumeCheckpointRef *string             `gorm:"type:varchar(256)"`
	TraceID             *string             `gorm:"type:varchar(128);index"`
	State               core.AttemptState   `gorm:"type:varchar(32)"`
	ErrorClass          *core.ErrorClass    `gorm:"type:varchar(32)"`
	ErrorCode           *string             `gorm:"type:varchar(64)"`
	StartedAt           time.Time           `gorm:"not null"`
	HeartbeatAt         *time.Time
	FinishedAt          *time.Time
}

func (AgentRunAttempt) TableName() string { return "agent_run_attempts" }

// AgentRunStep 是 agent_run_steps 表的 GORM 模型。
type AgentRunStep struct {
	ID                string           `gorm:"primaryKey;type:varchar(36)"`
	RunID             string           `gorm:"type:varchar(36);not null;index"`
	AttemptID         string           `gorm:"type:varchar(36);not null;index"`
	ParentStepID      *string          `gorm:"type:varchar(36);index"`
	Kind              core.StepKind    `gorm:"type:varchar(32);not null;index"`
	AgentName         string           `gorm:"type:varchar(128);not null;index"`
	State             core.StepState   `gorm:"type:varchar(32);not null;index"`
	InputHash         *string          `gorm:"type:char(64)"`
	ToolCallID        *string          `gorm:"type:varchar(128);index"`
	ResultArtifactRef *string          `gorm:"type:varchar(256)"`
	FencingToken      uint64           `gorm:"not null;unsigned"`
	ErrorClass        *core.ErrorClass `gorm:"type:varchar(32)"`
	ErrorCode         *string          `gorm:"type:varchar(64)"`
	StartedAt         time.Time        `gorm:"not null"`
	FinishedAt        *time.Time
}

func (AgentRunStep) TableName() string { return "agent_run_steps" }

// AgentRunEvent 是 agent_run_events 表的 GORM 模型。
type AgentRunEvent struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)"`
	RunID           string    `gorm:"type:varchar(36);not null;index:idx_run_sequence,unique"`
	Sequence        uint64    `gorm:"not null;unsigned;index:idx_run_sequence,unique"`
	EventID         string    `gorm:"type:varchar(36);not null;uniqueIndex"`
	AttemptID       *string   `gorm:"type:varchar(36);index"`
	StepID          *string   `gorm:"type:varchar(36);index"`
	EventType       string    `gorm:"type:varchar(64);not null"`
	PayloadVersion  uint      `gorm:"not null;unsigned"`
	PayloadJSON     string    `gorm:"type:json;not null"`
	CreatedAt       time.Time `gorm:"not null"`
}

func (AgentRunEvent) TableName() string { return "agent_run_events" }
