package harness

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"YoudaoNoteLm/internal/agentcontext"
	agentcontextEino "YoudaoNoteLm/internal/agentcontext/eino"
	"YoudaoNoteLm/internal/agentcontext/writeback"
	"YoudaoNoteLm/pkg/config"
)

const defaultFinalizationTimeout = 10 * time.Second

type RuntimeConfig struct {
	ContextConfig       config.ContextManagementConfig
	Registry            *agentcontext.Registry
	Compiler            agentcontext.ContextCompiler
	Store               Store
	Writers             writeback.WriterRegistry
	FinalizationTimeout time.Duration
}

// Runtime 是最小 Harness 与 ContextCompiler 的请求级门面。
type Runtime struct {
	modeResolver        *ModeResolver
	registry            *agentcontext.Registry
	compiler            agentcontext.ContextCompiler
	service             *Service
	coordinator         *writeback.TurnLifecycleCoordinator
	middleware          adk.ChatModelAgentMiddleware
	finalizationTimeout time.Duration

	recordsMu sync.Mutex
	records   map[string][]agentcontext.CompileRecord
}

type BeginChatRequest struct {
	UserID          uint
	ConversationID  uint
	Content         string
	CurrentInputRef *agentcontext.MessageRef
	Model           agentcontext.ModelRef
}

type Execution struct {
	PreparedTurn *agentcontext.PreparedTurn
	Authority    agentcontext.ActiveExecutionAuthority
	Mode         agentcontext.ContextModeSnapshot
}

type FinalizeChatRequest struct {
	Status   agentcontext.TurnStatus
	Content  string
	Metadata []byte
	Messages []*schema.Message
}

func NewRuntime(cfg RuntimeConfig) (*Runtime, error) {
	if cfg.Registry == nil {
		return nil, fmt.Errorf("Context Registry 未配置")
	}
	if cfg.Compiler == nil {
		return nil, fmt.Errorf("ContextCompiler 未配置")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("Harness Store 未配置")
	}
	if cfg.Writers.Manifest == nil {
		return nil, fmt.Errorf("ManifestWriter 未配置")
	}
	timeout := cfg.FinalizationTimeout
	if timeout <= 0 {
		timeout = defaultFinalizationTimeout
	}

	runtime := &Runtime{
		modeResolver:        NewModeResolver(cfg.ContextConfig),
		registry:            cfg.Registry,
		compiler:            cfg.Compiler,
		service:             NewService(cfg.Store),
		finalizationTimeout: timeout,
		records:             make(map[string][]agentcontext.CompileRecord),
	}
	runtime.coordinator = writeback.NewTurnLifecycleCoordinator(writeback.CoordinatorConfig{
		Verifier: runtime.service,
		Writers:  cfg.Writers,
	})
	runtime.middleware = agentcontextEino.NewContextMiddleware(agentcontextEino.ContextMiddlewareConfig{
		Compiler:          runtime.compiler,
		Mode:              agentcontextEino.ContextModeLegacy,
		CompileRecordSink: runtime.recordCompile,
	})
	return runtime, nil
}

func (r *Runtime) Middleware() adk.ChatModelAgentMiddleware {
	return r.middleware
}

func (r *Runtime) BeginChat(
	ctx context.Context,
	req BeginChatRequest,
) (context.Context, *Execution, error) {
	mode := r.modeResolver.Resolve(req.UserID)
	execution := &Execution{Mode: mode}
	if mode.Mode == "legacy" {
		return ctx, execution, nil
	}

	profile, ok := r.registry.ResolveProfile(agentcontext.ChatV1)
	if !ok {
		return ctx, nil, fmt.Errorf("chat.v1 Profile 未注册")
	}
	handle, authority, err := r.service.AcceptTurn(ctx, agentcontext.AcceptedTurnHandle{
		AgentID:         agentcontext.AgentIDChat,
		UserID:          req.UserID,
		ConversationID:  req.ConversationID,
		Input:           agentcontext.UserMessageInput{Content: req.Content},
		CurrentInputRef: req.CurrentInputRef,
		ContextMode:     mode,
	}, profile.ToSnapshot())
	if err != nil {
		return ctx, nil, fmt.Errorf("接受 Context Run 失败: %w", err)
	}

	session, err := r.coordinator.BeginTurn(ctx, agentcontext.BeginTurnRequest{
		Handle:    handle,
		Authority: authority,
	})
	if err != nil {
		return ctx, nil, err
	}
	prepared, err := r.compiler.PrepareTurn(ctx, session, agentcontext.PrepareTurnRequest{Model: req.Model})
	if err != nil {
		r.finishBeginFailure(ctx, session, authority)
		return ctx, nil, err
	}

	execution.PreparedTurn = prepared
	execution.Authority = authority
	return agentcontextEino.WithPreparedTurn(ctx, prepared), execution, nil
}

