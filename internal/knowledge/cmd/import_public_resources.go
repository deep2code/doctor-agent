// +build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/doctor-agent/internal/knowledge"
)

func main() {
	dsn := flag.String("dsn", "root:@tcp(localhost:3306)/doctor_knowledge?parseTime=true&interpolateParams=true", "MySQL DSN")
	flag.Parse()

	// 读取资源文件
	data, err := os.ReadFile("data/public_resources.json")
	if err != nil {
		log.Fatalf("读取资源文件失败: %v", err)
	}

	var doc struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		log.Fatalf("解析JSON失败: %v", err)
	}

	// 打开知识库
	kb, err := knowledge.OpenKB(*dsn)
	if err != nil {
		log.Fatalf("打开知识库失败: %v", err)
	}
	defer kb.Close()

	// 导入资源
	for _, r := range doc.Resources {
		rJSON, err := json.Marshal(r)
		if err != nil {
			log.Printf("序列化资源 %s 失败: %v", r.ID, err)
			continue
		}
		if err := kb.Insert(knowledge.DSPublicResources, r.ID, rJSON); err != nil {
			log.Printf("导入资源 %s 失败: %v", r.ID, err)
			continue
		}
		fmt.Printf("✓ 导入: %s (%s)\n", r.NameZH, r.Category)
	}

	fmt.Printf("\n共导入 %d 个公共医学资源\n", len(doc.Resources))
}

type Resource struct {
	ID             string   `json:"id"`
	NameZH         string   `json:"name_zh"`
	NameEN         string   `json:"name_en"`
	Category       string   `json:"category"`
	DescriptionZH  string   `json:"description_zh"`
	DescriptionEN  string   `json:"description_en"`
	URL            string   `json:"url"`
	Keywords       []string `json:"keywords"`
}