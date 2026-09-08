package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FamilyMember 是一条家庭健康档案：本人或家人（孩子/老人等）的基础健康背景，
// 用于问答时自动带上「谁在问、有什么基础病、用什么药、对什么过敏」。
// 注意：档案是用户自行填写的背景信息，不存储身份证等实名隐私。
type FamilyMember struct {
	ID          int64     `json:"id"`
	UserID      string    `json:"user_id,omitempty"` // 预留多用户；本地模式为空
	Name        string    `json:"name"`              // 称呼，如「我」「爸爸」「小明」
	Relation    string    `json:"relation,omitempty"`
	BirthYear   int       `json:"birth_year,omitempty"`
	Gender      string    `json:"gender,omitempty"` // 男/女
	HeightCm    float64   `json:"height_cm,omitempty"`
	WeightKg   float64   `json:"weight_kg,omitempty"`
	Region      string    `json:"region,omitempty"`      // 居住/来源地区（地方病风险评估用）
	Conditions  string    `json:"conditions,omitempty"`  // 慢性病/既往史，顿号或换行分隔
	Allergies   string    `json:"allergies,omitempty"`   // 药物/食物过敏
	Medications string    `json:"medications,omitempty"` // 长期用药
	Notes       string    `json:"notes,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const familyColumns = `id, user_id, name, relation, birth_year, gender,
	height_cm, weight_kg, region, conditions, allergies, medications, notes,
	created_at, updated_at`

func scanFamilyMember(row interface{ Scan(...any) error }) (*FamilyMember, error) {
	var m FamilyMember
	var userID sql.NullString
	var relation sql.NullString
	var gender sql.NullString
	var region sql.NullString
	var conditions, allergies, medications, notes sql.NullString
	if err := row.Scan(&m.ID, &userID, &m.Name, &relation, &m.BirthYear, &gender,
		&m.HeightCm, &m.WeightKg, &region, &conditions, &allergies, &medications, &notes,
		&m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	m.UserID = userID.String
	m.Relation = relation.String
	m.Gender = gender.String
	m.Region = region.String
	m.Conditions = conditions.String
	m.Allergies = allergies.String
	m.Medications = medications.String
	m.Notes = notes.String
	return &m, nil
}

// CreateFamilyMember 新增成员。
func (db *DB) CreateFamilyMember(m *FamilyMember) error {
	res, err := db.conn.Exec(
		`INSERT INTO family_members
			(user_id, name, relation, birth_year, gender, height_cm, weight_kg,
			 region, conditions, allergies, medications, notes)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.UserID, m.Name, m.Relation, m.BirthYear, m.Gender, m.HeightCm, m.WeightKg,
		m.Region, m.Conditions, m.Allergies, m.Medications, m.Notes)
	if err != nil {
		return err
	}
	if m.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	return nil
}

// ListFamilyMembers 列出（当前用户的）全部成员。
func (db *DB) ListFamilyMembers(userID string) ([]*FamilyMember, error) {
	rows, err := db.conn.Query(
		`SELECT `+familyColumns+` FROM family_members
		 WHERE user_id = ? OR (user_id IS NULL AND ? = '')
		 ORDER BY id`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*FamilyMember
	for rows.Next() {
		m, err := scanFamilyMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetFamilyMember 按 id 取成员；userID 非空时校验归属。
func (db *DB) GetFamilyMember(id int64, userID string) (*FamilyMember, error) {
	row := db.conn.QueryRow(
		`SELECT `+familyColumns+` FROM family_members WHERE id = ?`, id)
	m, err := scanFamilyMember(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("member %d not found", id)
	}
	if err != nil {
		return nil, err
	}
	if userID != "" && m.UserID != userID {
		return nil, fmt.Errorf("member %d not found", id)
	}
	return m, nil
}

// UpdateFamilyMember 更新非空字段（空字符串保持不变，height/weight 以>0为准）。
func (db *DB) UpdateFamilyMember(m *FamilyMember) error {
	var sets []string
	var args []any
	add := func(col string, val any) { sets = append(sets, col+" = ?"); args = append(args, val) }
	if m.Name != "" {
		add("name", m.Name)
	}
	if m.Relation != "" {
		add("relation", m.Relation)
	}
	if m.BirthYear > 0 {
		add("birth_year", m.BirthYear)
	}
	if m.Gender != "" {
		add("gender", m.Gender)
	}
	if m.HeightCm > 0 {
		add("height_cm", m.HeightCm)
	}
	if m.WeightKg > 0 {
		add("weight_kg", m.WeightKg)
	}
	if m.Region != "" {
		add("region", m.Region)
	}
	if m.Conditions != "" {
		add("conditions", m.Conditions)
	}
	if m.Allergies != "" {
		add("allergies", m.Allergies)
	}
	if m.Medications != "" {
		add("medications", m.Medications)
	}
	if m.Notes != "" {
		add("notes", m.Notes)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, m.ID)
	_, err := db.conn.Exec(
		`UPDATE family_members SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return err
}

// DeleteFamilyMember 删除成员。
func (db *DB) DeleteFamilyMember(id int64, userID string) error {
	if userID != "" {
		res, err := db.conn.Exec(
			`DELETE FROM family_members WHERE id = ? AND user_id = ?`, id, userID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("member %d not found", id)
		}
		return nil
	}
	_, err := db.conn.Exec(`DELETE FROM family_members WHERE id = ?`, id)
	return err
}
