package database

import (
	"testing"
)

// openTestDB 返回本地测试库连接（127.0.0.1:3307, doctor-kb-test 容器或
// 任意空密码 MariaDB）；不可用则跳过。
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(Config{DSN: "root@tcp(127.0.0.1:3307)/doctor_knowledge?parseTime=true&interpolateParams=true&charset=utf8mb4"})
	if err != nil {
		t.Skipf("local test MariaDB (3307) unavailable: %v", err)
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
