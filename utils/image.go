package utils

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"

	"kiro/types"
)

// SupportedImageFormats 支持的图片格式
var SupportedImageFormats = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// MaxImageSize 最大图片大小 (20MB)
const MaxImageSize = 20 * 1024 * 1024

// DetectImageFormat 检测图片格式
func DetectImageFormat(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("文件太小，无法检测格式")
	}

	// 检测 JPEG
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg", nil
	}

	// 检测 PNG
	if len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "image/png", nil
	}

	// 检测 GIF
	if len(data) >= 6 &&
		((data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x37 && data[5] == 0x61) ||
			(data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x39 && data[5] == 0x61)) {
		return "image/gif", nil
	}

	// 检测 WebP
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp", nil
	}

	// 检测 BMP
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return "image/bmp", nil
	}

	return "", fmt.Errorf("不支持的图片格式")
}

// IsSupportedImageFormat 检查是否为支持的图片格式
func IsSupportedImageFormat(mediaType string) bool {
	return GetImageFormatFromMediaType(mediaType) != ""
}

// GetImageFormatFromMediaType 从 media type 获取图片格式
func GetImageFormatFromMediaType(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	default:
		return ""
	}
}

// CreateCodeWhispererImage 创建 CodeWhisperer 格式的图片对象
func CreateCodeWhispererImage(imageSource *types.ImageSource) *types.CodeWhispererImage {
	if imageSource == nil {
		return nil
	}

	format := GetImageFormatFromMediaType(imageSource.MediaType)
	if format == "" {
		return nil
	}

	return &types.CodeWhispererImage{
		Format: format,
		Source: struct {
			Bytes string `json:"bytes"`
		}{
			Bytes: imageSource.Data,
		},
	}
}

// ValidateImageContent 验证图片内容的完整性
func ValidateImageContent(imageSource *types.ImageSource) error {
	if imageSource == nil {
		return fmt.Errorf("图片数据为空")
	}

	if imageSource.Type != "base64" {
		return fmt.Errorf("不支持的图片类型: %s", imageSource.Type)
	}

	if !IsSupportedImageFormat(imageSource.MediaType) {
		return fmt.Errorf("不支持的图片格式: %s", imageSource.MediaType)
	}

	if imageSource.Data == "" {
		return fmt.Errorf("图片数据为空")
	}

	// 验证 base64 编码并检查大小
	decodedData, err := base64.StdEncoding.DecodeString(imageSource.Data)
	if err != nil {
		return fmt.Errorf("无效的 base64 编码: %v", err)
	}

	if len(decodedData) > MaxImageSize {
		return fmt.Errorf("图片数据过大: %d 字节，最大支持 %d 字节", len(decodedData), MaxImageSize)
	}

	// 验证图片格式与数据是否匹配
	detectedType, err := DetectImageFormat(decodedData)
	if err == nil && detectedType != imageSource.MediaType {
		return fmt.Errorf("图片格式不匹配: 声明为 %s，实际为 %s", imageSource.MediaType, detectedType)
	}

	return nil
}

// ParseDataURL 解析data URL，提取媒体类型和base64数据
func ParseDataURL(dataURL string) (mediaType, base64Data string, err error) {
	// data URL格式：data:[<mediatype>][;base64],<data>
	dataURLPattern := regexp.MustCompile(`^data:([^;,]+)(;base64)?,(.+)$`)

	matches := dataURLPattern.FindStringSubmatch(dataURL)
	if len(matches) != 4 {
		return "", "", fmt.Errorf("无效的data URL格式")
	}

	mediaType = matches[1]
	isBase64 := matches[2] == ";base64"
	data := matches[3]

	if !isBase64 {
		return "", "", fmt.Errorf("仅支持base64编码的data URL")
	}

	// 验证是否为支持的图片格式
	if !IsSupportedImageFormat(mediaType) {
		return "", "", fmt.Errorf("不支持的图片格式: %s", mediaType)
	}

	// 验证base64编码
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", "", fmt.Errorf("无效的base64编码: %v", err)
	}

	// 检查文件大小
	if len(decodedData) > MaxImageSize {
		return "", "", fmt.Errorf("图片数据过大: %d 字节，最大支持 %d 字节", len(decodedData), MaxImageSize)
	}

	// 验证图片格式与声明是否匹配
	detectedType, err := DetectImageFormat(decodedData)
	if err == nil && detectedType != mediaType {
		return "", "", fmt.Errorf("图片格式不匹配: 声明为 %s，实际为 %s", mediaType, detectedType)
	}

	return mediaType, data, nil
}

