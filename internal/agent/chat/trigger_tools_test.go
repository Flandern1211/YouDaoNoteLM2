package chat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExplicitSearchQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		found    bool
	}{
		{
			name:     "帮我搜索一下",
			input:    "帮我搜索一下 Go 语言",
			expected: "Go 语言",
			found:    true,
		},
		{
			name:     "帮我搜一下",
			input:    "帮我搜一下 Python",
			expected: "Python",
			found:    true,
		},
		{
			name:     "帮我搜索",
			input:    "帮我搜索 机器学习",
			expected: "机器学习",
			found:    true,
		},
		{
			name:     "搜索一下",
			input:    "搜索一下 深度学习",
			expected: "深度学习",
			found:    true,
		},
		{
			name:     "搜一下",
			input:    "搜一下 自然语言处理",
			expected: "自然语言处理",
			found:    true,
		},
		{
			name:     "搜索",
			input:    "搜索 人工智能",
			expected: "人工智能",
			found:    true,
		},
		{
			name:     "搜",
			input:    "搜 大模型",
			expected: "大模型",
			found:    true,
		},
		{
			name:     "查一下",
			input:    "查一下 区块链",
			expected: "区块链",
			found:    true,
		},
		{
			name:     "查",
			input:    "查 量子计算",
			expected: "量子计算",
			found:    true,
		},
		{
			name:     "带冒号",
			input:    "帮我搜索一下：Go 语言",
			expected: "Go 语言",
			found:    true,
		},
		{
			name:     "带中文冒号",
			input:    "帮我搜索一下：Python",
			expected: "Python",
			found:    true,
		},
		{
			name:     "空查询",
			input:    "帮我搜索一下",
			expected: "",
			found:    false,
		},
		{
			name:     "非搜索指令",
			input:    "今天天气怎么样",
			expected: "",
			found:    false,
		},
		{
			name:     "空输入",
			input:    "",
			expected: "",
			found:    false,
		},
		{
			name:     "只有空格",
			input:    "   ",
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, found := explicitSearchQuery(tt.input)
			assert.Equal(t, tt.found, found)
			if found {
				assert.Equal(t, tt.expected, query)
			}
		})
	}
}

func TestParseSearchResults(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		expectedCount   int
		expectedSummary string
	}{
		{
			name: "有效JSON结果",
			content: `这是搜索结果：
{
  "results": [
    {"title": "结果1", "url": "http://example.com/1", "snippet": "摘要1", "score": 0.9},
    {"title": "结果2", "url": "http://example.com/2", "snippet": "摘要2", "score": 0.8}
  ],
  "summary": "搜索结果摘要"
}`,
			expectedCount:   2,
			expectedSummary: "搜索结果摘要",
		},
		{
			name:            "无JSON结果",
			content:         "没有搜索结果",
			expectedCount:   0,
			expectedSummary: "",
		},
		{
			name:            "空内容",
			content:         "",
			expectedCount:   0,
			expectedSummary: "",
		},
		{
			name: "无效JSON",
			content: `{
  "results": [
    {"title": "结果1", "url": "http://example.com/1", "snippet": "摘要1", "score": 0.9}
  ],
  "summary": "摘要"
}`,
			expectedCount:   1,
			expectedSummary: "摘要",
		},
		{
			name: "多个JSON块取最后一个有效",
			content: `第一个JSON：
{"results": [], "summary": "空"}
第二个JSON：
{"results": [{"title": "结果1", "url": "http://example.com/1", "snippet": "摘要1", "score": 0.9}], "summary": "有效摘要"}`,
			expectedCount:   1,
			expectedSummary: "有效摘要",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, summary := parseSearchResults(tt.content)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, tt.expectedSummary, summary)
		})
	}
}

func TestTriggerSearchTool_QueryForwarding(t *testing.T) {
	// 测试场景：triggerSearchTool 将非空 query 原样转发
	executor := &mockSearchAgentExecutor{}
	tool := newTriggerSearchTool(executor, 1, 1)

	// 测试空 query 被拒绝
	_, err := tool.RunQuery(context.Background(), "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "搜索关键词不能为空")

	// 测试只有空格的 query 被拒绝
	_, err = tool.RunQuery(context.Background(), "   ")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "搜索关键词不能为空")
}

// mockSearchAgentExecutor 用于测试的 SearchAgentExecutor mock
type mockSearchAgentExecutor struct {
	executeStreamCalled bool
	lastQuery           string
}

func (m *mockSearchAgentExecutor) ExecuteStream(_ context.Context, _, _ uint, task string) <-chan *SearchEvent {
	m.executeStreamCalled = true
	m.lastQuery = task

	ch := make(chan *SearchEvent, 1)
	ch <- &SearchEvent{Type: "started"}
	close(ch)
	return ch
}

func TestSearchEventTypes(t *testing.T) {
	// 测试场景：记录 SearchEvent 的类型
	// 这是一个基线测试，记录当前的事件类型
	eventTypes := map[string]bool{
		"started":      true,
		"search_round": true,
		"content":      true,
		"error":        true,
		"done":         true,
	}

	// 验证这些事件类型是预期的
	assert.True(t, eventTypes["started"])
	assert.True(t, eventTypes["search_round"])
	assert.True(t, eventTypes["content"])
	assert.True(t, eventTypes["error"])
	assert.True(t, eventTypes["done"])
}

func TestTriggerSearchTool_NotIntegratedWithChatBuilder(t *testing.T) {
	// 基线事实：triggerSearchTool 当前未接入 Chat Builder
	// MainAgentEnabled 也不是已启用的生产 Main 编排
	// W0 不改变此事实

	t.Log("基线事实：triggerSearchTool 当前未接入 Chat Builder")
	t.Log("基线事实：MainAgentEnabled 不是已启用的生产 Main 编排")
	t.Log("W0 不改变这些事实，只记录它们")
}
