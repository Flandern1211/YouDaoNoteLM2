// Package finalization 实现 FinalizationPort 和 Repair Scanner。
package finalization

import (
	"time"
)

// WritebackJournal 写回日志记录。
type WritebackJournal struct {
	ID              string          `gorm:"primaryKey;type:varchar(36)"`
	RunID           string          `gorm:"type:varchar(36);not null;index"`
	Revision        uint64          `gorm:"not null;unsigned"`
	OperationType   OperationType   `gorm:"type:varchar(32);not null"`
	Status          OperationStatus `gorm:"type:varchar(32);not null;index"`
	IdempotencyKey  string          `gorm:"type:varchar(191);not null;uniqueIndex"`
	AuthorityJSON   string          `gorm:"type:json;not null"`
	DependsOn       *string         `gorm:"type:varchar(36)"`
	ErrorClass      *string         `gorm:"type:varchar(32)"`
	ErrorCode       *string         `gorm:"type:varchar(64)"`
	AttemptCount    int             `gorm:"not null;default:0"`
	MaxAttempts     int             `gorm:"not null;default:3"`
	NextRetryAt     *time.Time      `gorm:"index"`
	CreatedAt       time.Time       `gorm:"not null"`
	UpdatedAt       time.Time       `gorm:"not null"`
	FinishedAt      *time.Time
}

func (WritebackJournal) TableName() string { return "agent_writeback_journal" }

// OperationType 操作类型。
type OperationType string

const (
	OpTypeAssistantMessage OperationType = "assistant_message"
	OpTypeManifest         OperationType = "manifest"
	OpTypeEvent            OperationType = "event"
	OpTypeSummary          OperationType = "summary"
	OpTypeMemory           OperationType = "memory"
)

// OperationStatus 操作状态。
type OperationStatus string

const (
	OpStatusPending    OperationStatus = "pending"
	OpStatusInProgress OperationStatus = "in_progress"
	OpStatusCompleted  OperationStatus = "completed"
	OpStatusFailed     OperationStatus = "failed"
	OpStatusSkipped    OperationStatus = "skipped"
)

// IsTerminal 检查是否为终态。
func (s OperationStatus) IsTerminal() bool {
	return s == OpStatusCompleted || s == OpStatusFailed || s == OpStatusSkipped
}