// GetImageDimensions 从图片二进制数据解析宽高
// 支持 PNG, JPEG, GIF, WebP, BMP 格式
func GetImageDimensions(data []byte) (width, height int, err error) {
	if len(data) < 12 {
		return 0, 0, fmt.Errorf("图片数据太小")
	}

	// 检测格式并解析尺寸
	mediaType, err := DetectImageFormat(data)
	if err != nil {
		return 0, 0, err
	}

	switch mediaType {
	case "image/png":
		return getPNGDimensions(data)
	case "image/jpeg":
		return getJPEGDimensions(data)
	case "image/gif":
		return getGIFDimensions(data)
	case "image/webp":
		return getWebPDimensions(data)
	case "image/bmp":
		return getBMPDimensions(data)
	default:
		return 0, 0, fmt.Errorf("不支持的图片格式: %s", mediaType)
	}
}

// getPNGDimensions 解析 PNG 图片尺寸
// PNG 格式: 宽高在 IHDR chunk 中，位于第 16-23 字节 (big-endian)
func getPNGDimensions(data []byte) (width, height int, err error) {
	if len(data) < 24 {
		return 0, 0, fmt.Errorf("PNG 数据不完整")
	}
	width = int(binary.BigEndian.Uint32(data[16:20]))
	height = int(binary.BigEndian.Uint32(data[20:24]))
	return width, height, nil
}

// getJPEGDimensions 解析 JPEG 图片尺寸
// JPEG 格式: 需要查找 SOF (Start of Frame) 标记
func getJPEGDimensions(data []byte) (width, height int, err error) {
	if len(data) < 2 {
		return 0, 0, fmt.Errorf("JPEG 数据不完整")
	}

	// 跳过 SOI 标记 (0xFFD8)
	offset := 2

	for offset < len(data)-1 {
		// 查找标记
		if data[offset] != 0xFF {
			offset++
			continue
		}

		marker := data[offset+1]
		offset += 2

		// SOF0-SOF15 (排除 SOF4, SOF8, SOF12 等非帧标记)
		isSOF := (marker >= 0xC0 && marker <= 0xC3) ||
			(marker >= 0xC5 && marker <= 0xC7) ||
			(marker >= 0xC9 && marker <= 0xCB) ||
			(marker >= 0xCD && marker <= 0xCF)

		if isSOF {
			if offset+7 > len(data) {
				return 0, 0, fmt.Errorf("JPEG SOF 段数据不完整")
			}
			// SOF 结构: length(2) + precision(1) + height(2) + width(2)
			height = int(binary.BigEndian.Uint16(data[offset+3 : offset+5]))
			width = int(binary.BigEndian.Uint16(data[offset+5 : offset+7]))
			return width, height, nil
		}

		// 跳过其他段
		if marker == 0xD8 || marker == 0xD9 || (marker >= 0xD0 && marker <= 0xD7) {
			// SOI, EOI, RST 标记没有长度字段
			continue
		}

		if offset+2 > len(data) {
			break
		}
		segmentLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += segmentLen
	}

	return 0, 0, fmt.Errorf("未找到 JPEG SOF 标记")
}

// getGIFDimensions 解析 GIF 图片尺寸
// GIF 格式: 宽高在第 6-9 字节 (little-endian)
func getGIFDimensions(data []byte) (width, height int, err error) {
	if len(data) < 10 {
		return 0, 0, fmt.Errorf("GIF 数据不完整")
	}
	width = int(binary.LittleEndian.Uint16(data[6:8]))
	height = int(binary.LittleEndian.Uint16(data[8:10]))
	return width, height, nil
}

// getWebPDimensions 解析 WebP 图片尺寸
// WebP 格式: 需要解析 VP8/VP8L/VP8X chunk
func getWebPDimensions(data []byte) (width, height int, err error) {
	if len(data) < 30 {
		return 0, 0, fmt.Errorf("WebP 数据不完整")
	}

	// WebP 文件结构: RIFF(4) + size(4) + WEBP(4) + chunk...
	chunkType := string(data[12:16])

	switch chunkType {
	case "VP8 ":
		// 有损 WebP
		if len(data) < 30 {
			return 0, 0, fmt.Errorf("VP8 数据不完整")
		}
		w := binary.LittleEndian.Uint16(data[26:28])
		h := binary.LittleEndian.Uint16(data[28:30])
		width = int(w & 0x3FFF)
		height = int(h & 0x3FFF)
		return width, height, nil

	case "VP8L":
		// 无损 WebP
		if len(data) < 25 {
			return 0, 0, fmt.Errorf("VP8L 数据不完整")
		}
		bits := binary.LittleEndian.Uint32(data[21:25])
		width = int(bits&0x3FFF) + 1
		height = int((bits>>14)&0x3FFF) + 1
		return width, height, nil

	case "VP8X":
		// 扩展 WebP
		if len(data) < 30 {
			return 0, 0, fmt.Errorf("VP8X 数据不完整")
		}
		w := uint32(data[24]) | uint32(data[25])<<8 | uint32(data[26])<<16
		h := uint32(data[27]) | uint32(data[28])<<8 | uint32(data[29])<<16
		width = int(w) + 1
		height = int(h) + 1
		return width, height, nil

	default:
		return 0, 0, fmt.Errorf("未知的 WebP chunk 类型: %s", chunkType)
	}
}

