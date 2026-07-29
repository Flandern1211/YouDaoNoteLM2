package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// SSEHandler SSE 处理器。
type SSEHandler struct {
	store    EventStore
	notifier EventNotifier
}

// NewSSEHandler 创建 SSEHandler。
func NewSSEHandler(store EventStore, notifier EventNotifier) *SSEHandler {
	return &SSEHandler{
		store:    store,
		notifier: notifier,
	}
}

// HandleSSE 处理 SSE 连接。
// 路径: /api/v1/runs/:runID/events
func (h *SSEHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		http.Error(w, "missing runID", http.StatusBadRequest)
		return
	}

	// 解析 after_seq 参数
	afterSeqStr := r.URL.Query().Get("after_seq")
	afterSeq := uint64(0)
	if afterSeqStr != "" {
		var err error
		afterSeq, err = strconv.ParseUint(afterSeqStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid after_seq", http.StatusBadRequest)
			return
		}
	}

	// 验证 Run 所有权（简化版，实际需要从 context 获取用户 ID）
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 设置 SSE 头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// 1. 先回放 after_seq 之后的持久化事件
	events, err := h.store.ListEvents(ctx, runID, afterSeq, 100)
	if err != nil {
		http.Error(w, "failed to list events", http.StatusInternalServerError)
		return
	}

	for _, event := range events {
		if err := h.writeEvent(w, flusher, event); err != nil {
			return
		}
		afterSeq = event.Sequence
	}

	// 2. 订阅实时事件
	eventCh, err := h.notifier.Subscribe(runID)
	if err != nil {
		http.Error(w, "failed to subscribe", http.StatusInternalServerError)
		return
	}
	defer h.notifier.Unsubscribe(runID, eventCh)

	// 3. 持续推送事件
	for {
		select {
		case <-ctx.Done():
			// 客户端断开，只 detach，不取消 Run
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			// 只推送 after_seq 之后的事件
			if event.Sequence > afterSeq {
				if err := h.writeEvent(w, flusher, event); err != nil {
					return
				}
				afterSeq = event.Sequence
			}
		}
	}
}

// writeEvent 写入 SSE 事件。
func (h *SSEHandler) writeEvent(w http.ResponseWriter, flusher http.Flusher, event RunEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.EventType, string(data))
	if err != nil {
		return err
	}

	flusher.Flush()
	return nil
}

// HandleGetRun 处理获取 Run 状态请求。
// 路径: /api/v1/runs/:runID
func (h *SSEHandler) HandleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	if runID == "" {
		http.Error(w, "missing runID", http.StatusBadRequest)
		return
	}

	// 验证 Run 所有权
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 获取 Run 状态
	runInfo, err := h.store.GetRun(r.Context(), runID)
	if err != nil {
		http.Error(w, "failed to get run", http.StatusInternalServerError)
		return
	}

	// 返回 JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runInfo)
}
