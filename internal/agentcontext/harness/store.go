package harness

import (
	"context"
	"errors"
	"sync"

	"YoudaoNoteLm/internal/agentcontext"
)

var (
	ErrRunNotFound      = errors.New("context run not found")
	ErrRunAlreadyExists = errors.New("context run already exists")
	ErrAuthorityStale   = errors.New("context run authority is stale")
	ErrInvalidState     = errors.New("context run state transition is invalid")
)

type RunState string

const (
	RunStateRunning    RunState = "running"
	RunStateFinalizing RunState = "finalizing"
	RunStateCompleted  RunState = "completed"
	RunStateFailed     RunState = "failed"
	RunStateCancelled  RunState = "cancelled"
)

// RunRecord 是最小 Harness 的持久化事实。
// InputHash 只用于校验，Run/Manifest 表均不复制用户正文。
type RunRecord struct {
	Handle      agentcontext.AcceptedTurnHandle
	Profile     agentcontext.ContextProfileSnapshot
	InputHash   string
	State       RunState
	Authority   agentcontext.ActiveExecutionAuthority
	Revision    uint64
	FinalStatus agentcontext.TurnStatus
}

// Store 抽象最小 Harness 所需的原子持久化操作。
type Store interface {
	CreateRun(ctx context.Context, record RunRecord) error
	GetRun(ctx context.Context, runID string) (RunRecord, error)
	BeginFinalization(
		ctx context.Context,
		runID string,
		authority agentcontext.ActiveExecutionAuthority,
		status agentcontext.TurnStatus,
	) (RunRecord, error)
	CompleteRun(
		ctx context.Context,
		runID string,
		authority agentcontext.ActiveExecutionAuthority,
		status agentcontext.TurnStatus,
	) (RunRecord, error)
}

// MemoryStore 是测试和本地最小部署使用的单进程 Store。
type MemoryStore struct {
	mu   sync.Mutex
	runs map[string]RunRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{runs: make(map[string]RunRecord)}
}

func (s *MemoryStore) CreateRun(_ context.Context, record RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.runs[record.Handle.RunID]; exists {
		return ErrRunAlreadyExists
	}
	s.runs[record.Handle.RunID] = record
	return nil
}

func (s *MemoryStore) GetRun(_ context.Context, runID string) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.runs[runID]
	if !exists {
		return RunRecord{}, ErrRunNotFound
	}
	return record, nil
}

func (s *MemoryStore) BeginFinalization(
	_ context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.runs[runID]
	if !exists {
		return RunRecord{}, ErrRunNotFound
	}
	if record.State == RunStateFinalizing || isTerminal(record.State) {
		if sameExecution(record.Authority, authority) && record.FinalStatus == status {
			return record, nil
		}
		return RunRecord{}, ErrInvalidState
	}
	if record.State != RunStateRunning {
		return RunRecord{}, ErrInvalidState
	}
	if record.Authority != authority {
		return RunRecord{}, ErrAuthorityStale
	}

	record.State = RunStateFinalizing
	record.Revision++
	record.FinalStatus = status
	record.Authority.RunStateVersion++
	s.runs[runID] = record
	return record, nil
}

func (s *MemoryStore) CompleteRun(
	_ context.Context,
	runID string,
	authority agentcontext.ActiveExecutionAuthority,
	status agentcontext.TurnStatus,
) (RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, exists := s.runs[runID]
	if !exists {
		return RunRecord{}, ErrRunNotFound
	}
	if isTerminal(record.State) {
		if sameExecution(record.Authority, authority) && record.FinalStatus == status {
			return record, nil
		}
		return RunRecord{}, ErrInvalidState
	}
	if record.State != RunStateFinalizing || record.Authority != authority || record.FinalStatus != status {
		return RunRecord{}, ErrAuthorityStale
	}

	record.State = terminalState(status)
	record.Authority.RunStateVersion++
	s.runs[runID] = record
	return record, nil
}

func sameExecution(current, candidate agentcontext.ActiveExecutionAuthority) bool {
	return current.AttemptID == candidate.AttemptID &&
		current.FencingToken == candidate.FencingToken
}

func isTerminal(state RunState) bool {
	return state == RunStateCompleted || state == RunStateFailed || state == RunStateCancelled
}

func terminalState(status agentcontext.TurnStatus) RunState {
	switch status {
	case agentcontext.TurnStatusSuccess:
		return RunStateCompleted
	case agentcontext.TurnStatusCancelled:
		return RunStateCancelled
	default:
		return RunStateFailed
	}
}
