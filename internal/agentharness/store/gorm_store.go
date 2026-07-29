package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"YoudaoNoteLm/internal/agentharness/core"
)

// GormStore 实现 core.RunStore 接口。
type GormStore struct {
	db *gorm.DB
}

// NewGormStore 创建 GormStore。
func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

// CreateQueued 创建一个 queued 状态的 Run。
func (s *GormStore) CreateQueued(ctx context.Context, run core.Run) error {
	snapshotJSON, err := json.Marshal(run.VersionSnapshot)
	if err != nil {
		return fmt.Errorf("编码 VersionSnapshot 失败: %w", err)
	}

	record := AgentRun{
		ID:                  string(run.ID),
		ParentRunID:         parentRunIDPtr(run.ParentRunID),
		AgentType:           run.AgentType,
		UserID:              run.UserID,
		NotebookID:          run.NotebookID,
		ConversationID:      run.ConversationID,
		InputKind:           run.Input.Kind,
		InputRef:            run.Input.Ref,
		InputHash:           run.Input.Hash,
		VersionSnapshotJSON: string(snapshotJSON),
		State:               core.RunStateQueued,
		DesiredState:        core.DesiredStateRunning,
		StateVersion:        1,
		Revision:            0,
		FencingToken:        0,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	err = s.db.WithContext(ctx).Create(&record).Error
	if isDuplicateKey(err) {
		return core.ErrRunAlreadyExists
	}
	return err
}

// Get 获取 Run。
func (s *GormStore) Get(ctx context.Context, id core.RunID) (core.Run, error) {
	var record AgentRun
	err := s.db.WithContext(ctx).First(&record, "id = ?", string(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return core.Run{}, core.ErrRunNotFound
	}
	if err != nil {
		return core.Run{}, err
	}
	return decodeRun(record)
}

// Claim 原子地 claim 一个 queued Run。
func (s *GormStore) Claim(ctx context.Context, id core.RunID, workerID string) (core.Run, core.Attempt, error) {
	var run core.Run
	var attempt core.Attempt

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record AgentRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ? AND state = ?", string(id), core.RunStateQueued).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.ErrNotQueued
		}
		if err != nil {
			return err
		}

		// 递增 fencing token
		newFencingToken := record.FencingToken + 1
		newStateVersion := record.StateVersion + 1
		now := time.Now()
		attemptID := generateID()
		attemptNumber := 1

		// 查询当前最大的 attempt_number
		var maxAttempt struct {
			MaxNumber *int
		}
		tx.Model(&AgentRunAttempt{}).
			Where("run_id = ?", string(id)).
			Select("MAX(attempt_number) as max_number").
			Scan(&maxAttempt)
		if maxAttempt.MaxNumber != nil {
			attemptNumber = *maxAttempt.MaxNumber + 1
		}

		// 更新 Run
		result := tx.Model(&AgentRun{}).
			Where("id = ? AND state_version = ? AND state = ?", string(id), record.StateVersion, core.RunStateQueued).
			Updates(map[string]interface{}{
				"state":               core.RunStateRunning,
				"state_version":       newStateVersion,
				"fencing_token":       newFencingToken,
				"current_attempt_id":  attemptID,
				"started_at":          now,
				"updated_at":          now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return core.ErrAuthorityStale
		}

		// 创建 Attempt
		attemptRecord := AgentRunAttempt{
			ID:            attemptID,
			RunID:         string(id),
			AttemptNumber: uint(attemptNumber),
			WorkerID:      &workerID,
			FencingToken:  newFencingToken,
			State:         core.AttemptStateRunning,
			StartedAt:     now,
		}
		if err := tx.Create(&attemptRecord).Error; err != nil {
			return err
		}

		// 解码并返回
		updatedRecord := record
		updatedRecord.State = core.RunStateRunning
		updatedRecord.StateVersion = newStateVersion
		updatedRecord.FencingToken = newFencingToken
		updatedRecord.CurrentAttemptID = &attemptID
		updatedRecord.StartedAt = &now

		var err2 error
		run, err2 = decodeRun(updatedRecord)
		if err2 != nil {
			return err2
		}

		attempt = core.Attempt{
			ID:            core.AttemptID(attemptID),
			RunID:         id,
			AttemptNumber: uint(attemptNumber),
			WorkerID:      workerID,
			FencingToken:  core.FencingToken(newFencingToken),
			State:         core.AttemptStateRunning,
			StartedAt:     now.Unix(),
		}

		return nil
	})

	return run, attempt, err
}

