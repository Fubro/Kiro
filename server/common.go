package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kiro/config"
	"kiro/converter"

	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// respondErrorWithCode 标准化的错误响应结构
// 统一返回: {"error": {"message": string, "code": string}}
func respondErrorWithCode(c *gin.Context, statusCode int, code string, format string, args ...any) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": fmt.Sprintf(format, args...),
			"code":    code,
		},
	})
}

// respondError 简化封装，依据statusCode映射默认code
func respondError(c *gin.Context, statusCode int, format string, args ...any) {
	var code string
	switch statusCode {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	default:
		code = "internal_error"
	}
	respondErrorWithCode(c, statusCode, code, format, args...)
}

// 通用请求处理错误函数
func handleRequestBuildError(c *gin.Context, err error) {
	utils.Log("构建请求失败", addReqFields(c, utils.LogErr(err))...)
	respondError(c, http.StatusInternalServerError, "构建请求失败: %v", err)
}

func handleRequestSendError(c *gin.Context, err error) {
	utils.Log("发送请求失败", addReqFields(c, utils.LogErr(err))...)
	respondError(c, http.StatusInternalServerError, "发送请求失败: %v", err)
}

func handleResponseReadError(c *gin.Context, err error) {
	utils.Log("读取响应体失败", addReqFields(c, utils.LogErr(err))...)
	respondError(c, http.StatusInternalServerError, "读取响应体失败: %v", err)
}

// 通用请求执行函数
// filterSupportedTools 过滤掉不支持的工具（与上游转换逻辑保持一致）
// 设计原则：
// - DRY: 统一过滤逻辑，确保计费与上游请求一致
// - KISS: 简单直接的过滤规则
func filterSupportedTools(tools []types.AnthropicTool) []types.AnthropicTool {
	if len(tools) == 0 {
		return tools
	}

	filtered := make([]types.AnthropicTool, 0, len(tools))
	for _, tool := range tools {
		// 过滤不支持的工具：web_search（与 converter/codewhisperer.go 保持一致）
		if tool.Name == "web_search" || tool.Name == "websearch" {
			utils.Log("过滤不支持的工具（token计算）",
				utils.LogString("tool_name", tool.Name))
			continue
		}
		filtered = append(filtered, tool)
	}

	return filtered
}

func executeCodeWhispererRequest(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Response, error) {
	req, err := buildCodeWhispererRequest(c, anthropicReq, tokenInfo, isStream)
	if err != nil {
		// 检查是否是模型未找到错误，如果是，则响应已经发送，不需要再次处理
		if _, ok := err.(*types.ModelNotFoundErrorType); ok {
			return nil, err
		}
		handleRequestBuildError(c, err)
		return nil, err
	}

	resp, err := utils.DoRequest(req)
	if err != nil {
		handleRequestSendError(c, err)
		return nil, err
	}

	if handleCodeWhispererError(c, resp) {
		resp.Body.Close()
		return nil, fmt.Errorf("CodeWhisperer API error")
	}

	// 上游响应成功，记录方向与会话
	utils.Log("上游响应成功",
		addReqFields(c,
			utils.LogString("direction", "upstream_response"),
			utils.LogInt("status_code", resp.StatusCode),
		)...)

	return resp, nil
}

// execCWRequest 供测试覆盖的请求执行入口（可在测试中替换）
var execCWRequest = executeCodeWhispererRequest

