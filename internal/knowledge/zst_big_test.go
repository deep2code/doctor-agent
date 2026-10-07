package knowledge

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// 验证种子树里最大的几个归档真的能被 Go 端解压并解析成行。
//
// TestBakeClassification 出于速度跳过 >5MB 的归档，本测试正好补上那一头：名单不是
// 手工登记的，而是按压缩后体积从 gz 树里取前几名，所以数据增减不需要改这个文件
// （手工点名曾经在这里留下两个已删除的问答语料）。
//
// 解压并 json 解码大语料是包里的固定开销：实测 2026-10-02，
// `go test ./internal/knowledge -short` 742s、不带 -short 959s，即本测试约 3.6 分钟。
// 数据批次只需要召回门，所以 `-short` 跳过它，CI 全量跑（不带 -short）仍会执行。
func TestBakeBigCorporaZST(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: 跳过大语料解压与分类（数据批次的召回门不需要它）")
	}

	archives, err := listSeedArchives("gz")
	if err != nil {
		t.Fatalf("listing the gz tree: %v", err)
	}
	sizes := map[string]int64{}
	for _, a := range archives {
		st, err := os.Stat(a.Path)
		if err != nil {
			t.Fatalf("stat %s: %v", a.Path, err)
		}
		sizes[a.Path] = st.Size()
	}
	slices.SortStableFunc(archives, func(i, j seedArchive) int {
		return int(sizes[j.Path] - sizes[i.Path])
	})
	if len(archives) < 3 {
		t.Fatalf("expected >=3 archives, got %d", len(archives))
	}

	largest := 0
	for _, a := range archives[:3] {
		raw, err := decompressFile(a.Path)
		if err != nil {
			t.Fatalf("%s/%s 解压失败: %v", a.Dataset, a.Base, err)
		}
		if strings.HasPrefix(string(raw), "version https://git-lfs") {
			t.Fatalf("%s/%s 是 LFS 指针，不是数据", a.Dataset, a.Base)
		}
		if len(raw) > largest {
			largest = len(raw)
		}
		rows, err := seedList(raw)
		if err != nil {
			t.Fatalf("%s/%s 解析失败: %v", a.Dataset, a.Base, err)
		}
		if len(rows) == 0 {
			t.Errorf("%s/%s: 0 rows", a.Dataset, a.Base)
		}
		t.Logf("%s/%s: 压缩 %.1fMB → 解压 %.1fMB, rows=%d", a.Dataset, a.Base,
			float64(sizes[a.Path])/1e6, float64(len(raw))/1e6, len(rows))
	}
	// 至少有一个解压产物 >100MB：这才是本测试的意义（Go 端吃得下真正的大语料）。
	// 数据缩到没有 >100MB 的归档时这条会红，提示该测试已失去存在理由。
	if largest < 100_000_000 {
		t.Errorf("最大的归档解压后只有 %d bytes，本测试（大语料解压）已无意义", largest)
	}
}
