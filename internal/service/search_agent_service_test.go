package service

import (
	"context"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"YoudaoNoteLm/internal/model/dto/response"
	"YoudaoNoteLm/internal/model/entity"
)

// mockSearchAgent 用于测试的 SearchAgentInterface mock
type mockSearchAgent struct {
	executeCalled       bool
	executeStreamCalled bool
	lastQuery           string
	lastUserID          uint
	lastNotebookID      uint
}

func (m *mockSearchAgent) Execute(_ context.Context, userID, notebookID uint, query string) (*SearchAgentResult, error) {
	m.executeCalled = true
	m.lastQuery = query
	m.lastUserID = userID
	m.lastNotebookID = notebookID

	return &SearchAgentResult{
		Content:      `{"results": [{"title": "测试结果", "url": "http://example.com", "snippet": "摘要", "score": 0.9}], "summary": "测试摘要"}`,
		SearchRounds: 1,
	}, nil
}

func (m *mockSearchAgent) ExecuteStream(_ context.Context, userID, notebookID uint, query string) <-chan *SearchAgentEvent {
	m.executeStreamCalled = true
	m.lastQuery = query
	m.lastUserID = userID
	m.lastNotebookID = notebookID

	ch := make(chan *SearchAgentEvent, 2)
	ch <- &SearchAgentEvent{Type: "content", Content: "测试内容"}
	ch <- &SearchAgentEvent{Type: "done", SearchRounds: 1}
	close(ch)
	return ch
}

// mockImporterService 用于测试的 ImporterService mock
type mockImporterService struct{}

func (m *mockImporterService) ImportSearchResults(_ uint, _ uint, _ []SearchResultItem) (string, []uint, error) {
	return "task-1", []uint{1}, nil
}

func (m *mockImporterService) ImportFromURL(_ uint, _ uint, _ string) (string, uint, error) {
	return "task-1", 1, nil
}

func (m *mockImporterService) ImportFile(_ uint, _ uint, _ *multipart.FileHeader) (*entity.Source, error) {
	return nil, nil
}

func (m *mockImporterService) PreviewAudio(_ uint, _ uint, _ *multipart.FileHeader) (string, string, error) {
	return "", "", nil
}

func (m *mockImporterService) ConfirmAudio(_ uint, _ string, _ *string) (*entity.Source, error) {
	return nil, nil
}

func (m *mockImporterService) GetAudioPreviewStatus(_ uint, _ string) (interface{}, error) {
	return nil, nil
}

func (m *mockImporterService) GetImportTask(_ string) (interface{}, error) {
	return nil, nil
}

func (m *mockImporterService) DeleteImportTask(_ string) error {
	return nil
}

func TestSearchAgentService_Search_QueryForwarding(t *testing.T) {
	// 测试场景：API Search 将 query 原样传给 Search Agent
	mockAgent := &mockSearchAgent{}
	mockImporter := &mockImporterService{}

	service := NewSearchAgentService(nil, mockImporter, mockAgent)

	_, err := service.Search(context.Background(), 1, 1, "测试查询")
	require.NoError(t, err)

	// 验证 query 被原样转发
	assert.True(t, mockAgent.executeCalled)
	assert.Equal(t, "测试查询", mockAgent.lastQuery)
	assert.Equal(t, uint(1), mockAgent.lastUserID)
	assert.Equal(t, uint(1), mockAgent.lastNotebookID)
}

func TestSearchAgentService_SearchStream_QueryForwarding(t *testing.T) {
	// 测试场景：API SearchStream 将 query 原样传给 Search Agent
	mockAgent := &mockSearchAgent{}
	mockImporter := &mockImporterService{}

	service := NewSearchAgentService(nil, mockImporter, mockAgent)

	eventCh := service.SearchStream(context.Background(), 1, 1, "流式查询")

	// 消费事件
	var events []*SearchAgentEvent
	for event := range eventCh {
		events = append(events, event)
	}

	// 验证 query 被原样转发
	assert.True(t, mockAgent.executeStreamCalled)
	assert.Equal(t, "流式查询", mockAgent.lastQuery)
	assert.Equal(t, uint(1), mockAgent.lastUserID)
	assert.Equal(t, uint(1), mockAgent.lastNotebookID)

	// 验证事件被正确传递
	assert.Len(t, events, 2)
}

func TestSearchAgentService_Search_EmptyQuery(t *testing.T) {
	// 测试场景：空 query 保持当前拒绝行为
	// 注意：当前实现不拒绝空 query，由 SearchAgent 处理
	// 这里记录基线行为
	mockAgent := &mockSearchAgent{}
	mockImporter := &mockImporterService{}

	service := NewSearchAgentService(nil, mockImporter, mockAgent)

	// 空 query 不会被 service 层拒绝
	_, err := service.Search(context.Background(), 1, 1, "")
	require.NoError(t, err)

	// 验证空 query 被传递给 agent
	assert.True(t, mockAgent.executeCalled)
	assert.Equal(t, "", mockAgent.lastQuery)
}

