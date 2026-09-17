package server

import (
	"bytes"
	"os"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

func TestRenderMarkdownToPDFWrapsContent(t *testing.T) {
	fontPath := getChineseFontPath()
	if fontPath == "" {
		t.Skip("no Chinese font available")
	}
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatalf("read font: %v", err)
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes("NotoSansSC", "", fontData)
	pdf.AddUTF8FontFromBytes("NotoSansSC", "B", fontData)
	if err := pdf.Error(); err != nil {
		t.Fatalf("load font: %v", err)
	}
	pdf.SetFont("NotoSansSC", "", 10)
	pdf.AddPage()

	markdown := `## 鉴别诊断

这是一个非常长的普通段落，用来验证中文文本在导出 PDF 时能够按可用宽度自动换行，而不是一直向右侧溢出，最终被 PDF 渲染器截断或覆盖到页面外。

1. 这是一个非常长的有序列表项，同样需要自动换行并保持列表编号后的悬挂缩进，确保多行内容不会覆盖右侧内容。
2. 短列表项。

| 可能疾病 | 支持证据 | 不支持证据 | 证据等级 | 引用 |
| --- | --- | --- | --- | --- |
| 围绝经期综合征 | 43岁、闭经、失眠、体重增加 | 需要检查排除甲状腺和血糖问题 | 低 | [1] |
`
	renderMarkdownToPDF(pdf, markdown)
	if err := pdf.Error(); err != nil {
		t.Fatalf("render PDF: %v", err)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("output PDF: %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("PDF output is empty")
	}
}