// Transition 执行状态转换。
func (s *GormStore) Transition(ctx context.Context, req core.TransitionRequest) (core.Run, error) {
	var updated core.Run

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record AgentRun
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ?", string(req.RunID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.ErrRunNotFound
		}
		if err != nil {
			return err
		}

		// 验证当前状态
		if record.State != req.CurrentState {
			return core.ErrAuthorityStale
		}
		// 验证版本
		if record.StateVersion != uint64(req.StateVersion) {
			return core.ErrAuthorityStale
		}
		// 验证 fencing token
		if req.FencingToken > 0 && record.FencingToken != uint64(req.FencingToken) {
			return core.ErrAuthorityStale
		}

		newStateVersion := record.StateVersion + 1
		now := time.Now()
		updates := map[string]interface{}{
			"state":         req.TargetState,
			"state_version": newStateVersion,
			"updated_at":    now,
		}

		// 设置错误信息
		if req.ErrorClass != nil {
			ec := string(*req.ErrorClass)
			updates["last_error_class"] = ec
		}
		if req.ErrorCode != nil {
			updates["last_error_code"] = *req.ErrorCode
		}

		// 如果是终态，设置 finished_at
		if core.IsTerminalRunState(req.TargetState) {
			updates["finished_at"] = now
		}

		result := tx.Model(&AgentRun{}).
			Where("id = ? AND state_version = ?", string(req.RunID), record.StateVersion).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return core.ErrAuthorityStale
		}

		record.State = req.TargetState
		record.StateVersion = newStateVersion
		if req.ErrorClass != nil {
			ec := string(*req.ErrorClass)
			records := ec
			record.LastErrorClass = &records
		}
		if req.ErrorCode != nil {
			record.LastErrorCode = req.ErrorCode
		}
		if core.IsTerminalRunState(req.TargetState) {
			record.FinishedAt = &now
		}

		var err2 error
		updated, err2 = decodeRun(record)
		return err2
	})

	return updated, err
}

// CreateStep 创建 Step。
func (s *GormStore) CreateStep(ctx context.Context, step core.Step) error {
	record := AgentRunStep{
		ID:            string(step.ID),
		RunID:         string(step.RunID),
		AttemptID:     string(step.AttemptID),
		ParentStepID:  stepIDPtr(step.ParentStepID),
		Kind:          step.Kind,
		AgentName:     step.AgentName,
		State:         step.State,
		InputHash:     step.InputHash,
		ToolCallID:    step.ToolCallID,
		FencingToken:  uint64(step.FencingToken),
		ErrorClass:    step.ErrorClass,
		ErrorCode:     step.ErrorCode,
		StartedAt:     time.Unix(step.StartedAt, 0),
		FinishedAt:    finishedAtPtr(step.FinishedAt),
	}

	err := s.db.WithContext(ctx).Create(&record).Error
	if isDuplicateKey(err) {
		return core.ErrStepAlreadyExists
	}
	return err
}

// FinishStep 完成 Step。
func (s *GormStore) FinishStep(ctx context.Context, req core.FinishStepRequest) (core.Step, error) {
	var updated core.Step

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record AgentRunStep
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&record, "id = ? AND run_id = ? AND attempt_id = ?", string(req.StepID), string(req.RunID), string(req.AttemptID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.ErrStepNotFound
		}
		if err != nil {
			return err
		}

		// 验证 fencing token
		if record.FencingToken != uint64(req.FencingToken) {
			return core.ErrAuthorityStale
		}

		now := time.Now()
		updates := map[string]interface{}{
			"state":       req.State,
			"finished_at": now,
		}
		if req.ErrorClass != nil {
			updates["error_class"] = string(*req.ErrorClass)
		}
		if req.ErrorCode != nil {
			updates["error_code"] = *req.ErrorCode
		}
		if req.ResultArtifactRef != nil {
			updates["result_artifact_ref"] = *req.ResultArtifactRef
		}

		result := tx.Model(&AgentRunStep{}).
			Where("id = ? AND fencing_token = ?", string(req.StepID), uint64(req.FencingToken)).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return core.ErrAuthorityStale
		}

		record.State = req.State
		record.FinishedAt = &now
		if req.ErrorClass != nil {
			record.ErrorClass = req.ErrorClass
		}
		if req.ErrorCode != nil {
			record.ErrorCode = req.ErrorCode
		}
		if req.ResultArtifactRef != nil {
			record.ResultArtifactRef = req.ResultArtifactRef
		}

		updated = decodeStep(record)
		return nil
	})

	return updated, err
}

