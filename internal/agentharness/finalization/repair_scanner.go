package finalization

import (
	"context"
	"time"

	"YoudaoNoteLm/pkg/logger"

	"go.uber.org/zap"
)

// RepairStore Repair Scanner 所需的存储接口。
type RepairStore interface {
	// GetExpiredJournalEntries 获取过期且未完成的 journal 条目。
	GetExpiredJournalEntries(before time.Time, limit int) ([]WritebackJournal, error)
	// UpdateJournalEntry 更新 journal 条目。
	UpdateJournalEntry(entry WritebackJournal) error
}

// RepairScanner 修复扫描器。
// 首期由 all 角色的有界定时任务执行。
type RepairScanner struct {
	store   RepairStore
	writer  MessageWriter
	stopCh  chan struct{}
}

// NewRepairScanner 创建 RepairScanner。
func NewRepairScanner(store RepairStore, writer MessageWriter) *RepairScanner {
	return &RepairScanner{
		store:  store,
		writer: writer,
		stopCh: make(chan struct{}),
	}
}

// Start 启动 RepairScanner。
func (s *RepairScanner) Start(ctx context.Context, interval time.Duration) {
	go s.run(ctx, interval)
}

// Stop 停止 RepairScanner。
func (s *RepairScanner) Stop() {
	close(s.stopCh)
}

// run 运行扫描循环。
func (s *RepairScanner) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

// scan 执行一次扫描。
func (s *RepairScanner) scan(ctx context.Context) {
	// 获取过期的 journal 条目（5分钟前）
	before := time.Now().Add(-5 * time.Minute)
	entries, err := s.store.GetExpiredJournalEntries(before, 10)
	if err != nil {
		logger.Error("RepairScanner 获取过期条目失败", zap.Error(err))
		return
	}

	for _, entry := range entries {
		s.processEntry(ctx, entry)
	}
}

// processEntry 处理单个 journal 条目。
func (s *RepairScanner) processEntry(ctx context.Context, entry WritebackJournal) {
	// 检查是否超过最大尝试次数
	if entry.AttemptCount >= entry.MaxAttempts {
		entry.Status = OpStatusFailed
		s.store.UpdateJournalEntry(entry)
		return
	}

	// 更新尝试次数
	entry.AttemptCount++
	entry.Status = OpStatusInProgress
	entry.UpdatedAt = time.Now()
	if err := s.store.UpdateJournalEntry(entry); err != nil {
		logger.Error("RepairScanner 更新条目失败", zap.Error(err))
		return
	}

	// 根据操作类型处理
	switch entry.OperationType {
	case OpTypeAssistantMessage:
		s.retryAssistantMessage(ctx, entry)
	default:
		// 其他操作类型暂不处理
		entry.Status = OpStatusSkipped
		s.store.UpdateJournalEntry(entry)
	}
}

// retryAssistantMessage 重试 assistant message 写入。
func (s *RepairScanner) retryAssistantMessage(ctx context.Context, entry WritebackJournal) {
	// 写入 assistant message
	_, err := s.writer.WriteAssistantMessage(
		entry.RunID,
		"Agent 执行成功", // 实际内容需要从其他地方获取
		entry.IdempotencyKey,
	)
	if err != nil {
		logger.Error("RepairScanner 重试写入 assistant message 失败",
			zap.String("runID", entry.RunID),
			zap.Error(err),
		)
		// 设置下次重试时间
		nextRetry := time.Now().Add(1 * time.Minute)
		entry.NextRetryAt = &nextRetry
		entry.Status = OpStatusFailed
		ec := string("transient")
		entry.ErrorClass = &ec
		s.store.UpdateJournalEntry(entry)
		return
	}

	// 成功
	entry.Status = OpStatusCompleted
	now := time.Now()
	entry.FinishedAt = &now
	s.store.UpdateJournalEntry(entry)
}