// buildCodeWhispererRequest 构建通用的CodeWhisperer请求
func buildCodeWhispererRequest(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Request, error) {
	cwReq, err := converter.BuildCodeWhispererRequest(anthropicReq, c)
	if err != nil {
		// 检查是否是模型未找到错误
		if modelNotFoundErr, ok := err.(*types.ModelNotFoundErrorType); ok {
			// 直接返回用户期望的JSON格式
			c.JSON(http.StatusBadRequest, modelNotFoundErr.ErrorData)
			return nil, err
		}
		return nil, fmt.Errorf("构建CodeWhisperer请求失败: %v", err)
	}

	cwReqBody, err := utils.SafeMarshal(cwReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	// 记录发送给CodeWhisperer的请求
	var toolNamesPreview string
	if len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools) > 0 {
		names := make([]string, 0, len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools))
		for _, t := range cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools {
			if t.ToolSpecification.Name != "" {
				names = append(names, t.ToolSpecification.Name)
			}
		}
		toolNamesPreview = strings.Join(names, ",")
	}

	utils.Log("发送给CodeWhisperer的请求",
		utils.LogString("direction", "upstream_request"),
		utils.LogInt("request_size", len(cwReqBody)),
		utils.LogString("request_body", string(cwReqBody)),
		utils.LogInt("tools_count", len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools)),
		utils.LogString("tools_names", toolNamesPreview))

	req, err := http.NewRequest("POST", config.CodeWhispererURL, bytes.NewReader(cwReqBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// 添加上游请求必需的header
	req.Header.Set("x-amzn-kiro-agent-mode", "spec")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.18 KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")
	req.Header.Set("user-agent", "aws-sdk-js/1.0.18 ua/2.1 os/darwin#25.0.0 lang/js md/nodejs#20.16.0 api/codewhispererstreaming#1.0.18 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")

	return req, nil
}

// handleCodeWhispererError 处理CodeWhisperer API错误响应 (重构后符合SOLID原则)
func handleCodeWhispererError(c *gin.Context, resp *http.Response) bool {
	if resp.StatusCode == http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Log("读取错误响应失败",
			addReqFields(c,
				utils.LogString("direction", "upstream_response"),
				utils.LogErr(err),
			)...)
		respondError(c, http.StatusInternalServerError, "%s", "读取响应失败")
		return true
	}

	utils.Log("上游响应错误",
		addReqFields(c,
			utils.LogString("direction", "upstream_response"),
			utils.LogInt("status_code", resp.StatusCode),
			utils.LogInt("response_len", len(body)),
			utils.LogString("response_body", string(body)),
		)...)

	// 特殊处理：403错误表示token失效 (保持向后兼容)
	if resp.StatusCode == http.StatusForbidden {
		utils.Log("收到403错误，token可能已失效")
		respondErrorWithCode(c, http.StatusUnauthorized, "unauthorized", "%s", "Token已失效，请重试")
		return true
	}

	// *** 新增：使用错误映射器处理错误，符合Claude API规范 ***
	errorMapper := NewErrorMapper()
	claudeError := errorMapper.MapCodeWhispererError(resp.StatusCode, body)

	// 根据映射结果发送符合Claude规范的响应
	if claudeError.StopReason == "max_tokens" {
		// CONTENT_LENGTH_EXCEEDS_THRESHOLD -> max_tokens stop_reason
		utils.Log("内容长度超限，映射为max_tokens stop_reason",
			addReqFields(c,
				utils.LogString("upstream_reason", "CONTENT_LENGTH_EXCEEDS_THRESHOLD"),
				utils.LogString("claude_stop_reason", "max_tokens"),
			)...)
		errorMapper.SendClaudeError(c, claudeError)
	} else {
		// 其他错误使用传统方式处理 (向后兼容)
		respondErrorWithCode(c, http.StatusInternalServerError, "cw_error", "CodeWhisperer Error: %s", string(body))
	}

	return true
}

// StreamEventSender 统一的流事件发送接口
type StreamEventSender interface {
	SendEvent(c *gin.Context, data any) error
	SendError(c *gin.Context, message string, err error) error
}

// AnthropicStreamSender Anthropic格式的流事件发送器
type AnthropicStreamSender struct{}

func (s *AnthropicStreamSender) SendEvent(c *gin.Context, data any) error {
	var eventType string
	var orderedData any = data

	// 如果是 map，转换为有序 struct
	if dataMap, ok := data.(map[string]any); ok {
		if t, exists := dataMap["type"]; exists {
			eventType, _ = t.(string)
		}
		orderedData = convertToOrderedStruct(dataMap)
	} else {
		// 从 struct 中提取 type
		switch v := data.(type) {
		case *types.MessageStartEvent:
			eventType = v.Type
		case *types.ContentBlockStartEvent:
			eventType = v.Type
		case *types.ContentBlockDeltaEvent:
			eventType = v.Type
		case *types.ContentBlockStopEvent:
			eventType = v.Type
		case *types.MessageDeltaEvent:
			eventType = v.Type
		case *types.MessageStopEvent:
			eventType = v.Type
		case *types.ErrorEvent:
			eventType = v.Type
		}
	}

	json, err := utils.SafeMarshal(orderedData)
	if err != nil {
		return err
	}

	utils.Log("发送SSE事件",
		addReqFields(c,
			utils.LogString("event", eventType),
			utils.LogString("payload_preview", string(json)),
		)...)

	fmt.Fprintf(c.Writer, "event: %s\n", eventType)
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(json))
	c.Writer.Flush()
	return nil
}