func (r *Runtime) FinalizeChat(
	ctx context.Context,
	execution *Execution,
	req FinalizeChatRequest,
) (*agentcontext.FinalizeResult, error) {
	if execution == nil || execution.Mode.Mode == "legacy" || execution.PreparedTurn == nil {
		return &agentcontext.FinalizeResult{}, nil
	}

	status := req.Status
	if status == agentcontext.TurnStatusSuccess && req.Content == "" {
		status = agentcontext.TurnStatusFailed
	}
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.finalizationTimeout)
	defer cancel()

	runID := execution.PreparedTurn.Session.Handle.RunID
	key, authority, err := r.service.BeginFinalization(finalizeCtx, runID, execution.Authority, status)
	if err != nil {
		return nil, err
	}

	outcome := agentcontext.TurnOutcome{
		Status:   status,
		Messages: req.Messages,
	}
	if status == agentcontext.TurnStatusSuccess {
		outcome.PrimaryOutput = agentcontext.ConversationOutput{
			FinalMessage: schema.AssistantMessage(req.Content, nil),
			Metadata:     req.Metadata,
		}
	}
	result, err := r.coordinator.FinalizeTurn(finalizeCtx, agentcontext.FinalizeRequest{
		Turn:           execution.PreparedTurn,
		Outcome:        outcome,
		CompileRecords: r.compileRecords(runID),
		FinalizeKey:    key,
		Authority:      authority,
	})
	if err != nil {
		return result, err
	}
	if err := r.service.Complete(finalizeCtx, runID, authority, status); err != nil {
		return result, err
	}
	r.deleteCompileRecords(runID)
	return result, nil
}

func (r *Runtime) UsesContextWriteback(execution *Execution) bool {
	return execution != nil && execution.Mode.WritebackOwner == "context"
}

func (r *Runtime) recordCompile(runID string, record agentcontext.CompileRecord) {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	record.ModelCallID = fmt.Sprintf("%s-call-%d", runID, len(r.records[runID])+1)
	r.records[runID] = append(r.records[runID], record)
}

func (r *Runtime) compileRecords(runID string) []agentcontext.CompileRecord {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	return append([]agentcontext.CompileRecord(nil), r.records[runID]...)
}

func (r *Runtime) deleteCompileRecords(runID string) {
	r.recordsMu.Lock()
	defer r.recordsMu.Unlock()
	delete(r.records, runID)
}

func (r *Runtime) finishBeginFailure(
	ctx context.Context,
	session *agentcontext.TurnSession,
	authority agentcontext.ActiveExecutionAuthority,
) {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.finalizationTimeout)
	defer cancel()
	key, finalizingAuthority, err := r.service.BeginFinalization(
		finalizeCtx,
		session.Handle.RunID,
		authority,
		agentcontext.TurnStatusFailed,
	)
	if err != nil {
		return
	}
	turn := &agentcontext.PreparedTurn{
		Session: session,
		Profile: session.Profile,
		BaseManifest: agentcontext.ContextManifest{
			ProfileID:      session.Profile.Key.Name,
			ProfileVersion: session.Profile.Key.Version,
			TurnStatus:     string(agentcontext.TurnStatusFailed),
			Degraded:       true,
		},
	}
	_, _ = r.coordinator.FinalizeTurn(finalizeCtx, agentcontext.FinalizeRequest{
		Turn:        turn,
		Outcome:     agentcontext.TurnOutcome{Status: agentcontext.TurnStatusFailed},
		FinalizeKey: key,
		Authority:   finalizingAuthority,
	})
	_ = r.service.Complete(
		finalizeCtx,
		session.Handle.RunID,
		finalizingAuthority,
		agentcontext.TurnStatusFailed,
	)
}
