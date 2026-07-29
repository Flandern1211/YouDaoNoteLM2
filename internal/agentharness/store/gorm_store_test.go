package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"YoudaoNoteLm/internal/agentharness/core"
)

// getTestDB 获取测试数据库连接
// 需要设置环境变量 TEST_MYSQL_DSN，格式为：user:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
func getTestDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 环境变量未设置，跳过 MySQL 集成测试")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "连接测试数据库失败")

	// 自动迁移表结构
	err = db.AutoMigrate(&AgentRun{}, &AgentRunAttempt{}, &AgentRunStep{})
	require.NoError(t, err, "自动迁移表结构失败")

	return db
}

// cleanupTestDB 清理测试数据
func cleanupTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Exec("DELETE FROM agent_run_steps")
	db.Exec("DELETE FROM agent_run_attempts")
	db.Exec("DELETE FROM agent_runs")
}

func TestGormStore_CreateQueued(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 验证 Run 已创建
	var record AgentRun
	err = db.First(&record, "id = ?", "run-1").Error
	require.NoError(t, err)
	assert.Equal(t, "run-1", record.ID)
	assert.Equal(t, core.RunStateQueued, record.State)
	assert.Equal(t, "chat", record.AgentType)
	assert.Equal(t, uint(1), record.UserID)
}

func TestGormStore_CreateQueued_Duplicate(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 尝试创建重复的 Run
	err = store.CreateQueued(context.Background(), run)
	assert.ErrorIs(t, err, core.ErrRunAlreadyExists)
}

func TestGormStore_Get(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 获取 Run
	fetched, err := store.Get(context.Background(), "run-1")
	require.NoError(t, err)
	assert.Equal(t, core.RunID("run-1"), fetched.ID)
	assert.Equal(t, "chat", fetched.AgentType)
	assert.Equal(t, uint(1), fetched.UserID)
	assert.Equal(t, core.RunStateQueued, fetched.State)
}

func TestGormStore_Get_NotFound(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	_, err := store.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, core.ErrRunNotFound)
}

func TestGormStore_Claim(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// Claim
	claimedRun, attempt, err := store.Claim(context.Background(), "run-1", "worker-1")
	require.NoError(t, err)

	// 验证 Run 状态
	assert.Equal(t, core.RunStateRunning, claimedRun.State)
	assert.Equal(t, core.StateVersion(2), claimedRun.StateVersion)
	require.NotNil(t, claimedRun.Authority)
	assert.Equal(t, core.FencingToken(1), claimedRun.Authority.FencingToken)

	// 验证 Attempt
	assert.Equal(t, core.RunID("run-1"), attempt.RunID)
	assert.Equal(t, "worker-1", attempt.WorkerID)
	assert.Equal(t, core.FencingToken(1), attempt.FencingToken)
	assert.Equal(t, core.AttemptStateRunning, attempt.State)
}

func TestGormStore_Claim_NotQueued(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 第一次 Claim
	_, _, err = store.Claim(context.Background(), "run-1", "worker-1")
	require.NoError(t, err)

	// 第二次 Claim 应该失败
	_, _, err = store.Claim(context.Background(), "run-1", "worker-2")
	assert.ErrorIs(t, err, core.ErrNotQueued)
}

func TestGormStore_Transition(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// Claim
	claimedRun, _, err := store.Claim(context.Background(), "run-1", "worker-1")
	require.NoError(t, err)

	// 转换到 finalizing
	transitionReq := core.TransitionRequest{
		RunID:        "run-1",
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateFinalizing,
		StateVersion: claimedRun.StateVersion,
		FencingToken: claimedRun.Authority.FencingToken,
	}
	transitioned, err := store.Transition(context.Background(), transitionReq)
	require.NoError(t, err)
	assert.Equal(t, core.RunStateFinalizing, transitioned.State)
	assert.Equal(t, core.StateVersion(3), transitioned.StateVersion)
}

func TestGormStore_Transition_AuthorityStale(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// Claim
	_, _, err = store.Claim(context.Background(), "run-1", "worker-1")
	require.NoError(t, err)

	// 使用错误的版本号
	transitionReq := core.TransitionRequest{
		RunID:        "run-1",
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateFinalizing,
		StateVersion: 999, // 错误的版本号
		FencingToken: 1,
	}
	_, err = store.Transition(context.Background(), transitionReq)
	assert.ErrorIs(t, err, core.ErrAuthorityStale)
}

func TestGormStore_CreateStep(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: time.Now().Unix(),
	}

	err := store.CreateStep(context.Background(), step)
	require.NoError(t, err)

	// 验证 Step 已创建
	var record AgentRunStep
	err = db.First(&record, "id = ?", "step-1").Error
	require.NoError(t, err)
	assert.Equal(t, "step-1", record.ID)
	assert.Equal(t, "run-1", record.RunID)
	assert.Equal(t, "attempt-1", record.AttemptID)
	assert.Equal(t, core.StepKindSearch, record.Kind)
	assert.Equal(t, "search-agent", record.AgentName)
	assert.Equal(t, core.StepStateRunning, record.State)
}

