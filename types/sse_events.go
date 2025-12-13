package types

// ==================== SSE 事件结构（保证 type 字段在最前） ====================

// MessageStartEvent message_start 事件
type MessageStartEvent struct {
	Type    string       `json:"type"`
	Message *MessageInfo `json:"message"`
}

// MessageInfo 消息信息
type MessageInfo struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []any     `json:"content"`
	Model        string    `json:"model"`
	StopReason   *string   `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
	Usage        *UsageInfo `json:"usage"`
}

// UsageInfo 使用量信息（与官方 Claude API 一致）
type UsageInfo struct {
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// ContentBlockStartEvent content_block_start 事件
// 字段顺序: type, content_block, index (与官方 Claude API 一致)
type ContentBlockStartEvent struct {
	Type         string `json:"type"`
	ContentBlock any    `json:"content_block"`
	Index        int    `json:"index"`
}

// SSEContentBlock SSE 事件专用内容块（与 anthropic.ContentBlock 区分）
// 文本块必须包含 text 字段（即使为空字符串）
type SSEContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// SSETextContentBlock 文本内容块（text 字段始终显示）
type SSETextContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ContentBlockDeltaEvent content_block_delta 事件
// 字段顺序: type, delta, index (与官方 Claude API 一致)
type ContentBlockDeltaEvent struct {
	Type  string      `json:"type"`
	Delta *DeltaBlock `json:"delta"`
	Index int         `json:"index"`
}

// DeltaBlock delta 块
type DeltaBlock struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// ContentBlockStopEvent content_block_stop 事件
type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// MessageDeltaEvent message_delta 事件
type MessageDeltaEvent struct {
	Type  string            `json:"type"`
	Delta *MessageDeltaInfo `json:"delta"`
	Usage *UsageInfo        `json:"usage,omitempty"`
}

// MessageDeltaInfo message delta 信息
// stop_sequence 字段始终显示（即使为 null）
type MessageDeltaInfo struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

// MessageStopEvent message_stop 事件
type MessageStopEvent struct {
	Type string `json:"type"`
}

// ErrorEvent error 事件
type ErrorEvent struct {
	Type  string     `json:"type"`
	Error *ErrorInfo `json:"error"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ==================== 构造函数 ====================

// NewMessageStartEvent 创建 message_start 事件
func NewMessageStartEvent(msg *MessageInfo) *MessageStartEvent {
	return &MessageStartEvent{
		Type:    "message_start",
		Message: msg,
	}
}

// NewContentBlockStartEvent 创建 content_block_start 事件
func NewContentBlockStartEvent(index int, block any) *ContentBlockStartEvent {
	return &ContentBlockStartEvent{
		Type:         "content_block_start",
		ContentBlock: block,
		Index:        index,
	}
}

// NewTextContentBlock 创建文本内容块
func NewTextContentBlock(text string) *SSEContentBlock {
	return &SSEContentBlock{
		Type: "text",
		Text: text,
	}
}

// NewToolUseContentBlock 创建工具使用内容块
func NewToolUseContentBlock(id, name string, input any) *SSEContentBlock {
	if input == nil {
		input = map[string]any{}
	}
	return &SSEContentBlock{
		Type:  "tool_use",
		ID:    id,
		Name:  name,
		Input: input,
	}
}

// NewContentBlockDeltaEvent 创建 content_block_delta 事件
func NewContentBlockDeltaEvent(index int, delta *DeltaBlock) *ContentBlockDeltaEvent {
	return &ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: delta,
	}
}

// NewTextDelta 创建文本 delta
func NewTextDelta(text string) *DeltaBlock {
	return &DeltaBlock{
		Type: "text_delta",
		Text: text,
	}
}

// NewInputJSONDelta 创建 JSON delta
func NewInputJSONDelta(partialJSON string) *DeltaBlock {
	return &DeltaBlock{
		Type:        "input_json_delta",
		PartialJSON: partialJSON,
	}
}

// NewContentBlockStopEvent 创建 content_block_stop 事件
func NewContentBlockStopEvent(index int) *ContentBlockStopEvent {
	return &ContentBlockStopEvent{
		Type:  "content_block_stop",
		Index: index,
	}
}

// NewMessageDeltaEvent 创建 message_delta 事件
func NewMessageDeltaEvent(stopReason string, usage *UsageInfo) *MessageDeltaEvent {
	return &MessageDeltaEvent{
		Type: "message_delta",
		Delta: &MessageDeltaInfo{
			StopReason: stopReason,
		},
		Usage: usage,
	}
}

// NewMessageStopEvent 创建 message_stop 事件
func NewMessageStopEvent() *MessageStopEvent {
	return &MessageStopEvent{
		Type: "message_stop",
	}
}

// NewErrorEvent 创建 error 事件
func NewErrorEvent(errType, message string) *ErrorEvent {
	return &ErrorEvent{
		Type: "error",
		Error: &ErrorInfo{
			Type:    errType,
			Message: message,
		},
	}
}
