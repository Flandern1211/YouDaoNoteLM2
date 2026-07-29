// Package cancel 实现持久化 Cancel 命令。
package cancel

import (
	"time"
)

// Command 命令记录（对应 agent_run_commands 表）。
type Command struct {
	ID              string        `gorm:"primaryKey;type:varchar(36)"`
	RunID           string        `gorm:"type:varchar(36);not null;index:idx_run_idempotency,unique"`
	CommandType     CommandType   `gorm:"type:varchar(32);not null"`
	ActorID         uint          `gorm:"not null"`
	IdempotencyKey  string        `gorm:"type:varchar(191);not null;index:idx_run_idempotency,unique"`
	AuthorityJSON   string        `gorm:"type:json"`
	StateVersion    uint64        `gorm:"not null;unsigned"`
	Status          CommandStatus `gorm:"type:varchar(32);not null;index"`
	ReasonCode      *string       `gorm:"type:varchar(64)"`
	CreatedAt       time.Time     `gorm:"not null"`
	ConfirmedAt     *time.Time
}

func (Command) TableName() string { return "agent_run_commands" }

// CommandType 命令类型。
type CommandType string

const (
	CommandTypeCancel CommandType = "cancel"
	CommandTypePause  CommandType = "pause"
	CommandTypeResume CommandType = "resume"
)

// CommandStatus 命令状态。
type CommandStatus string

const (
	CommandStatusPending    CommandStatus = "pending"
	CommandStatusAccepted   CommandStatus = "accepted"
	CommandStatusSuperseded CommandStatus = "superseded"
	CommandStatusRejected   CommandStatus = "rejected"
)

// CancelRequest 取消请求。
type CancelRequest struct {
	RunID          string
	ActorID        uint
	IdempotencyKey string
	ReasonCode     *string
}

// CancelResponse 取消响应。
type CancelResponse struct {
	CommandID string        `json:"command_id"`
	Status    CommandStatus `json:"status"`
	Message   string        `json:"message"`
}
