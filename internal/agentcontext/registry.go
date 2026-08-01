package agentcontext

import (
	"fmt"
	"sync"
)

// Registry 不可变的 Profile 和模型能力注册表
type Registry struct {
	mu       sync.RWMutex
	profiles map[ProfileKey]ContextProfile
	models   map[string]ModelCapabilities
}

// NewRegistry 创建不可变注册表
func NewRegistry(profiles []ContextProfile, models map[string]ModelCapabilities) (*Registry, error) {
	r := &Registry{
		profiles: make(map[ProfileKey]ContextProfile),
		models:   make(map[string]ModelCapabilities),
	}

	// 注册 Profiles
	for _, p := range profiles {
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("invalid profile %s.%s: %w", p.Key.Name, p.Key.Version, err)
		}
		key := p.Key
		if _, exists := r.profiles[key]; exists {
			return nil, NewError(ErrCodeDuplicateKey, fmt.Sprintf("duplicate profile: %s.%s", key.Name, key.Version))
		}
		r.profiles[key] = p
	}

	// 注册模型能力
	for id, cap := range models {
		if cap.ContextWindow <= 0 {
			return nil, NewError(ErrCodeInvalidConfig, fmt.Sprintf("invalid context window for model %s", id))
		}
		r.models[id] = cap
	}

	return r, nil
}

// ResolveProfile 按 ProfileKey 获取 Profile
func (r *Registry) ResolveProfile(key ProfileKey) (ContextProfile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[key]
	return p, ok
}

// ResolveModel 按模型标识获取能力
func (r *Registry) ResolveModel(ref ModelRef) (ModelCapabilities, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := modelKey(ref)
	cap, ok := r.models[id]
	return cap, ok
}

// Profiles 返回所有注册的 Profile（只读）
func (r *Registry) Profiles() []ContextProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ContextProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		result = append(result, p)
	}
	return result
}

func modelKey(ref ModelRef) string {
	return ref.Provider + "/" + ref.ModelID
}
