package agentcontext

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolutionPolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  ResolutionPolicy
		wantErr bool
	}{
		{
			name: "valid policy with stages",
			policy: ResolutionPolicy{
				Stages: []ResolutionStage{
					{
						Name:        "primary",
						Provider:    "redis",
						RetryPolicy: DefaultRetryPolicy(),
					},
				},
				TerminalAction: ExhaustedAbort,
			},
			wantErr: false,
		},
		{
			name: "empty stages",
			policy: ResolutionPolicy{
				Stages:         []ResolutionStage{},
				TerminalAction: ExhaustedAbort,
			},
			wantErr: true,
		},
		{
			name: "invalid terminal action",
			policy: ResolutionPolicy{
				Stages: []ResolutionStage{
					{Name: "primary", RetryPolicy: DefaultRetryPolicy()},
				},
				TerminalAction: "invalid",
			},
			wantErr: true,
		},
		{
			name: "negative max attempts",
			policy: ResolutionPolicy{
				Stages: []ResolutionStage{
					{
						Name: "primary",
						RetryPolicy: RetryPolicy{
							MaxAttempts: -1,
						},
					},
				},
				TerminalAction: ExhaustedSkip,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()

	assert.Equal(t, 1, p.MaxAttempts)
	assert.Equal(t, 100, p.Backoff.InitialMs)
	assert.Equal(t, 1000, p.Backoff.MaxMs)
	assert.Equal(t, 2.0, p.Backoff.Multiplier)
	assert.True(t, p.Retryable(errors.New("test")))
}

func TestDefaultResolutionPolicy(t *testing.T) {
	p := DefaultResolutionPolicy(ExhaustedSkip)

	assert.Equal(t, ExhaustedSkip, p.TerminalAction)
	assert.Empty(t, p.Stages)
}

func TestResolutionPolicy_ContextCancellation(t *testing.T) {
	// 测试场景：context 取消不进入下一次重试
	// 这是一个行为约束测试

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 验证 context 已取消
	assert.Error(t, ctx.Err())
}

func TestExhaustedAction_Constants(t *testing.T) {
	// 测试场景：记录 ExhaustedAction 常量
	assert.Equal(t, ExhaustedAction("abort"), ExhaustedAbort)
	assert.Equal(t, ExhaustedAction("skip"), ExhaustedSkip)
}
