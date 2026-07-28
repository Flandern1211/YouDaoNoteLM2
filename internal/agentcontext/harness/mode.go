package harness

import (
	"YoudaoNoteLm/internal/agentcontext"
	"YoudaoNoteLm/pkg/config"
)

const ContextContractVersion = "v1"

// ModeResolver 为新 Run 生成不可变的上下文模式和写回所有者快照。
type ModeResolver struct {
	config   config.ContextManagementConfig
	selector *agentcontext.ShadowBucketSelector
}

func NewModeResolver(cfg config.ContextManagementConfig) *ModeResolver {
	return &ModeResolver{
		config:   cfg,
		selector: agentcontext.NewShadowBucketSelector(),
	}
}

func (r *ModeResolver) Resolve(userID uint) agentcontext.ContextModeSnapshot {
	mode := r.config.GetMode()
	effectiveMode := mode

	switch mode {
	case "shadow":
		if !r.selector.IsSelected(userID, r.config.RolloutVersion, r.config.ShadowSampleBasisPoints) {
			effectiveMode = "legacy"
		}
	case "enabled":
		if r.config.ShadowSampleBasisPoints > 0 &&
			!r.selector.IsSelected(userID, r.config.RolloutVersion, r.config.ShadowSampleBasisPoints) {
			effectiveMode = "legacy"
		}
	}

	owner := "legacy"
	if effectiveMode == "enabled" && r.config.WritebackEnabled {
		owner = "context"
	}

	return agentcontext.ContextModeSnapshot{
		Mode:            effectiveMode,
		WritebackOwner:  owner,
		ContractVersion: ContextContractVersion,
	}
}
