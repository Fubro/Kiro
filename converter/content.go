package converter

import (
	"fmt"
	"strings"

	"kiro/types"
	"kiro/utils"
)

// 消息内容处理器

// processMessageContent 处理消息内容，提取文本和图片
func processMessageContent(content any) (string, []types.CodeWhispererImage, error) {
	var thinkingParts []string // thinking 内容（放在最前面）
	var textParts []string
	var images []types.CodeWhispererImage

	switch v := content.(type) {
	case string:
		// 简单字符串内容
		return v, nil, nil

	case []any:
		// 内容块数组
		for i, item := range v {
			if block, ok := item.(map[string]any); ok {
				contentBlock, err := parseContentBlock(block)
				if err != nil {
					utils.Log("解析内容块失败，跳过", utils.LogErr(err), utils.LogInt("index", i))
					continue // 跳过无法解析的块
				}

				switch contentBlock.Type {
				case "text":
					if contentBlock.Text != nil {
						textParts = append(textParts, *contentBlock.Text)
					} else {
						utils.Log("文本块的Text字段为nil")
					}
				case "image":
					// ... 图片处理保持不变
					if contentBlock.Source != nil {
						// 验证图片内容
						if err := utils.ValidateImageContent(contentBlock.Source); err != nil {
							return "", nil, fmt.Errorf("图片验证失败: %v", err)
						}

						// 转换为 CodeWhisperer 格式
						cwImage := utils.CreateCodeWhispererImage(contentBlock.Source)
						if cwImage != nil {
							images = append(images, *cwImage)
						}
					}
				case "tool_result":
					// 处理工具结果，支持复杂的内容结构
					if contentBlock.Content != nil {
						parsedContent := utils.ParseToolResultContent(contentBlock.Content)
						// 如果内容为空，提供默认值
						if parsedContent == "" {
							parsedContent = "Tool executed successfully"
						}
						textParts = append(textParts, parsedContent)
					}
				case "thinking":
					// thinking 块转换为 <thinking> 标签格式，加到正文前面
					if contentBlock.Text != nil && *contentBlock.Text != "" {
						thinkingParts = append(thinkingParts, "<thinking>\n"+*contentBlock.Text+"\n</thinking>")
					}
				}
			} else {
				utils.Log("内容块不是map[string]any类型",
					utils.LogInt("index", i),
					utils.LogString("actual_type", fmt.Sprintf("%T", item)))
			}
		}

	case []types.ContentBlock:
		// 结构化的内容块数组
		for _, block := range v {
			switch block.Type {
			case "text":
				if block.Text != nil {
					textParts = append(textParts, *block.Text)
				} else {
					utils.Log("结构化文本块的Text字段为nil")
				}
			case "image":
				if block.Source != nil {
					// 验证图片内容
					if err := utils.ValidateImageContent(block.Source); err != nil {
						return "", nil, fmt.Errorf("图片验证失败: %v", err)
					}

					// 转换为 CodeWhisperer 格式
					cwImage := utils.CreateCodeWhispererImage(block.Source)
					if cwImage != nil {
						images = append(images, *cwImage)
					}
				}
			case "tool_result":
				// 处理工具结果，支持复杂的内容结构
				if block.Content != nil {
					parsedContent := utils.ParseToolResultContent(block.Content)
					// 如果内容为空，提供默认值
					if parsedContent == "" {
						parsedContent = "Tool executed successfully"
					}
					textParts = append(textParts, parsedContent)
				}
			case "thinking":
				// thinking 块转换为 <thinking> 标签格式
				if block.Text != nil && *block.Text != "" {
					thinkingParts = append(thinkingParts, "<thinking>\n"+*block.Text+"\n</thinking>")
				}
			}
		}

	default:
		// 不支持的内容类型，返回错误而非fallback
		return "", nil, fmt.Errorf("不支持的内容类型: %T", content)
	}

	// 组合结果：thinking 内容在前，普通文本在后
	var allParts []string
	allParts = append(allParts, thinkingParts...)
	allParts = append(allParts, textParts...)
	result := strings.Join(allParts, "\n\n")

	// 保留关键调试信息用于问题定位
	if result == "" && len(images) == 0 {
		utils.Log("消息内容处理结果为空",
			utils.LogString("content_type", fmt.Sprintf("%T", content)),
			utils.LogInt("thinking_parts_count", len(thinkingParts)),
			utils.LogInt("text_parts_count", len(textParts)),
			utils.LogInt("images_count", len(images)))
	}

	return result, images, nil
}

// parseContentBlock 解析内容块
func parseContentBlock(block map[string]any) (types.ContentBlock, error) {
	var contentBlock types.ContentBlock

	// 解析类型
	if blockType, ok := block["type"].(string); ok {
		contentBlock.Type = blockType
	} else {
		utils.Log("内容块缺少type字段或type不是字符串",
			utils.LogString("type_value", fmt.Sprintf("%v", block["type"])),
			utils.LogString("type_type", fmt.Sprintf("%T", block["type"])))
		return contentBlock, fmt.Errorf("缺少内容块类型")
	}

	// 根据类型解析不同字段
	switch contentBlock.Type {
	case "text":
		if text, ok := block["text"].(string); ok {
			contentBlock.Text = &text
		} else {
			utils.Log("文本块缺少text字段或不是字符串",
				utils.LogString("text_value", fmt.Sprintf("%v", block["text"])),
				utils.LogString("text_type", fmt.Sprintf("%T", block["text"])))
		}

	case "image":
		if source, ok := block["source"].(map[string]any); ok {
			imageSource := &types.ImageSource{}

			if sourceType, ok := source["type"].(string); ok {
				imageSource.Type = sourceType
			}
			if mediaType, ok := source["media_type"].(string); ok {
				imageSource.MediaType = mediaType
			}
			if data, ok := source["data"].(string); ok {
				imageSource.Data = data
			}

			contentBlock.Source = imageSource
		}

	case "image_url":
		// 处理 image_url 格式的图片块，转换为 Anthropic 格式
		if imageURL, ok := block["image_url"].(map[string]any); ok {
			imageSource, err := utils.ConvertImageURLToImageSource(imageURL)
			if err != nil {
				return contentBlock, fmt.Errorf("转换image_url失败: %v", err)
			}
			// 将类型改为image并设置source
			contentBlock.Type = "image"
			contentBlock.Source = imageSource
		}

	case "tool_result":
		if toolUseId, ok := block["tool_use_id"].(string); ok {
			contentBlock.ToolUseId = &toolUseId
		}
		if content, ok := block["content"]; ok {
			contentBlock.Content = content
		}
		if isError, ok := block["is_error"].(bool); ok {
			contentBlock.IsError = &isError
		}

	case "tool_use":
		if id, ok := block["id"].(string); ok {
			contentBlock.ID = &id
		}
		if name, ok := block["name"].(string); ok {
			contentBlock.Name = &name
		}
		if input, ok := block["input"]; ok {
			contentBlock.Input = &input
		}

	case "thinking":
		// 处理 thinking 块，提取 thinking 内容，忽略 signature 字段
		// Anthropic API 要求在后续请求中保留 thinking 内容，但我们需要将其
		// 转换为上游 API 可以理解的格式（作为文本或忽略）
		if thinking, ok := block["thinking"].(string); ok {
			contentBlock.Text = &thinking
		}
		// 注意：signature 字段被故意忽略，不会被传递到上游
	}

	return contentBlock, nil
}