// convertToOrderedStruct 将 map 转换为有序 struct（保证 type 在最前）
func convertToOrderedStruct(m map[string]any) any {
	eventType, _ := m["type"].(string)

	switch eventType {
	case "message_start":
		return convertMessageStart(m)
	case "content_block_start":
		return convertContentBlockStart(m)
	case "content_block_delta":
		return convertContentBlockDelta(m)
	case "content_block_stop":
		return convertContentBlockStop(m)
	case "message_delta":
		return convertMessageDelta(m)
	case "message_stop":
		return types.NewMessageStopEvent()
	case "error":
		return convertError(m)
	default:
		return m // 未知类型保持原样
	}
}

func convertMessageStart(m map[string]any) *types.MessageStartEvent {
	msg := &types.MessageInfo{}
	if message, ok := m["message"].(map[string]any); ok {
		msg.ID, _ = message["id"].(string)
		msg.Type, _ = message["type"].(string)
		msg.Role, _ = message["role"].(string)
		msg.Model, _ = message["model"].(string)
		if content, ok := message["content"].([]any); ok {
			msg.Content = content
		} else {
			msg.Content = []any{}
		}
		if usage, ok := message["usage"].(map[string]any); ok {
			msg.Usage = &types.UsageInfo{}
			// cache 相关字段
			if v, ok := usage["cache_creation_input_tokens"].(int); ok {
				msg.Usage.CacheCreationInputTokens = v
			} else if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
				msg.Usage.CacheCreationInputTokens = int(v)
			}
			if v, ok := usage["cache_read_input_tokens"].(int); ok {
				msg.Usage.CacheReadInputTokens = v
			} else if v, ok := usage["cache_read_input_tokens"].(float64); ok {
				msg.Usage.CacheReadInputTokens = int(v)
			}
			// 基础 token 字段
			if v, ok := usage["input_tokens"].(int); ok {
				msg.Usage.InputTokens = v
			} else if v, ok := usage["input_tokens"].(float64); ok {
				msg.Usage.InputTokens = int(v)
			}
			if v, ok := usage["output_tokens"].(int); ok {
				msg.Usage.OutputTokens = v
			} else if v, ok := usage["output_tokens"].(float64); ok {
				msg.Usage.OutputTokens = int(v)
			}
		}
	}
	return types.NewMessageStartEvent(msg)
}

func convertContentBlockStart(m map[string]any) *types.ContentBlockStartEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}

	var block any
	if cb, ok := m["content_block"].(map[string]any); ok {
		blockType, _ := cb["type"].(string)
		if blockType == "text" {
			// 文本块：始终包含 text 字段（即使为空）
			text, _ := cb["text"].(string)
			block = &types.SSETextContentBlock{
				Type: "text",
				Text: text,
			}
		} else {
			// 其他类型（如 tool_use）
			sseBlock := &types.SSEContentBlock{}
			sseBlock.Type = blockType
			sseBlock.Text, _ = cb["text"].(string)
			sseBlock.ID, _ = cb["id"].(string)
			sseBlock.Name, _ = cb["name"].(string)
			if input, exists := cb["input"]; exists {
				sseBlock.Input = input
			}
			block = sseBlock
		}
	}
	return types.NewContentBlockStartEvent(index, block)
}

func convertContentBlockDelta(m map[string]any) *types.ContentBlockDeltaEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}

	delta := &types.DeltaBlock{}
	if d, ok := m["delta"].(map[string]any); ok {
		delta.Type, _ = d["type"].(string)
		delta.Text, _ = d["text"].(string)
		delta.PartialJSON, _ = d["partial_json"].(string)
	}
	return types.NewContentBlockDeltaEvent(index, delta)
}

func convertContentBlockStop(m map[string]any) *types.ContentBlockStopEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}
	return types.NewContentBlockStopEvent(index)
}

