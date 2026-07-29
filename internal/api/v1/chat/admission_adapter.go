package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"YoudaoNoteLm/internal/agentharness/core"
	"YoudaoNoteLm/internal/agentharness/run"
	"YoudaoNoteLm/internal/middleware"
	"YoudaoNoteLm/internal/model/dto/request"
	"YoudaoNoteLm/internal/service"
	"YoudaoNoteLm/pkg/response"

	"github.com/gin-gonic/gin"
)

// AdmissionAdapter 是 Chat API 的 Admission 适配器。
// 它将现有 Chat API 请求转换为 Admission 请求，通过 AdmissionService 创建 Run。
type AdmissionAdapter struct {
	admissionSvc *run.AdmissionService
	// enabled 控制是否启用 Admission 路径（feature flag）。
	enabled bool
}

// NewAdmissionAdapter 创建 AdmissionAdapter。
// enabled 参数为 feature flag，控制是否使用 Admission 路径。
func NewAdmissionAdapter(admissionSvc *run.AdmissionService, enabled bool) *AdmissionAdapter {
	return &AdmissionAdapter{
		admissionSvc: admissionSvc,
		enabled:      enabled,
	}
}

// IsEnabled 返回是否启用 Admission 路径。
func (a *AdmissionAdapter) IsEnabled() bool {
	return a.enabled && a.admissionSvc != nil
}

// SendMessageAdapted 适配现有的 SendMessage 接口，通过 Admission 创建 Run。
// 保持与现有 API 兼容的请求/响应格式。
func (a *AdmissionAdapter) SendMessageAdapted(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "用户未登录")
		return
	}

	convID, err := parseConvID(c)
	if err != nil {
		response.BadRequest(c, "无效的对话 ID")
		return
	}

	var req request.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 构建 AcceptRequest
	contentHash := sha256.Sum256([]byte(req.Content))
	acceptReq := core.AcceptRequest{
		UserID:         userID,
		ConversationID: uintPtr(convID),
		NotebookID:     uintPtr(req.NotebookID),
		AgentType:      "chat",
		Input: core.InputRef{
			Kind: "chat_message",
			Ref:  req.Content,
			Hash: hex.EncodeToString(contentHash[:]),
		},
		SourceIDs:      req.SourceIDs,
		IdempotencyKey: "", // 自动生成
		VersionSnapshot: core.VersionSnapshot{
			AgentDefinitionVersion: "v1",
			PromptVersion:          "v1",
			ToolSchemaVersion:      "v1",
			ModelConfigHash:        "default",
			ContextProfileVersion:  "v1",
			EinoVersion:            "v1",
		},
	}

	// 调用 AdmissionService
	accepted, err := a.admissionSvc.Accept(c.Request.Context(), acceptReq)
	if err != nil {
		response.BizError(c, err)
		return
	}

	// 返回兼容格式的响应
	response.Success(c, gin.H{
		"run_id":              string(accepted.RunID),
		"message_id":          accepted.MessageID,
		"state":               string(accepted.State),
		"sequence":            accepted.Sequence,
		"is_idempotent_replay": accepted.IsIdempotentReplay,
	})
}

// RegisterAdmissionRoutes 注册 Admission 路由。
func RegisterAdmissionRoutes(rg *gin.RouterGroup, adapter *AdmissionAdapter, tokenBlacklist service.TokenBlacklistService, statusCheck gin.HandlerFunc) {
	admission := rg.Group("/admission")
	admission.Use(middleware.Auth(tokenBlacklist), statusCheck)
	{
		// 兼容现有 Chat API 的 Admission 端点
		admission.POST("/conversations/:convId/messages", adapter.SendMessageAdapted)
	}
}

// parseConvID 从 URL 参数解析对话 ID。
func parseConvID(c *gin.Context) (uint, error) {
	convID, err := parseUintParam(c, "convId")
	if err != nil {
		return 0, err
	}
	return convID, nil
}

// parseUintParam 从 URL 参数解析 uint。
func parseUintParam(c *gin.Context, param string) (uint, error) {
	val, err := parseUint(c.Param(param))
	if err != nil {
		return 0, err
	}
	return val, nil
}

// parseUint 解析 uint 字符串。
func parseUint(s string) (uint, error) {
	var val uint
	_, err := fmt.Sscanf(s, "%d", &val)
	return val, err
}

// uintPtr 返回 uint 的指针。
func uintPtr(v uint) *uint {
	if v == 0 {
		return nil
	}
	return &v
}
