package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"YoudaoNoteLm/pkg/config"
)

func TestModeResolver(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.ContextManagementConfig
		userID    uint
		wantMode  string
		wantOwner string
	}{
		{
			name:      "legacy",
			cfg:       config.ContextManagementConfig{Mode: "legacy"},
			userID:    42,
			wantMode:  "legacy",
			wantOwner: "legacy",
		},
		{
			name: "sampled shadow",
			cfg: config.ContextManagementConfig{
				Mode:                    "shadow",
				RolloutVersion:          "v1",
				ShadowSampleBasisPoints: 1019,
			},
			userID:    42,
			wantMode:  "shadow",
			wantOwner: "legacy",
		},
		{
			name: "unsampled shadow",
			cfg: config.ContextManagementConfig{
				Mode:                    "shadow",
				RolloutVersion:          "v1",
				ShadowSampleBasisPoints: 1018,
			},
			userID:    42,
			wantMode:  "legacy",
			wantOwner: "legacy",
		},
		{
			name: "enabled all with context writeback",
			cfg: config.ContextManagementConfig{
				Mode:             "enabled",
				WritebackEnabled: true,
			},
			userID:    42,
			wantMode:  "enabled",
			wantOwner: "context",
		},
		{
			name: "enabled compiler with legacy writeback fallback",
			cfg: config.ContextManagementConfig{
				Mode:             "enabled",
				WritebackEnabled: false,
			},
			userID:    42,
			wantMode:  "enabled",
			wantOwner: "legacy",
		},
		{
			name: "unsampled enabled rollout",
			cfg: config.ContextManagementConfig{
				Mode:                    "enabled",
				WritebackEnabled:        true,
				RolloutVersion:          "v1",
				ShadowSampleBasisPoints: 1018,
			},
			userID:    42,
			wantMode:  "legacy",
			wantOwner: "legacy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := NewModeResolver(tt.cfg).Resolve(tt.userID)
			assert.Equal(t, tt.wantMode, snapshot.Mode)
			assert.Equal(t, tt.wantOwner, snapshot.WritebackOwner)
			assert.Equal(t, ContextContractVersion, snapshot.ContractVersion)
		})
	}
}