func convertMessageDelta(m map[string]any) *types.MessageDeltaEvent {
	stopReason := ""
	if delta, ok := m["delta"].(map[string]any); ok {
		stopReason, _ = delta["stop_reason"].(string)
	}

	var usage *types.UsageInfo
	if u, ok := m["usage"].(map[string]any); ok {
		usage = &types.UsageInfo{}
		// cache 相关字段
		if v, ok := u["cache_creation_input_tokens"].(int); ok {
			usage.CacheCreationInputTokens = v
		} else if v, ok := u["cache_creation_input_tokens"].(float64); ok {
			usage.CacheCreationInputTokens = int(v)
		}
		if v, ok := u["cache_read_input_tokens"].(int); ok {
			usage.CacheReadInputTokens = v
		} else if v, ok := u["cache_read_input_tokens"].(float64); ok {
			usage.CacheReadInputTokens = int(v)
		}
		// 基础 token 字段
		if v, ok := u["input_tokens"].(int); ok {
			usage.InputTokens = v
		} else if v, ok := u["input_tokens"].(float64); ok {
			usage.InputTokens = int(v)
		}
		if v, ok := u["output_tokens"].(int); ok {
			usage.OutputTokens = v
		} else if v, ok := u["output_tokens"].(float64); ok {
			usage.OutputTokens = int(v)
		}
	}
	return types.NewMessageDeltaEvent(stopReason, usage)
}

func convertError(m map[string]any) *types.ErrorEvent {
	errType := "error"
	errMsg := ""
	if e, ok := m["error"].(map[string]any); ok {
		errType, _ = e["type"].(string)
		errMsg, _ = e["message"].(string)
	}
	return types.NewErrorEvent(errType, errMsg)
}

func (s *AnthropicStreamSender) SendError(c *gin.Context, message string, _ error) error {
	return s.SendEvent(c, types.NewErrorEvent("overloaded_error", message))
}

// RequestContext 请求处理上下文，封装通用的请求处理逻辑
type RequestContext struct {
	GinContext  *gin.Context
	AuthService interface {
		GetToken() (types.TokenInfo, error)
		GetTokenWithUsage() (*types.TokenWithUsage, error)
	}
	RequestType string // "Anthropic"
}

// GetTokenAndBody 通用的token获取和请求体读取
// 返回: tokenInfo, requestBody, error
func (rc *RequestContext) GetTokenAndBody() (types.TokenInfo, []byte, error) {
	// 获取token
	tokenInfo, err := rc.AuthService.GetToken()
	if err != nil {
		utils.Log("获取token失败", utils.LogErr(err))
		respondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	// 读取请求体
	body, err := rc.GinContext.GetRawData()
	if err != nil {
		utils.Log("读取请求体失败", utils.LogErr(err))
		respondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	// 记录请求日志
	utils.Log(fmt.Sprintf("收到%s请求", rc.RequestType),
		addReqFields(rc.GinContext,
			utils.LogString("direction", "client_request"),
			utils.LogString("body", string(body)),
			utils.LogInt("body_size", len(body)),
			utils.LogString("remote_addr", rc.GinContext.ClientIP()),
			utils.LogString("user_agent", rc.GinContext.GetHeader("User-Agent")),
		)...)

	return tokenInfo, body, nil
}

// GetTokenWithUsageAndBody 获取token（包含使用信息）和请求体
// 返回: tokenWithUsage, requestBody, error
func (rc *RequestContext) GetTokenWithUsageAndBody() (*types.TokenWithUsage, []byte, error) {
	// 获取token（包含使用信息）
	tokenWithUsage, err := rc.AuthService.GetTokenWithUsage()
	if err != nil {
		utils.Log("获取token失败", utils.LogErr(err))
		respondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return nil, nil, err
	}

	// 读取请求体
	body, err := rc.GinContext.GetRawData()
	if err != nil {
		utils.Log("读取请求体失败", utils.LogErr(err))
		respondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return nil, nil, err
	}

	// 记录请求日志
	utils.Log(fmt.Sprintf("收到%s请求", rc.RequestType),
		addReqFields(rc.GinContext,
			utils.LogString("direction", "client_request"),
			utils.LogString("body", string(body)),
			utils.LogInt("body_size", len(body)),
			utils.LogString("remote_addr", rc.GinContext.ClientIP()),
			utils.LogString("user_agent", rc.GinContext.GetHeader("User-Agent")),
			utils.LogAny("available_count", tokenWithUsage.AvailableCount),
		)...)

	return tokenWithUsage, body, nil
}