// getBMPDimensions 解析 BMP 图片尺寸
// BMP 格式: 宽高在 DIB header 中，位于第 18-25 字节 (little-endian, signed)
func getBMPDimensions(data []byte) (width, height int, err error) {
	if len(data) < 26 {
		return 0, 0, fmt.Errorf("BMP 数据不完整")
	}
	width = int(int32(binary.LittleEndian.Uint32(data[18:22])))
	h := int32(binary.LittleEndian.Uint32(data[22:26]))
	// BMP 高度可以是负数（表示自上而下存储），取绝对值
	if h < 0 {
		height = int(-h)
	} else {
		height = int(h)
	}
	return width, height, nil
}

// EstimateImageTokens 根据图片分辨率计算 token 数量
// 遵循 Anthropic 官方计算规则:
// 1. 长边最大 1568px，超过则等比缩放
// 2. tokens = (width * height) / 750
// 3. 最小 85 tokens
func EstimateImageTokens(width, height int) int {
	if width <= 0 || height <= 0 {
		return 1500 // 无法获取尺寸时使用默认值
	}

	// 应用缩放规则：长边最大 1568px
	maxDim := 1568
	if width > maxDim || height > maxDim {
		if width > height {
			scale := float64(maxDim) / float64(width)
			width = maxDim
			height = int(float64(height) * scale)
		} else {
			scale := float64(maxDim) / float64(height)
			height = maxDim
			width = int(float64(width) * scale)
		}
	}

	// 官方公式: tokens = (width * height) / 750
	tokens := (width * height) / 750

	// 最小值保护
	if tokens < 85 {
		tokens = 85
	}

	return tokens
}

// EstimateImageTokensFromBase64 从 base64 编码的图片数据计算 token 数量
func EstimateImageTokensFromBase64(base64Data string) int {
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return 1500 // 解码失败使用默认值
	}
	return EstimateImageTokensFromBytes(data)
}

// EstimateImageTokensFromBytes 从已解码的图片字节数据计算 token 数量
// 当已有解码数据时使用此函数，避免重复 base64 解码
func EstimateImageTokensFromBytes(data []byte) int {
	width, height, err := GetImageDimensions(data)
	if err != nil {
		return 1500 // 解析失败使用默认值
	}
	return EstimateImageTokens(width, height)
}

// ConvertImageURLToImageSource 将 image_url 格式转换为 Anthropic 的 ImageSource 格式
func ConvertImageURLToImageSource(imageURL map[string]any) (*types.ImageSource, error) {
	urlValue, exists := imageURL["url"]
	if !exists {
		return nil, fmt.Errorf("image_url缺少url字段")
	}

	urlStr, ok := urlValue.(string)
	if !ok {
		return nil, fmt.Errorf("image_url的url字段必须是字符串")
	}

	if !strings.HasPrefix(urlStr, "data:") {
		return nil, fmt.Errorf("目前仅支持data URL格式的图片")
	}

	mediaType, base64Data, err := ParseDataURL(urlStr)
	if err != nil {
		return nil, fmt.Errorf("解析data URL失败: %v", err)
	}

	return &types.ImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      base64Data,
	}, nil
}

// EstimateDocumentTokensFromBase64 从 base64 编码的 PDF 数据估算 token 数量
// 根据 Anthropic 官方文档：
// - 文本 token: 每页约 1,500-3,000 tokens
// - 图片 token: 每页作为图片处理
// - 假设 PDF 平均每页约 100KB，每页约 3000 tokens
// - 估算公式: tokens ≈ 原始字节数 / 35
// - 最大 100 页 (API 限制)，最大约 300,000 tokens
func EstimateDocumentTokensFromBase64(base64Data string) int {
	if base64Data == "" {
		return 500 // 空数据使用默认值
	}

	// base64 编码后大小约为原始大小的 4/3
	// 原始大小 ≈ base64长度 * 3 / 4
	originalSize := len(base64Data) * 3 / 4

	// 估算页数: 假设每页约 100KB
	pageCount := originalSize / (100 * 1024)
	if pageCount < 1 {
		pageCount = 1
	}
	// 最大 100 页 (API 限制)
	if pageCount > 100 {
		pageCount = 100
	}

	// 每页约 3000 tokens (文本 2250 + 图片 750)
	tokens := pageCount * 3000

	// 最小值保护
	if tokens < 500 {
		tokens = 500
	}

	return tokens
}
