package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/doctor-agent/internal/database"
	"github.com/doctor-agent/internal/session"
)

// Family health profiles (/family): 本地家庭档案 CRUD, 问答时通过
// ChatRequest.member_id 把成员背景注入会话的 PatientContext。
//
// 档案是最敏感的一类数据（一家人的基础病、过敏与长期用药），所以每个读写
// 都以调用方归属为过滤条件；未登录时归属为空，即原本的单用户本地行为。

func (s *Server) handleFamily(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "database disabled"})
		return
	}
	owner := s.ownerOf(r)
	switch r.Method {
	case http.MethodGet:
		members, err := s.db.ListFamilyMembers(owner)
		if err != nil {
			slog.Error("Listing family members", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to list members"})
			return
		}
		if members == nil {
			members = []*database.FamilyMember{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"members": members})

	case http.MethodPost:
		var m database.FamilyMember
		if !decodeJSONBody(w, r, jsonBodyLimit, &m) {
			return
		}
		// 归属只认凭证，不认请求体：否则客户端可以往别人的档案里插成员。
		m.UserID = owner
		m.Name = strings.TrimSpace(m.Name)
		if m.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		if err := s.db.CreateFamilyMember(&m); err != nil {
			slog.Error("Creating family member", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create member"})
			return
		}
		writeJSON(w, http.StatusCreated, m)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func (s *Server) handleFamilyByID(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "database disabled"})
		return
	}
	idStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/family/"), "/")
	if idStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "member id required"})
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid member id"})
		return
	}
	owner := s.ownerOf(r)

	switch r.Method {
	case http.MethodGet:
		m, err := s.db.GetFamilyMember(id, owner)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "member not found"})
			return
		}
		writeJSON(w, http.StatusOK, m)

	case http.MethodPut, http.MethodPatch:
		var m database.FamilyMember
		if !decodeJSONBody(w, r, jsonBodyLimit, &m) {
			return
		}
		m.ID = id
		// 归属由凭证决定：请求体里的 user_id 一律忽略，
		// 更新语句也会带上同一个归属条件。
		m.UserID = owner
		if err := s.db.UpdateFamilyMember(&m); err != nil {
			slog.Error("Updating family member", "id", id, "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to update member"})
			return
		}
		out, err := s.db.GetFamilyMember(id, owner)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "member not found"})
			return
		}
		writeJSON(w, http.StatusOK, out)

	case http.MethodDelete:
		if err := s.db.DeleteFamilyMember(id, owner); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "member not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// applyFamilyMember 把家庭成员档案写入会话 PatientContext，使本次问答
// 自动携带「谁在问 / 基础病 / 过敏 / 长期用药」。owner 是调用方的归属，
// 档案不属于他就当作不存在。返回描述性错误或 nil。
func (s *Server) applyFamilyMember(sess *session.Session, memberID int64, owner string) error {
	m, err := s.db.GetFamilyMember(memberID, owner)
	if err != nil {
		return fmt.Errorf("member %d not found", memberID)
	}
	pc := sess.GetPatientContext()
	if pc == nil {
		pc = &session.PatientContext{}
	}
	pc.Region = m.Region
	if m.Conditions != "" {
		pc.KnownConditions = splitLines(m.Conditions)
	}
	if m.Allergies != "" {
		pc.KnownAllergies = splitLines(m.Allergies)
	}
	if m.Medications != "" {
		pc.CurrentMedications = splitLines(m.Medications)
	}
	pc.ProfileSummary = familySummary(m)
	sess.SetPatientContext(pc)
	return nil
}

// familySummary 把档案压成一行背景描述。
func familySummary(m *database.FamilyMember) string {
	parts := []string{"提问对象: " + m.Name}
	if m.Relation != "" && m.Relation != m.Name {
		parts[0] += "（" + m.Relation + "）"
	}
	if m.BirthYear > 0 {
		parts = append(parts, fmt.Sprintf("出生年份 %d", m.BirthYear))
	}
	if m.Gender != "" {
		parts = append(parts, m.Gender)
	}
	if m.Medications != "" {
		parts = append(parts, "长期用药: "+strings.ReplaceAll(m.Medications, "\n", "；"))
	}
	if m.Notes != "" {
		parts = append(parts, strings.ReplaceAll(m.Notes, "\n", "；"))
	}
	return strings.Join(parts, "，")
}

// splitLines 按行/分号/顿号切分为列表。
func splitLines(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == ';' || r == '；' || r == '、' || r == ','
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
