package finalization

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"YoudaoNoteLm/internal/agentharness/core"
)

// FinalizationStore 终态化所需的存储接口。
type FinalizationStore interface {
	// CreateJournalEntry 创建写回日志条目。
	CreateJournalEntry(entry WritebackJournal) error
	// GetJournalEntry 获取写回日志条目。
	GetJournalEntry(id string) (WritebackJournal, error)
	// UpdateJournalEntry 更新写回日志条目。
	UpdateJournalEntry(entry WritebackJournal) error
	// FinalizeRun 终态化 Run（CAS）。
	FinalizeRun(runID string, authority core.ExecutionAuthority, expectedVersion core.StateVersion, newState core.RunState) (core.StateVersion, error)
}

// MessageWriter 消息写入器接口。
type MessageWriter interface {
	// WriteAssistantMessage 写入 assistant 消息（幂等）。
	WriteAssistantMessage(runID string, content string, idempotencyKey string) (string, error)
}

// FinalizationPort 实现 core.FinalizationPort 接口。
type FinalizationPort struct {
	store    FinalizationStore
	writer   MessageWriter
}

// NewFinalizationPort 创建 FinalizationPort。
func NewFinalizationPort(store FinalizationStore, writer MessageWriter) *FinalizationPort {
	return &FinalizationPort{
		store:  store,
		writer: writer,
	}
}

// Finalize 实现终态化。
func (f *FinalizationPort) Finalize(req core.FinalizationRequest) (core.FinalizeResult, error) {
	// 1. 生成稳定 FinalizeKey
	finalizeKey := generateFinalizeKey(req.RunID, req.Revision)

	// 2. 创建 assistant writeback journal entry
	authorityJSON, _ := json.Marshal(req.Authority)
	journalEntry := WritebackJournal{
		ID:             uuid.NewString(),
		RunID:          string(req.RunID),
		Revision:       uint64(req.Revision),
		OperationType:  OpTypeAssistantMessage,
		Status:         OpStatusPending,
		IdempotencyKey: finalizeKey,
		AuthorityJSON:  string(authorityJSON),
		MaxAttempts:    3,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// 3. 根据 Outcome 处理
	var newState core.RunState
	switch req.Outcome.Status {
	case core.OutcomeStatusSuccess:
		newState = core.RunStateSucceeded
		// 成功时写入 assistant message
		if err := f.writeAssistantMessage(req, journalEntry); err != nil {
			return core.FinalizeResult{}, err
		}
	case core.OutcomeStatusFailed:
		newState = core.RunStateFailed
		journalEntry.Status = OpStatusSkipped
		if req.Outcome.ErrorClass != nil {
			ec := string(*req.Outcome.ErrorClass)
			journalEntry.ErrorClass = &ec
		}
		if req.Outcome.ErrorCode != nil {
			journalEntry.ErrorCode = req.Outcome.ErrorCode
		}
		if err := f.store.CreateJournalEntry(journalEntry); err != nil {
			return core.FinalizeResult{}, fmt.Errorf("创建 journal 失败: %w", err)
		}
	case core.OutcomeStatusCancelled:
		newState = core.RunStateCancelled
		journalEntry.Status = OpStatusSkipped
		if err := f.store.CreateJournalEntry(journalEntry); err != nil {
			return core.FinalizeResult{}, fmt.Errorf("创建 journal 失败: %w", err)
		}
	default:
		return core.FinalizeResult{}, fmt.Errorf("未知的 Outcome 状态: %s", req.Outcome.Status)
	}

	// 4. CAS finalizing -> terminal
	newVersion, err := f.store.FinalizeRun(
		string(req.RunID),
		req.Authority,
		req.FinalizingStateVersion,
		newState,
	)
	if err != nil {
		return core.FinalizeResult{}, fmt.Errorf("终态化 Run 失败: %w", err)
	}

	return core.FinalizeResult{
		NewState:    newState,
		NewVersion:  newVersion,
		NewRevision: req.Revision + 1,
	}, nil
}

// writeAssistantMessage 写入 assistant 消息。
func (f *FinalizationPort) writeAssistantMessage(req core.FinalizationRequest, journalEntry WritebackJournal) error {
	// 准备消息内容
	content := ""
	if req.Outcome.Status == core.OutcomeStatusSuccess {
		content = "Agent 执行成功" // 实际内容从 Outcome 中获取
	}

	// 创建 journal entry
	if err := f.store.CreateJournalEntry(journalEntry); err != nil {
		return fmt.Errorf("创建 journal 失败: %w", err)
	}

	// 写入 assistant message
	_, err := f.writer.WriteAssistantMessage(
		string(req.RunID),
		content,
		journalEntry.IdempotencyKey,
	)
	if err != nil {
		// 更新 journal 状态为 failed
		journalEntry.Status = OpStatusFailed
		ec := string(core.ErrorClassPermanent)
		journalEntry.ErrorClass = &ec
		f.store.UpdateJournalEntry(journalEntry)
		return fmt.Errorf("写入 assistant message 失败: %w", err)
	}

	// 更新 journal 状态为 completed
	journalEntry.Status = OpStatusCompleted
	now := time.Now()
	journalEntry.FinishedAt = &now
	return f.store.UpdateJournalEntry(journalEntry)
}

// generateFinalizeKey 生成稳定的 FinalizeKey。
func generateFinalizeKey(runID core.RunID, revision core.Revision) string {
	return fmt.Sprintf("%s:%d", string(runID), revision)
}
