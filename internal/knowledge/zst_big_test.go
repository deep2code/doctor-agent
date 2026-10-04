package knowledge

import (
	"path/filepath"
	"testing"
)

// 直接验证两个大语料的 zst 能被 Go 端解压并分类（绕过 >5MB 跳过）
//
// 解压并 json 解码 140MB+327MB 种子是包里的固定开销：实测 2026-10-02，
// `go test ./internal/knowledge -short` 742s、不带 -short 959s，即本测试约 3.6 分钟。
// 数据批次只需要召回门，所以 `-short` 跳过它，CI 全量跑（不带 -short）仍会执行。
func TestBakeBigCorporaZST(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: 跳过大语料解压与分类（数据批次的召回门不需要它）")
	}
	for _, f := range []struct{ dataset, base string }{
		{DSHuatuo, "huatuo_qa.json"},
		{DSMedicalQA, "medical_qa_pairs.json"},
	} {
		path := filepath.Join("gz", f.dataset, f.base+".zst")
		raw, err := decompressFile(path)
		if err != nil {
			t.Fatalf("%s 解压失败: %v", path, err)
		}
		if len(raw) < 100_000_000 {
			t.Errorf("%s 解压后过小 (%d bytes)，可能仍是 LFS 指针", path, len(raw))
		}
		rows, err := seedList(raw)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", path, err)
		}
		t.Logf("%s: dataset=%s rows=%d 解压后 %.1fMB", path, f.dataset, len(rows), float64(len(raw))/1e6)
	}
}
