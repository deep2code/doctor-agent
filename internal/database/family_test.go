package database

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// openTestDB 连接被测 MariaDB：参数取自标准 MARIA_DB_* 环境变量（CI 的 service
// 容器用 3306；本地跑 `MARIA_DB_PORT=3307 go test ./...`），连不上则跳过。
func openTestDB(t *testing.T) *DB {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User = envOr("MARIA_DB_USER", "root")
	cfg.Passwd = os.Getenv("MARIA_DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%s", envOr("MARIA_DB_HOST", "127.0.0.1"), envOr("MARIA_DB_PORT", "3307"))
	cfg.DBName = envOr("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge")
	cfg.ParseTime = true
	cfg.InterpolateParams = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	db, err := New(Config{DSN: cfg.FormatDSN()})
	if err != nil {
		t.Skipf("test MariaDB (%s) unavailable: %v", cfg.Addr, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestFamilyMemberCRUD(t *testing.T) {
	db := openTestDB(t)

	m := &FamilyMember{
		Name: "爸爸", Relation: "父亲", BirthYear: 1958, Gender: "男",
		Region: "广西",
		Conditions: "高血压、2型糖尿病",
		Allergies:  "青霉素",
		Medications: "氨氯地平 5mg qd\n二甲双胍 0.5g bid",
	}
	if err := db.CreateFamilyMember(m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("id not set")
	}

	got, err := db.GetFamilyMember(m.ID, "")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "爸爸" || got.Region != "广西" || got.Allergies != "青霉素" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// 更新部分字段
	if err := db.UpdateFamilyMember(&FamilyMember{ID: m.ID, WeightKg: 70.5, Conditions: "高血压"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = db.GetFamilyMember(m.ID, "")
	if got.WeightKg != 70.5 || got.Conditions != "高血压" {
		t.Errorf("update mismatch: %+v", got)
	}
	if got.Medications == "" {
		t.Errorf("空字段不应被清空")
	}

	list, err := db.ListFamilyMembers("")
	if err != nil || len(list) == 0 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	if err := db.DeleteFamilyMember(m.ID, ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.GetFamilyMember(m.ID, ""); err == nil {
		t.Fatal("deleted member still readable")
	}
}
