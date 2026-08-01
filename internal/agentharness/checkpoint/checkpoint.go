// Package checkpoint 实现 Checkpoint、Pause 与 Resume。
package checkpoint

import (
	"time"
)

// Checkpoint 检查点记录（对应 agent_checkpoints 表）。
type Checkpoint struct {
	ID                   string     `gorm:"primaryKey;type:varchar(36)"`
	RunID                string     `gorm:"type:varchar(36);not null;index:idx_run_current,unique"`
	AttemptID            string     `gorm:"type:varchar(36);not null"`
	Version              uint64     `gorm:"not null;unsigned"`
	StorageKind          StorageKind `gorm:"type:varchar(32);not null"`
	BlobURI              *string    `gorm:"type:varchar(512)"`
	Bytes                []byte     `gorm:"type:blob"`
	ByteSize             uint64     `gorm:"not null;unsigned"`
	SHA256               string     `gorm:"type:char(64);not null"`
	FencingToken         uint64     `gorm:"not null;unsigned"`
	VersionSnapshotHash  string     `gorm:"type:char(64);not null"`
	IsCurrent            bool       `gorm:"not null;default:false;index:idx_run_current,unique"`
	CreatedAt            time.Time  `gorm:"not null"`
	ExpiresAt            *time.Time `gorm:"index"`
}

func (Checkpoint) TableName() string { return "agent_checkpoints" }

// StorageKind 存储类型。
type StorageKind string

const (
	StorageKindMySQL StorageKind = "mysql"
	StorageKindMinIO StorageKind = "minio"
)

// PauseResumeRecord Pause/Resume 记录。
type PauseResumeRecord struct {
	ID             string    `gorm:"primaryKey;type:varchar(36)"`
	RunID          string    `gorm:"type:varchar(36);not null;index"`
	CommandType    string    `gorm:"type:varchar(32);not null"`
	Status         string    `gorm:"type:varchar(32);not null"`
	CheckpointRef  *string   `gorm:"type:varchar(36)"`
	CreatedAt      time.Time `gorm:"not null"`
	ConfirmedAt    *time.Time
}

func (PauseResumeRecord) TableName() string { return "agent_pause_resume_records" }
