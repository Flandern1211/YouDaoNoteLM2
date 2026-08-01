package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/internal/agentcontext/writeback"
)

// Service 提供最小 Run 接受、执行权校验和终态状态机。
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) AcceptTurn(
	ctx context.Context,
	handle agentcontext.AcceptedTurnHandle,
	profile agentcontext.ContextProfileSnapshot,
) (agentcontext.AcceptedTurnHandle, agentcontext.ActiveExecutionAuthority, error) {
	if s == nil || s.store == nil {
		return agentcontext.AcceptedTurnHandle{}, agentcontext.ActiveExecutionAuthority{}, fmt.Errorf("Harness Store 未配置")
	}
	if handle.RunID == "" {
		handle.RunID = uuid.NewString()
	}
	if handle.StepID == "" {
		handle.StepID = "step-0"
	}
	authority := agentcontext.ActiveExecutionAuthority{
		AttemptID:       uuid.NewString(),
		FencingToken:    1,
		RunStateVersion: 1,
	}
	record := RunRecord{
		Handle:    handle,
		Profile:   profile,
		InputHash: hashTurnInput(handle.Input),
		State:     RunStateRunning,
		Authority: authority,
	}
	if err := s.store.CreateRun(ctx, record); err != nil {
		return agentcontext.AcceptedTurnHandle{}, agentcontext.ActiveExecutionAuthority{}, err
	}
	return handle, authority, nil
}

// VerifyAccepted 实现 writeback.TurnVerifier。
func (s *Service) VerifyAccepted(
	ctx context.Context,
	handle agentcontext.AcceptedTurnHandle,
) (writeback.VerifiedTurn, error) {
	record, err := s.store.GetRun(ctx, handle.RunID)
	if err != nil {
		return writeback.VerifiedTurn{}, err
	}
	if isTerminal(record.State) {
		return writeback.VerifiedTurn{}, ErrInvalidState
	}
	if !sameHandle(record, handle) {
		return writeback.VerifiedTurn{}, agentcontext.NewError(
			agentcontext.ErrCodeInvalidTurnHandle,
			"AcceptedTurnHandle 与持久化 Run 不一致",
		)
	}
	return writeback.VerifiedTurn{
		Handle:  handle,
		Profile: record.Profile,
	}, nil
}

// VerifyAuthority 实现 writeback.TurnVerifier。
func (s *Service) VerifyAuthority(
	ctx context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
) error {
	record, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if record.Authority != authority {
		return ErrAuthorityStale
	}
	if record.State != RunStateRunning && record.State != RunStateFinalizing && !isTerminal(record.State) {
		return ErrInvalidState
	}
	return nil
}

func (s *Service) BeginFinalization(
	ctx context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) (agentcontext.FinalizeKey, agentcontext.ActiveExecutionAuthority, error) {
	record, err := s.store.BeginFinalization(ctx, runID, authority, status)
	if err != nil {
		return agentcontext.FinalizeKey{}, agentcontext.ActiveExecutionAuthority{}, err
	}
	return agentcontext.FinalizeKey{
		RunID:    runID,
		Revision: record.Revision,
	}, record.Authority, nil
}

func (s *Service) Complete(
	ctx context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) error {
	_, err := s.store.CompleteRun(ctx, runID, authority, status)
	return err
}

func sameHandle(record RunRecord, handle agentcontext.AcceptedTurnHandle) bool {
	stored := record.Handle
	return stored.RunID == handle.RunID &&
		stored.StepID == handle.StepID &&
		stored.AgentID == handle.AgentID &&
		stored.UserID == handle.UserID &&
		stored.ConversationID == handle.ConversationID &&
		reflect.DeepEqual(stored.CurrentInputRef, handle.CurrentInputRef) &&
		stored.ContextMode == handle.ContextMode &&
		record.InputHash == hashTurnInput(handle.Input)
}

func hashTurnInput(input agentcontext.TurnInput) string {
	var canonical string
	switch value := input.(type) {
	case agentcontext.UserMessageInput:
		canonical = "user:" + value.Content
	case agentcontext.SearchTaskInput:
		canonical = "search:" + value.Task.Query
	default:
		canonical = "unknown"
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

var _ writeback.TurnVerifier = (*Service)(nil)
