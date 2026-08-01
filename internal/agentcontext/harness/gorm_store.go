package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/internal/agentcontext/writeback"
	"YoudaoNoteLm/internal/model/entity"
)

// PersistentRun 是最小 Harness 的 MySQL Run 表。
type PersistentRun struct {
	RunID               string   `gorm:"primaryKey;type:varchar(36)"`
	StepID              string   `gorm:"type:varchar(64);not null"`
	AgentID             string   `gorm:"type:varchar(32);not null;index"`
	UserID              uint     `gorm:"not null;index"`
	ConversationID      uint     `gorm:"not null;index"`
	InputHash           string   `gorm:"type:char(64);not null"`
	CurrentInputRefJSON string   `gorm:"type:json"`
	ProfileJSON         string   `gorm:"type:json;not null"`
	ContextMode         string   `gorm:"type:varchar(16);not null"`
	WritebackOwner      string   `gorm:"type:varchar(16);not null"`
	ContractVersion     string   `gorm:"type:varchar(32);not null"`
	State               RunState `gorm:"type:varchar(16);not null;index"`
	AttemptID           string   `gorm:"type:varchar(36);not null"`
	FencingToken        uint64   `gorm:"not null"`
	StateVersion        uint64   `gorm:"not null"`
	Revision            uint64   `gorm:"not null"`
	FinalStatus         string   `gorm:"type:varchar(16)"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (PersistentRun) TableName() string { return "agent_context_runs" }

// PersistentManifest 只保存 ContextManifest 元数据，不保存 Prompt、消息或工具正文。
type PersistentManifest struct {
	ID              uint   `gorm:"primaryKey;autoIncrement"`
	IdempotencyKey  string `gorm:"type:varchar(191);not null;uniqueIndex"`
	RunID           string `gorm:"type:varchar(36);not null;index"`
	Revision        uint64 `gorm:"not null"`
	ModelCallID     string `gorm:"type:varchar(191);not null"`
	ProfileID       string `gorm:"type:varchar(64);not null"`
	ProfileVersion  string `gorm:"type:varchar(32);not null"`
	PromptVersion   string `gorm:"type:varchar(64)"`
	ToolsetVersion  string `gorm:"type:varchar(64)"`
	Model           string `gorm:"type:varchar(191)"`
	InputBudget     int
	EstimatedTokens int
	ExactTokens     *int
	CounterMode     string `gorm:"type:varchar(32)"`
	SourcesJSON     string `gorm:"type:json"`
	TurnStatus      string `gorm:"type:varchar(16);not null"`
	Degraded        bool
	ContextHMAC     string `gorm:"type:char(64)"`
	CreatedAt       time.Time
}

func (PersistentManifest) TableName() string { return "agent_context_manifests" }

// PersistentWriteback 记录主结果写入的稳定幂等键。
type PersistentWriteback struct {
	ID               uint   `gorm:"primaryKey;autoIncrement"`
	IdempotencyKey   string `gorm:"type:varchar(191);not null;uniqueIndex"`
	RunID            string `gorm:"type:varchar(36);not null;index"`
	Revision         uint64 `gorm:"not null"`
	Operation        string `gorm:"type:varchar(32);not null"`
	Status           string `gorm:"type:varchar(16);not null"`
	PrimaryMessageID uint
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (PersistentWriteback) TableName() string { return "agent_context_writebacks" }

type AssistantCacheWriter interface {
	AddMessage(ctx context.Context, conversationID uint, userMsg, assistantMsg string) error
}

// GormStore 提供单数据库、单进程最小 Harness 持久化。
type GormStore struct {
	db    *gorm.DB
	cache AssistantCacheWriter
}

func NewGormStore(db *gorm.DB, cache AssistantCacheWriter) *GormStore {
	return &GormStore{db: db, cache: cache}
}

func (s *GormStore) CreateRun(ctx context.Context, record RunRecord) error {
	persistent, err := encodeRun(record)
	if err != nil {
		return err
	}
	err = s.db.WithContext(ctx).Create(&persistent).Error
	if isDuplicateKey(err) {
		return ErrRunAlreadyExists
	}
	return err
}

func (s *GormStore) GetRun(ctx context.Context, runID string) (RunRecord, error) {
	var persistent PersistentRun
	err := s.db.WithContext(ctx).First(&persistent, "run_id = ?", runID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunRecord{}, ErrRunNotFound
	}
	if err != nil {
		return RunRecord{}, err
	}
	return decodeRun(persistent)
}

func (s *GormStore) BeginFinalization(
	ctx context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) (RunRecord, error) {
	var updated RunRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var persistent PersistentRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&persistent, "run_id = ?", runID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRunNotFound
		}
		if err != nil {
			return err
		}
		record, err := decodeRun(persistent)
		if err != nil {
			return err
		}
		if record.State == RunStateFinalizing || isTerminal(record.State) {
			if sameExecution(record.Authority, authority) && record.FinalStatus == status {
				updated = record
				return nil
			}
			return ErrInvalidState
		}
		if record.State != RunStateRunning {
			return ErrInvalidState
		}
		if record.Authority != authority {
			return ErrAuthorityStale
		}

		nextVersion := record.Authority.RunStateVersion + 1
		result := tx.Model(&PersistentRun{}).
			Where("run_id = ? AND state_version = ? AND state = ?", runID, authority.RunStateVersion, RunStateRunning).
			Updates(map[string]interface{}{
				"state":         RunStateFinalizing,
				"state_version": nextVersion,
				"revision":      record.Revision + 1,
				"final_status":  string(status),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAuthorityStale
		}
		record.State = RunStateFinalizing
		record.Revision++
		record.FinalStatus = status
		record.Authority.RunStateVersion = nextVersion
		updated = record
		return nil
	})
	return updated, err
}

func (s *GormStore) CompleteRun(
	ctx context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) (RunRecord, error) {
	var updated RunRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var persistent PersistentRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&persistent, "run_id = ?", runID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRunNotFound
		}
		if err != nil {
			return err
		}
		record, err := decodeRun(persistent)
		if err != nil {
			return err
		}
		if isTerminal(record.State) {
			if sameExecution(record.Authority, authority) && record.FinalStatus == status {
				updated = record
				return nil
			}
			return ErrInvalidState
		}
		if record.State != RunStateFinalizing || record.Authority != authority || record.FinalStatus != status {
			return ErrAuthorityStale
		}

		nextState := terminalState(status)
		nextVersion := authority.RunStateVersion + 1
		result := tx.Model(&PersistentRun{}).
			Where("run_id = ? AND state_version = ? AND state = ?", runID, authority.RunStateVersion, RunStateFinalizing).
			Updates(map[string]interface{}{
				"state":         nextState,
				"state_version": nextVersion,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAuthorityStale
		}
		record.State = nextState
		record.Authority.RunStateVersion = nextVersion
		updated = record
		return nil
	})
	return updated, err
}

func (s *GormStore) StoreManifest(
	ctx context.Context,
	req writeback.ManifestWriteRequest,
) error {
	sourcesJSON, err := json.Marshal(req.Manifest.Sources)
	if err != nil {
		return fmt.Errorf("编码 Manifest Sources 失败: %w", err)
	}
	record := PersistentManifest{
		IdempotencyKey:  req.IdempotencyKey,
		RunID:           req.FinalizeKey.RunID,
		Revision:        req.FinalizeKey.Revision,
		ModelCallID:     req.ModelCallID,
		ProfileID:       req.Manifest.ProfileID,
		ProfileVersion:  req.Manifest.ProfileVersion,
		PromptVersion:   req.Manifest.PromptVersion,
		ToolsetVersion:  req.Manifest.ToolsetVersion,
		Model:           req.Manifest.Model,
		InputBudget:     req.Manifest.InputBudget,
		EstimatedTokens: req.Manifest.EstimatedTokens,
		ExactTokens:     req.Manifest.ExactTokens,
		CounterMode:     req.Manifest.CounterMode,
		SourcesJSON:     string(sourcesJSON),
		TurnStatus:      req.TurnStatus,
		Degraded:        req.Manifest.Degraded,
		ContextHMAC:     req.Manifest.ContextHMAC,
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).
		Create(&record).Error
}

func (s *GormStore) CommitAssistant(
	ctx context.Context,
	req writeback.AssistantWriteRequest,
) (writeback.CommittedMessage, error) {
	var committed writeback.CommittedMessage
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing PersistentWriteback
		err := tx.First(&existing, "idempotency_key = ?", req.IdempotencyKey).Error
		if err == nil {
			committed.MessageID = existing.PrimaryMessageID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var run PersistentRun
		result := tx.First(&run,
			"run_id = ? AND state = ? AND attempt_id = ? AND fencing_token = ? AND state_version = ?",
			req.RunID,
			RunStateFinalizing,
			req.Authority.AttemptID,
			req.Authority.FencingToken,
			req.Authority.RunStateVersion,
		)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrAuthorityStale
		}
		if result.Error != nil {
			return result.Error
		}

		metadata := string(req.References)
		if metadata == "" {
			metadata = "{}"
		}
		message := &entity.Message{
			ConversationID: req.ConversationID,
			Role:           "assistant",
			Content:        req.Content,
			Metadata:       metadata,
		}
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		writebackRecord := PersistentWriteback{
			IdempotencyKey:   req.IdempotencyKey,
			RunID:            req.RunID,
			Revision:         req.FinalizeKey.Revision,
			Operation:        string(writeback.WritebackOperationAssistant),
			Status:           string(writeback.WritebackStatusSuccess),
			PrimaryMessageID: message.ID,
		}
		if err := tx.Create(&writebackRecord).Error; err != nil {
			return err
		}
		committed.MessageID = message.ID
		return nil
	})
	if err != nil {
		return writeback.CommittedMessage{}, err
	}

	if s.cache != nil {
		_ = s.cache.AddMessage(ctx, req.ConversationID, req.UserContent, req.Content)
	}
	return committed, nil
}

func encodeRun(record RunRecord) (PersistentRun, error) {
	profileJSON, err := json.Marshal(record.Profile)
	if err != nil {
		return PersistentRun{}, fmt.Errorf("编码 Profile 快照失败: %w", err)
	}
	inputRefJSON, err := json.Marshal(record.Handle.CurrentInputRef)
	if err != nil {
		return PersistentRun{}, fmt.Errorf("编码输入引用失败: %w", err)
	}
	return PersistentRun{
		RunID:               record.Handle.RunID,
		StepID:              record.Handle.StepID,
		AgentID:             string(record.Handle.AgentID),
		UserID:              record.Handle.UserID,
		ConversationID:      record.Handle.ConversationID,
		InputHash:           record.InputHash,
		CurrentInputRefJSON: string(inputRefJSON),
		ProfileJSON:         string(profileJSON),
		ContextMode:         record.Handle.ContextMode.Mode,
		WritebackOwner:      record.Handle.ContextMode.WritebackOwner,
		ContractVersion:     record.Handle.ContextMode.ContractVersion,
		State:               record.State,
		AttemptID:           record.Authority.AttemptID,
		FencingToken:        record.Authority.FencingToken,
		StateVersion:        record.Authority.RunStateVersion,
		Revision:            record.Revision,
		FinalStatus:         string(record.FinalStatus),
	}, nil
}

func decodeRun(persistent PersistentRun) (RunRecord, error) {
	var profile agentcontext.ContextProfileSnapshot
	if err := json.Unmarshal([]byte(persistent.ProfileJSON), &profile); err != nil {
		return RunRecord{}, fmt.Errorf("解码 Profile 快照失败: %w", err)
	}
	var inputRef *agentcontext.MessageRef
	if persistent.CurrentInputRefJSON != "" && persistent.CurrentInputRefJSON != "null" {
		inputRef = &agentcontext.MessageRef{}
		if err := json.Unmarshal([]byte(persistent.CurrentInputRefJSON), inputRef); err != nil {
			return RunRecord{}, fmt.Errorf("解码输入引用失败: %w", err)
		}
	}
	return RunRecord{
		Handle: agentcontext.AcceptedTurnHandle{
			RunID:           persistent.RunID,
			StepID:          persistent.StepID,
			AgentID:         agentcontext.AgentID(persistent.AgentID),
			UserID:          persistent.UserID,
			ConversationID:  persistent.ConversationID,
			CurrentInputRef: inputRef,
			ContextMode: agentcontext.ContextModeSnapshot{
				Mode:            persistent.ContextMode,
				WritebackOwner:  persistent.WritebackOwner,
				ContractVersion: persistent.ContractVersion,
			},
		},
		Profile:   profile,
		InputHash: persistent.InputHash,
		State:     persistent.State,
		Authority: agentcontext.ActiveExecutionAuthority{
			AttemptID:       persistent.AttemptID,
			FencingToken:    persistent.FencingToken,
			RunStateVersion: persistent.StateVersion,
		},
		Revision:    persistent.Revision,
		FinalStatus: agentcontext.TurnStatus(persistent.FinalStatus),
	}, nil
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

var (
	_ Store                            = (*GormStore)(nil)
	_ writeback.ManifestWriter         = (*GormStore)(nil)
	_ writeback.AssistantMessageWriter = (*GormStore)(nil)
)
