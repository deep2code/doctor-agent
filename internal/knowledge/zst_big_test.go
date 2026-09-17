package knowledge

import (
	"path/filepath"
	"testing"
)

// 直接验证两个大语料的 zst 能被 Go 端解压并分类（绕过 >5MB 跳过）
func TestBakeBigCorporaZST(t *testing.T) {
	for _, name := range []string{"huatuo_qa.json", "medical_qa_pairs.json"} {
		raw, err := decompressFile(filepath.Join("gz", name+".zst"))
		if err != nil {
			t.Fatalf("%s 解压失败: %v", name, err)
		}
		if len(raw) < 100_000_000 {
			t.Errorf("%s 解压后过小 (%d bytes)，可能仍是 LFS 指针", name, len(raw))
		}
		ds, rows, err := seedFile(name, raw)
		if err != nil {
			t.Fatalf("%s 分类失败: %v", name, err)
		}
		t.Logf("%s: dataset=%s rows=%d 解压后 %.1fMB", name, ds, len(rows), float64(len(raw))/1e6)
	}
}
