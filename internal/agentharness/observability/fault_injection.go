package observability

import (
	"math/rand"
	"time"
)

// FaultType 故障类型。
type FaultType string

const (
	FaultNone          FaultType = "none"
	FaultPanic         FaultType = "panic"
	FaultTimeout       FaultType = "timeout"
	FaultTransient     FaultType = "transient"
	FaultPermanent     FaultType = "permanent"
	FaultNetworkDelay  FaultType = "network_delay"
)

// FaultConfig 故障注入配置。
type FaultConfig struct {
	Enabled       bool
	FaultType     FaultType
	Probability   float64 // 0.0 - 1.0
	DelayMs       int     // 用于 network_delay
}

// FaultInjector 故障注入器。
type FaultInjector struct {
	config FaultConfig
}

// NewFaultInjector 创建 FaultInjector。
func NewFaultInjector(config FaultConfig) *FaultInjector {
	return &FaultInjector{config: config}
}

// ShouldInject 检查是否应该注入故障。
func (f *FaultInjector) ShouldInject() bool {
	if !f.config.Enabled {
		return false
	}
	return rand.Float64() < f.config.Probability
}

// GetFaultType 获取故障类型。
func (f *FaultInjector) GetFaultType() FaultType {
	if !f.ShouldInject() {
		return FaultNone
	}
	return f.config.FaultType
}

// GetDelay 获取延迟时间（毫秒）。
func (f *FaultInjector) GetDelay() int {
	if f.config.FaultType == FaultNetworkDelay {
		return f.config.DelayMs
	}
	return 0
}

// SLO SLO 定义。
type SLO struct {
	Name        string
	Target      float64 // 目标百分比 (0.0 - 1.0)
	Window      time.Duration
}

// SLOTracker SLO 追踪器。
type SLOTracker struct {
	slos    []SLO
	metrics map[string]*SLOMetrics
}

// SLOMetrics SLO 指标。
type SLOMetrics struct {
	Total   int
	Success int
}

// NewSLOTracker 创建 SLOTracker。
func NewSLOTracker(slos []SLO) *SLOTracker {
	metrics := make(map[string]*SLOMetrics)
	for _, slo := range slos {
		metrics[slo.Name] = &SLOMetrics{}
	}
	return &SLOTracker{slos: slos, metrics: metrics}
}

// Record 记录一次结果。
func (t *SLOTracker) Record(name string, success bool) {
	m, exists := t.metrics[name]
	if !exists {
		return
	}
	m.Total++
	if success {
		m.Success++
	}
}

// GetCompliance 获取 SLO 合规率。
func (t *SLOTracker) GetCompliance(name string) float64 {
	m, exists := t.metrics[name]
	if !exists || m.Total == 0 {
		return 0
	}
	return float64(m.Success) / float64(m.Total)
}

// IsCompliant 检查是否满足 SLO。
func (t *SLOTracker) IsCompliant(name string) bool {
	for _, slo := range t.slos {
		if slo.Name == name {
			return t.GetCompliance(name) >= slo.Target
		}
	}
	return false
}
