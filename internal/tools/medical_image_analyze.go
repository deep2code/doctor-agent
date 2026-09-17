package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/doctor-agent/internal/llm"
)

// MedicalImageAnalyze provides medical image analysis.
type MedicalImageAnalyze struct {
	provider llm.LLMProvider
}

// NewMedicalImageAnalyze creates a new medical image analysis tool.
func NewMedicalImageAnalyze(provider llm.LLMProvider) *MedicalImageAnalyze {
	return &MedicalImageAnalyze{provider: provider}
}

func (t *MedicalImageAnalyze) Name() string {
	return "medical_image_analyze"
}

func (t *MedicalImageAnalyze) Description() string {
	return "分析医学影像(X光、CT、MRI、超声等)并提供专业解读"
}

func (t *MedicalImageAnalyze) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"image_base64": map[string]any{
				"type":        "string",
				"description": "Base64编码的医学影像图片",
			},
			"image_type": map[string]any{
				"type":        "string",
				"description": "影像类型: xray, ct, mri, ultrasound, ecg, pathology",
				"enum":        []string{"xray", "ct", "mri", "ultrasound", "ecg", "pathology"},
			},
			"body_part": map[string]any{
				"type":        "string",
				"description": "检查部位",
			},
			"clinical_context": map[string]any{
				"type":        "string",
				"description": "临床背景信息",
			},
		},
		"required": []string{"image_base64", "image_type"},
	}
}

func (t *MedicalImageAnalyze) Execute(ctx context.Context, params map[string]any) (*ToolResult, error) {
	imageBase64, _ := params["image_base64"].(string)
	imageType, _ := params["image_type"].(string)
	bodyPart, _ := params["body_part"].(string)
	clinicalContext, _ := params["clinical_context"].(string)

	if imageBase64 == "" {
		return &ToolResult{
			Success: false,
			Error:   "请提供医学影像图片",
		}, nil
	}

	// Validate base64 and detect media type in one decode pass
	data, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   "无效的Base64编码图片",
		}, nil
	}

	// Detect media type from decoded bytes
	mediaType := detectMediaTypeFromBytes(data)

	// Build analysis prompt
	prompt := t.buildAnalysisPrompt(imageType, bodyPart, clinicalContext)

	// Create multimodal message
	msg := llm.Message{
		Role: "user",
		Parts: []llm.ContentPart{
			{
				Type: "text",
				Text: prompt,
			},
			{
				Type: "image",
				Image: &llm.ImageInput{
					Base64Data: imageBase64,
					MediaType:  mediaType,
				},
			},
		},
	}

	// Call LLM for analysis
	response, err := t.provider.Chat(ctx, []llm.Message{msg}, nil, medicalImageSystemPrompt)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("影像分析失败: %v", err),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"analysis": response.Text,
		},
		Citations: []CitationRef{
			{
				ID:    "medical_image_analyze",
				Title: "AI医学影像分析",
			},
		},
	}, nil
}

func (t *MedicalImageAnalyze) buildAnalysisPrompt(imageType, bodyPart, clinicalContext string) string {
	var sb strings.Builder

	sb.WriteString("请分析这张医学影像：\n\n")

	if bodyPart != "" {
		fmt.Fprintf(&sb, "检查部位: %s\n", bodyPart)
	}

	if clinicalContext != "" {
		fmt.Fprintf(&sb, "临床背景: %s\n", clinicalContext)
	}

	sb.WriteString("\n请提供以下分析：\n")
	sb.WriteString("1. 影像质量评估\n")
	sb.WriteString("2. 主要发现（异常征象）\n")
	sb.WriteString("3. 可能的诊断\n")
	sb.WriteString("4. 建议的进一步检查\n")
	sb.WriteString("5. 注意事项\n")

	return sb.String()
}

func detectMediaTypeFromBytes(data []byte) string {
	if len(data) < 4 {
		return "image/jpeg"
	}

	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "image/jpeg"
	}

	// PNG: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "image/png"
	}

	// GIF: 47 49 46 38
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return "image/gif"
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(data) >= 12 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp"
	}

	return "image/jpeg"
}

const medicalImageSystemPrompt = `你是一位专业的医学影像诊断专家，具有丰富的放射科临床经验。

请遵循以下原则：
1. 基于影像特征进行客观分析
2. 提出可能的诊断，按可能性排序
3. 建议进一步检查以明确诊断
4. 指出需要紧急处理的情况
5. 使用专业但易于理解的语言

注意：
- 影像诊断需结合临床资料
- 本分析仅供参考，不能替代医生面诊
- 如有紧急情况，请立即就医`