// decodeRun 将 GORM 模型解码为领域类型。
func decodeRun(record AgentRun) (core.Run, error) {
	var snapshot core.VersionSnapshot
	if err := json.Unmarshal([]byte(record.VersionSnapshotJSON), &snapshot); err != nil {
		return core.Run{}, fmt.Errorf("解码 VersionSnapshot 失败: %w", err)
	}

	var parentRunID *core.RunID
	if record.ParentRunID != nil {
		rid := core.RunID(*record.ParentRunID)
		parentRunID = &rid
	}

	var authority *core.ExecutionAuthority
	if record.CurrentAttemptID != nil {
		authority = &core.ExecutionAuthority{
			AttemptID:       core.AttemptID(*record.CurrentAttemptID),
			FencingToken:    core.FencingToken(record.FencingToken),
			RunStateVersion: core.StateVersion(record.StateVersion),
		}
	}

	return core.Run{
		ID:              core.RunID(record.ID),
		ParentRunID:     parentRunID,
		AgentType:       record.AgentType,
		UserID:          record.UserID,
		NotebookID:      record.NotebookID,
		ConversationID:  record.ConversationID,
		Input: core.InputRef{
			Kind: record.InputKind,
			Ref:  record.InputRef,
			Hash: record.InputHash,
		},
		VersionSnapshot: snapshot,
		State:           record.State,
		DesiredState:    record.DesiredState,
		StateVersion:    core.StateVersion(record.StateVersion),
		Revision:        core.Revision(record.Revision),
		Authority:       authority,
	}, nil
}

// decodeStep 将 GORM 模型解码为领域类型。
func decodeStep(record AgentRunStep) core.Step {
	var parentStepID *core.StepID
	if record.ParentStepID != nil {
		sid := core.StepID(*record.ParentStepID)
		parentStepID = &sid
	}

	return core.Step{
		ID:                core.StepID(record.ID),
		RunID:             core.RunID(record.RunID),
		AttemptID:         core.AttemptID(record.AttemptID),
		ParentStepID:      parentStepID,
		Kind:              record.Kind,
		AgentName:         record.AgentName,
		State:             record.State,
		InputHash:         record.InputHash,
		ToolCallID:        record.ToolCallID,
		ResultArtifactRef: record.ResultArtifactRef,
		FencingToken:      core.FencingToken(record.FencingToken),
		ErrorClass:        record.ErrorClass,
		ErrorCode:         record.ErrorCode,
		StartedAt:         record.StartedAt.Unix(),
		FinishedAt:        finishedAtUnixPtr(record.FinishedAt),
	}
}

func parentRunIDPtr(id *core.RunID) *string {
	if id == nil {
		return nil
	}
	s := string(*id)
	return &s
}

func stepIDPtr(id *core.StepID) *string {
	if id == nil {
		return nil
	}
	s := string(*id)
	return &s
}

func finishedAtPtr(t *int64) *time.Time {
	if t == nil {
		return nil
	}
	tm := time.Unix(*t, 0)
	return &tm
}

func finishedAtUnixPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	unix := t.Unix()
	return &unix
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// generateID 生成 UUIDv7。首期实现使用简单的时间戳+随机数。
func generateID() string {
	// TODO: 使用真正的 UUIDv7 生成器
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000000)
}

// --- AdmissionStore 实现 ---