func TestGormStore_FinishStep(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: time.Now().Unix(),
	}

	err := store.CreateStep(context.Background(), step)
	require.NoError(t, err)

	// 完成 Step
	finishReq := core.FinishStepRequest{
		StepID:       "step-1",
		RunID:        "run-1",
		AttemptID:    "attempt-1",
		FencingToken: 0,
		State:        core.StepStateCompleted,
	}
	finished, err := store.FinishStep(context.Background(), finishReq)
	require.NoError(t, err)
	assert.Equal(t, core.StepStateCompleted, finished.State)
	assert.NotNil(t, finished.FinishedAt)
}

func TestGormStore_FinishStep_WithArtifact(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: time.Now().Unix(),
	}

	err := store.CreateStep(context.Background(), step)
	require.NoError(t, err)

	// 完成 Step 并设置 artifact
	artifactRef := "artifact-1"
	finishReq := core.FinishStepRequest{
		StepID:            "step-1",
		RunID:             "run-1",
		AttemptID:         "attempt-1",
		FencingToken:      0,
		State:             core.StepStateCompleted,
		ResultArtifactRef: &artifactRef,
	}
	finished, err := store.FinishStep(context.Background(), finishReq)
	require.NoError(t, err)
	assert.Equal(t, core.StepStateCompleted, finished.State)
	require.NotNil(t, finished.ResultArtifactRef)
	assert.Equal(t, "artifact-1", *finished.ResultArtifactRef)
}

func TestGormStore_FinishStep_WithError(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	step := core.Step{
		ID:        "step-1",
		RunID:     "run-1",
		AttemptID: "attempt-1",
		Kind:      core.StepKindSearch,
		AgentName: "search-agent",
		State:     core.StepStateRunning,
		StartedAt: time.Now().Unix(),
	}

	err := store.CreateStep(context.Background(), step)
	require.NoError(t, err)

	// 完成 Step 并设置错误
	errorClass := core.ErrorClassTransient
	errorCode := "TIMEOUT"
	finishReq := core.FinishStepRequest{
		StepID:       "step-1",
		RunID:        "run-1",
		AttemptID:    "attempt-1",
		FencingToken: 0,
		State:        core.StepStateFailed,
		ErrorClass:   &errorClass,
		ErrorCode:    &errorCode,
	}
	finished, err := store.FinishStep(context.Background(), finishReq)
	require.NoError(t, err)
	assert.Equal(t, core.StepStateFailed, finished.State)
	require.NotNil(t, finished.ErrorClass)
	assert.Equal(t, core.ErrorClassTransient, *finished.ErrorClass)
	require.NotNil(t, finished.ErrorCode)
	assert.Equal(t, "TIMEOUT", *finished.ErrorCode)
}

func TestGormStore_FinishStep_NotFound(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	finishReq := core.FinishStepRequest{
		StepID:       "nonexistent",
		RunID:        "run-1",
		AttemptID:    "attempt-1",
		FencingToken: 0,
		State:        core.StepStateCompleted,
	}
	_, err := store.FinishStep(context.Background(), finishReq)
	assert.ErrorIs(t, err, core.ErrStepNotFound)
}

// TestGormStore_ConcurrentClaim 测试并发 Claim 只有一个赢家
func TestGormStore_ConcurrentClaim(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 并发 Claim
	const numWorkers = 5
	results := make(chan error, numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			_, _, err := store.Claim(context.Background(), "run-1", fmt.Sprintf("worker-%d", workerID))
			results <- err
		}(i)
	}

	// 统计结果
	successCount := 0
	for i := 0; i < numWorkers; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else {
			assert.ErrorIs(t, err, core.ErrNotQueued)
		}
	}

	// 只有一个赢家
	assert.Equal(t, 1, successCount, "并发 Claim 应该只有一个赢家")
}

// TestGormStore_OldTokenRejected 测试旧 token 拒绝写入
func TestGormStore_OldTokenRejected(t *testing.T) {
	db := getTestDB(t)
	defer cleanupTestDB(t, db)
	store := NewGormStore(db)

	run := core.Run{
		ID:        "run-1",
		AgentType: "chat",
		UserID:    1,
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  "msg-1",
			Hash: "hash-1",
		},
		State:    core.RunStateQueued,
		StateVersion: 1,
	}

	err := store.CreateQueued(context.Background(), run)
	require.NoError(t, err)

	// 第一次 Claim
	claimedRun, _, err := store.Claim(context.Background(), "run-1", "worker-1")
	require.NoError(t, err)

	// 使用旧的 FencingToken 尝试转换
	transitionReq := core.TransitionRequest{
		RunID:        "run-1",
		CurrentState: core.RunStateRunning,
		TargetState:  core.RunStateFinalizing,
		StateVersion: claimedRun.StateVersion,
		FencingToken: 0, // 旧的 token
	}
	_, err = store.Transition(context.Background(), transitionReq)
	assert.ErrorIs(t, err, core.ErrAuthorityStale)
}