func TestSearchAgentService_Search_ResultParsing(t *testing.T) {
	// 测试场景：验证结果解析
	mockAgent := &mockSearchAgent{
		// 返回有效的 JSON 结果
	}
	mockImporter := &mockImporterService{}

	service := NewSearchAgentService(nil, mockImporter, mockAgent)

	result, err := service.Search(context.Background(), 1, 1, "查询")
	require.NoError(t, err)

	// 验证结果被正确解析
	assert.NotNil(t, result)
	assert.Equal(t, "测试摘要", result.Summary)
	assert.Len(t, result.Results, 1)
	assert.Equal(t, "测试结果", result.Results[0].Title)
	assert.Equal(t, "http://example.com", result.Results[0].URL)
}

func TestParseAgentResult_ValidJSON(t *testing.T) {
	// 测试场景：解析有效的 JSON 结果
	// 注意：直接解析整个内容为 JSON 时，SearchRounds 不会被设置
	// 只有通过 extractJSONBlock 提取的 JSON 才会设置 SearchRounds
	content := `{
		"results": [
			{"title": "结果1", "url": "http://example.com/1", "snippet": "摘要1", "score": 0.9}
		],
		"summary": "搜索摘要"
	}`

	result, err := parseAgentResult(content, 1)
	require.NoError(t, err)

	assert.Equal(t, "搜索摘要", result.Summary)
	assert.Len(t, result.Results, 1)
	// 基线行为：直接解析 JSON 时不设置 SearchRounds
	assert.Equal(t, 0, result.SearchRounds)
}

func TestParseAgentResult_JSONBlock(t *testing.T) {
	// 测试场景：解析 ```json 代码块
	content := `这是搜索结果：
` + "```json" + `
{
	"results": [
		{"title": "结果1", "url": "http://example.com/1", "snippet": "摘要1", "score": 0.9}
	],
	"summary": "JSON块摘要"
}
` + "```" + `
这是结尾`

	result, err := parseAgentResult(content, 2)
	require.NoError(t, err)

	assert.Equal(t, "JSON块摘要", result.Summary)
	assert.Len(t, result.Results, 1)
	assert.Equal(t, 2, result.SearchRounds)
}

func TestParseAgentResult_PlainText(t *testing.T) {
	// 测试场景：解析纯文本结果
	content := "这是一段纯文本搜索结果"

	result, err := parseAgentResult(content, 1)
	require.NoError(t, err)

	assert.Equal(t, "这是一段纯文本搜索结果", result.Summary)
	assert.Empty(t, result.Results)
}

func TestParseAgentResult_WithURLs(t *testing.T) {
	// 测试场景：从文本中提取 URL
	content := `搜索结果如下：
https://example.com/result1
https://example.com/result2`

	result, err := parseAgentResult(content, 1)
	require.NoError(t, err)

	assert.Len(t, result.Results, 2)
	assert.Equal(t, "https://example.com/result1", result.Results[0].URL)
	assert.Equal(t, "https://example.com/result2", result.Results[1].URL)
}

func TestParseAgentResult_WithMarkdownLinks(t *testing.T) {
	// 测试场景：从 Markdown 链接中提取 URL
	content := `搜索结果：
[结果1](https://example.com/result1)
[结果2](https://example.com/result2)`

	result, err := parseAgentResult(content, 1)
	require.NoError(t, err)

	assert.Len(t, result.Results, 2)
	assert.Equal(t, "https://example.com/result1", result.Results[0].URL)
	assert.Equal(t, "https://example.com/result2", result.Results[1].URL)
}

func TestSearchAgentInterface_Contract(t *testing.T) {
	// 测试场景：记录 SearchAgentInterface 的契约
	// 这是一个基线测试，记录当前的接口要求

	t.Log("基线契约：SearchAgentInterface 必须实现 Execute 和 ExecuteStream")
	t.Log("基线契约：Execute 接收 ctx, userID, notebookID, query")
	t.Log("基线契约：ExecuteStream 接收相同的参数并返回事件 channel")
	t.Log("基线契约：query 在 API 层不做预处理，原样传递给 agent")

	// 验证 mock 实现了接口
	var _ SearchAgentInterface = &mockSearchAgent{}
}

func TestSearchResponse_Structure(t *testing.T) {
	// 测试场景：记录 SearchResponse 的结构
	// 这是一个基线测试，记录当前的响应结构

	resp := &response.SearchResponse{
		Results: []response.SearchResultItem{
			{
				Title:   "标题",
				URL:     "http://example.com",
				Snippet: "摘要",
				Score:   0.9,
			},
		},
		Summary:      "搜索摘要",
		SearchRounds: 1,
	}

	assert.Len(t, resp.Results, 1)
	assert.Equal(t, "标题", resp.Results[0].Title)
	assert.Equal(t, "http://example.com", resp.Results[0].URL)
	assert.Equal(t, "摘要", resp.Results[0].Snippet)
	// 注意：Score 是 float32 类型
	assert.InDelta(t, float32(0.9), resp.Results[0].Score, 0.001)
	assert.Equal(t, "搜索摘要", resp.Summary)
	assert.Equal(t, 1, resp.SearchRounds)
}