// messageRecord 是 messages 表的 GORM 模型（仅用于 Accept 事务内创建）。
type messageRecord struct {
	ID             uint   `gorm:"primaryKey;autoIncrement"`
	ConversationID uint   `gorm:"not null;index"`
	Role           string `gorm:"type:varchar(32);not null"`
	Content        string `gorm:"type:text;not null"`
	Metadata       string `gorm:"type:json"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (messageRecord) TableName() string { return "messages" }

// Accept 在一个事务中完成：创建入口 Message、queued Run 和首条 run.accepted 事件。
func (s *GormStore) Accept(ctx context.Context, req core.AcceptRequest) (core.AcceptedRun, error) {
	var result core.AcceptedRun

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 幂等查询：按 (user_id, idempotency_key) 查找既有 Run
		var existing AgentRun
		err := tx.Where("user_id = ? AND idempotency_key = ?", req.UserID, req.IdempotencyKey).
			First(&existing).Error
		if err == nil {
			// 幂等命中：验证请求不可变字段一致
			if existing.AgentType != req.AgentType ||
				existing.InputKind != req.Input.Kind ||
				existing.InputRef != req.Input.Ref {
				return core.ErrIdempotencyKeyConflict
			}
			// 返回既有结果
			result = core.AcceptedRun{
				RunID:              core.RunID(existing.ID),
				State:              existing.State,
				Sequence:           existing.Sequence,
				IsIdempotentReplay: true,
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 2. 计算 conversation 内序号（使用 SELECT MAX(sequence) + 1）
		var nextSequence uint64 = 1
		if req.ConversationID != nil {
			var maxSeq struct {
				MaxSeq *uint64
			}
			tx.Model(&AgentRun{}).
				Where("conversation_id = ?", *req.ConversationID).
				Select("MAX(sequence) as max_seq").
				Scan(&maxSeq)
			if maxSeq.MaxSeq != nil {
				nextSequence = *maxSeq.MaxSeq + 1
			}
		}

		// 3. 创建入口 Message
		msgID := generateID()
		msg := messageRecord{
			ID:             0, // auto increment
			ConversationID: derefUint(req.ConversationID),
			Role:           "user",
			Content:        req.Input.Ref, // InputRef.Ref 指向消息内容标识
			Metadata:       "{}",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if err := tx.Create(&msg).Error; err != nil {
			return fmt.Errorf("创建入口消息失败: %w", err)
		}
		// 获取自增 ID
		msgID = fmt.Sprintf("%d", msg.ID)

		// 4. 冻结 VersionSnapshot 并创建 queued Run
		snapshotJSON, err := json.Marshal(req.VersionSnapshot)
		if err != nil {
			return fmt.Errorf("编码 VersionSnapshot 失败: %w", err)
		}

		runID := generateID()
		run := AgentRun{
			ID:                  runID,
			AgentType:           req.AgentType,
			UserID:              req.UserID,
			NotebookID:          req.NotebookID,
			ConversationID:      req.ConversationID,
			IdempotencyKey:      req.IdempotencyKey,
			Sequence:            nextSequence,
			InputKind:           req.Input.Kind,
			InputRef:            fmt.Sprintf("chat_message:%s", msgID),
			InputHash:           req.Input.Hash,
			VersionSnapshotJSON: string(snapshotJSON),
			State:               core.RunStateQueued,
			DesiredState:        core.DesiredStateRunning,
			StateVersion:        1,
			Revision:            0,
			FencingToken:        0,
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}
		if err := tx.Create(&run).Error; err != nil {
			if isDuplicateKey(err) {
				return core.ErrRunAlreadyExists
			}
			return fmt.Errorf("创建 Run 失败: %w", err)
		}

		// 5. 插入首条 run.accepted 事件 (sequence=1)
		eventID := generateID()
		event := AgentRunEvent{
			ID:              generateID(),
			RunID:           runID,
			Sequence:        1,
			EventID:         eventID,
			EventType:       string(core.EventRunAccepted),
			PayloadVersion:  1,
			PayloadJSON:     fmt.Sprintf(`{"run_id":"%s","agent_type":"%s","user_id":%d}`, runID, req.AgentType, req.UserID),
			CreatedAt:       time.Now(),
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("创建事件失败: %w", err)
		}

		result = core.AcceptedRun{
			RunID:              core.RunID(runID),
			MessageID:          msgID,
			State:              core.RunStateQueued,
			Sequence:           nextSequence,
			IsIdempotentReplay: false,
		}
		return nil
	})

	return result, err
}

// derefUint 解引用 *uint，nil 时返回 0。
func derefUint(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}
