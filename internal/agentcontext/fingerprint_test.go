package agentcontext

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

func TestContextFingerprinter_SameInput(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	input := FingerprintInput{
		ProfileID:        "chat",
		ProfileVersion:   "v1",
		PromptVersion:    "v1",
		ToolsetVersion:   "tools-3",
		Model:            "openai/gpt-4o",
		Mode:             "prepared",
		MessageRoleOrder: []string{"system", "user", "assistant"},
		ToolNames:        []string{"search_knowledge", "get_sources_summary"},
		CounterMode:      "conservative_utf8_bytes",
	}

	fp1 := f.Generate(input)
	fp2 := f.Generate(input)
	assert.Equal(t, fp1, fp2, "相同输入应产生相同指纹")
}

func TestContextFingerprinter_DifferentInput(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	input1 := FingerprintInput{
		ProfileID:      "chat",
		ProfileVersion: "v1",
		Model:          "openai/gpt-4o",
	}
	input2 := FingerprintInput{
		ProfileID:      "chat",
		ProfileVersion: "v1",
		Model:          "openai/gpt-4o-mini",
	}

	fp1 := f.Generate(input1)
	fp2 := f.Generate(input2)
	assert.NotEqual(t, fp1, fp2, "不同输入应产生不同指纹")
}

func TestContextFingerprinter_DifferentSalt(t *testing.T) {
	f1 := NewContextFingerprinter("salt-1")
	f2 := NewContextFingerprinter("salt-2")

	input := FingerprintInput{
		ProfileID:      "chat",
		ProfileVersion: "v1",
	}

	fp1 := f1.Generate(input)
	fp2 := f2.Generate(input)
	assert.NotEqual(t, fp1, fp2, "不同盐应产生不同指纹")
}

func TestContextFingerprinter_ToolOrderInsensitive(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	input1 := FingerprintInput{
		ToolNames: []string{"b_tool", "a_tool"},
	}
	input2 := FingerprintInput{
		ToolNames: []string{"a_tool", "b_tool"},
	}

	fp1 := f.Generate(input1)
	fp2 := f.Generate(input2)
	assert.Equal(t, fp1, fp2, "工具顺序不应影响指纹（已排序）")
}

func TestContextFingerprinter_MessageRoleOrderSensitive(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	input1 := FingerprintInput{
		MessageRoleOrder: []string{"system", "user", "assistant"},
	}
	input2 := FingerprintInput{
		MessageRoleOrder: []string{"user", "system", "assistant"},
	}

	fp1 := f.Generate(input1)
	fp2 := f.Generate(input2)
	assert.NotEqual(t, fp1, fp2, "消息角色顺序应影响指纹")
}

func TestGenerateManifestFingerprint(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	manifest1 := ContextManifest{
		ProfileID:      "chat",
		ProfileVersion: "v1",
		PromptVersion:  "v1",
		ToolsetVersion: "tools-3",
		Model:          "openai/gpt-4o",
		TurnStatus:     "prepared",
		CounterMode:    "conservative_utf8_bytes",
		Sources: []SourceManifest{
			{Provider: "prompt", SelectedCount: 1, TokenCount: 100},
			{Provider: "history", SelectedCount: 5, TokenCount: 500},
		},
	}

	manifest2 := ContextManifest{
		ProfileID:      "chat",
		ProfileVersion: "v1",
		PromptVersion:  "v1",
		ToolsetVersion: "tools-3",
		Model:          "openai/gpt-4o",
		TurnStatus:     "prepared",
		CounterMode:    "conservative_utf8_bytes",
		Sources: []SourceManifest{
			{Provider: "history", SelectedCount: 5, TokenCount: 500},
			{Provider: "prompt", SelectedCount: 1, TokenCount: 100},
		},
	}

	fp1 := f.GenerateManifestFingerprint(manifest1)
	fp2 := f.GenerateManifestFingerprint(manifest2)
	assert.Equal(t, fp1.Fingerprint, fp2.Fingerprint, "Source 顺序不应影响指纹（已排序）")
	assert.Equal(t, FingerprintSchemaVersion, fp1.SchemaVersion)
}

func TestGenerateManifestFingerprint_DifferentManifests(t *testing.T) {
	f := NewContextFingerprinter("test-salt")

	manifest1 := ContextManifest{
		ProfileID:  "chat",
		TurnStatus: "prepared",
	}
	manifest2 := ContextManifest{
		ProfileID:  "search",
		TurnStatus: "prepared",
	}

	fp1 := f.GenerateManifestFingerprint(manifest1)
	fp2 := f.GenerateManifestFingerprint(manifest2)
	assert.NotEqual(t, fp1.Fingerprint, fp2.Fingerprint, "不同 Manifest 应产生不同指纹")
}

func TestGenerateCompiledFingerprint_ContentSensitiveWithoutPlaintext(t *testing.T) {
	f := NewContextFingerprinter("test-salt")
	manifest := ContextManifest{
		ProfileID:      "chat",
		ProfileVersion: "v1",
	}

	fp1 := f.GenerateCompiledFingerprint(
		manifest,
		[]*schema.Message{schema.UserMessage("first secret")},
		nil,
	)
	fp2 := f.GenerateCompiledFingerprint(
		manifest,
		[]*schema.Message{schema.UserMessage("second secret")},
		nil,
	)

	assert.NotEqual(t, fp1.Fingerprint, fp2.Fingerprint)
	assert.NotContains(t, fp1.Fingerprint, "first secret")
	assert.Len(t, fp1.Fingerprint, 64)
}
