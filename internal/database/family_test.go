package database

import (
	"database/sql"
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

// testAppDBName is the scratch schema these tests own. It must never be the
// knowledge database: internal/database creates the *application* tables
// (users, sessions, family_members, …) on connect, and seeding them into
// doctor_knowledge leaks app schema into the doctor-agent-kb data image, which
// is published to every deployment (see update-kb.sh's guard).
const testAppDBName = "doctor_agent_test_db"

// openTestDB 连接被测 MariaDB：参数取自标准 MARIA_DB_* 环境变量（CI 的 service
// 容器用 3306；本地跑 `MARIA_DB_PORT=3307 go test ./...`），连不上则跳过。
func openTestDB(t *testing.T) *DB {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User = envOr("MARIA_DB_USER", "root")
	cfg.Passwd = os.Getenv("MARIA_DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%s", envOr("MARIA_DB_HOST", "127.0.0.1"), envOr("MARIA_DB_PORT", "3307"))
	cfg.DBName = envOr("MARIA_DB_APP_DB", testAppDBName)
	if cfg.DBName == envOr("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge") {
		// An operator who points both databases at the same name would recreate
		// the pollution this guard exists to prevent; prefer an explicit skip.
		t.Skipf("MARIA_DB_APP_DB must differ from the knowledge db (%q)", cfg.DBName)
	}
	cfg.ParseTime = true
	cfg.InterpolateParams = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}

	// Create the scratch schema before New() migrates into it — the DSN cannot
	// select a database that does not exist yet.
	serverCfg := *cfg
	serverCfg.DBName = ""
	server, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("test MariaDB (%s) unavailable: %v", cfg.Addr, err)
	}
	defer server.Close()
	if _, err := server.Exec("CREATE DATABASE IF NOT EXISTS `" + cfg.DBName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create test db %s: %v", cfg.DBName, err)
	}

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

// 多用户隔离：一个账号看不到、改不动、删不掉另一个账号的档案。尤其是
// userID 为空的那个「匿名」档 —— 早先它被当成「不校验」的超级用户，
// 于是任何未登录请求都能凭自增 id 读到别人全家的病史。
func TestFamilyMemberUserIsolation(t *testing.T) {
	db := openTestDB(t)

	own := &FamilyMember{UserID: "user-a", Name: "妈妈", Conditions: "高血压"}
	if err := db.CreateFamilyMember(own); err != nil {
		t.Fatalf("create owned: %v", err)
	}
	t.Cleanup(func() { _ = db.DeleteFamilyMember(own.ID, "user-a") })

	anon := &FamilyMember{Name: "邻居"} // 无主档案
	if err := db.CreateFamilyMember(anon); err != nil {
		t.Fatalf("create anonymous: %v", err)
	}
	t.Cleanup(func() { _ = db.DeleteFamilyMember(anon.ID, "") })

	for name, userID := range map[string]string{
		"another user": "user-b",
		"anonymous":    "",
	} {
		if _, err := db.GetFamilyMember(own.ID, userID); err == nil {
			t.Errorf("%s can read user-a's member", name)
		}
		if err := db.DeleteFamilyMember(own.ID, userID); err == nil {
			t.Errorf("%s can delete user-a's member", name)
		}
		if err := db.UpdateFamilyMember(&FamilyMember{ID: own.ID, UserID: userID, Notes: "越权"}); err != nil {
			t.Fatalf("update by %s: %v", name, err)
		}
		got, err := db.GetFamilyMember(own.ID, "user-a")
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Notes != "" {
			t.Errorf("%s modified user-a's member: notes=%q", name, got.Notes)
		}
	}

	list, err := db.ListFamilyMembers("user-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range list {
		if m.ID == anon.ID {
			t.Error("user-a's list contains an anonymous member")
		}
	}
	anonList, err := db.ListFamilyMembers("")
	if err != nil {
		t.Fatalf("list anonymous: %v", err)
	}
	for _, m := range anonList {
		if m.ID == own.ID {
			t.Error("anonymous list contains user-a's member")
		}
	}
}
