package agentcontext

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// FingerprintSchemaVersion 指纹 schema 版本
const FingerprintSchemaVersion = "v1"

// ContextFingerprinter 上下文指纹生成器。
// 使用带密钥的 HMAC 生成指纹，不直接保存完整 Prompt 的 SHA-256。
type ContextFingerprinter struct {
	salt string
}

// NewContextFingerprinter 创建上下文指纹生成器。
func NewContextFingerprinter(salt string) *ContextFingerprinter {
	return &ContextFingerprinter{salt: salt}
}

// FingerprintInput 指纹输入
type FingerprintInput struct {
	ProfileID      string
	ProfileVersion string
	PromptVersion  string
	ToolsetVersion string
	Model          string
	Mode           string
	// MessageRoleOrder 消息角色顺序（不包含内容）
	MessageRoleOrder []string
	// ToolNames 工具名称列表（排序后）
	ToolNames []string
	// CounterMode 计数模式
	CounterMode string
}

// Generate 生成上下文指纹。
// 使用 HMAC-SHA256(盐, 规范化输入) 生成指纹。
func (f *ContextFingerprinter) Generate(input FingerprintInput) string {
	canonical := f.canonicalize(input)
	mac := hmac.New(sha256.New, []byte(f.salt))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// canonicalize 规范化输入。
// 对顺序敏感字段保持顺序，对 map key 规范排序。
func (f *ContextFingerprinter) canonicalize(input FingerprintInput) string {
	var b strings.Builder

	// 指纹 schema 版本
	b.WriteString(FingerprintSchemaVersion)
	b.WriteString("|")

	// Profile
	b.WriteString(input.ProfileID)
	b.WriteString(":")
	b.WriteString(input.ProfileVersion)
	b.WriteString("|")

	// Prompt
	b.WriteString(input.PromptVersion)
	b.WriteString("|")

	// Toolset
	b.WriteString(input.ToolsetVersion)
	b.WriteString("|")

	// Model
	b.WriteString(input.Model)
	b.WriteString("|")

	// Mode
	b.WriteString(input.Mode)
	b.WriteString("|")

	// Message role order（保持顺序）
	b.WriteString(strings.Join(input.MessageRoleOrder, ","))
	b.WriteString("|")

	// Tool names（排序后）
	sortedTools := make([]string, len(input.ToolNames))
	copy(sortedTools, input.ToolNames)
	sort.Strings(sortedTools)
	b.WriteString(strings.Join(sortedTools, ","))
	b.WriteString("|")

	// Counter mode
	b.WriteString(input.CounterMode)

	return b.String()
}

// ManifestFingerprint Manifest 指纹。
// 用于判断装配结果是否相同，不用于恢复正文。
type ManifestFingerprint struct {
	SchemaVersion string
	Fingerprint   string
}

// GenerateManifestFingerprint 生成 Manifest 指纹。
func (f *ContextFingerprinter) GenerateManifestFingerprint(manifest ContextManifest) ManifestFingerprint {
	input := FingerprintInput{
		ProfileID:      manifest.ProfileID,
		ProfileVersion: manifest.ProfileVersion,
		PromptVersion:  manifest.PromptVersion,
		ToolsetVersion: manifest.ToolsetVersion,
		Model:          manifest.Model,
		Mode:           manifest.TurnStatus,
		CounterMode:    manifest.CounterMode,
	}

	// Source 名称排序
	var sourceNames []string
	for _, s := range manifest.Sources {
		sourceNames = append(sourceNames, fmt.Sprintf("%s:%d:%d", s.Provider, s.SelectedCount, s.TokenCount))
	}
	sort.Strings(sourceNames)
	input.ToolNames = sourceNames // 复用 ToolNames 字段存储排序后的 source 信息

	return ManifestFingerprint{
		SchemaVersion: FingerprintSchemaVersion,
		Fingerprint:   f.Generate(input),
	}
}
