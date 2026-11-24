// Package models 讀神經模型的上游清單（models.lock.json）與等價雜湊（equivalent.json，#180），
// 判斷安裝包內建的模型是不是 Hugging Face 上的那一份（設定程式「模型」頁顯示的來源與授權）。
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// 兩個模型的角色，依這個順序顯示。
const (
	RoleSelect = "select"
	RolePunct  = "punct"
)

// Roles 是兩個模型的角色。
var Roles = []string{RoleSelect, RolePunct}

// Label 是角色的中文名稱。
func Label(role string) string {
	if role == RolePunct {
		return "補標點"
	}
	return "選字"
}

// File 是一個檔案的大小與 SHA-256。
type File struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Model 是 models.lock 裡的一個上游模型。
type Model struct {
	Repository string          `json:"repository"`
	Revision   string          `json:"revision"`
	Version    string          `json:"version"`
	License    string          `json:"license"`
	Files      map[string]File `json:"files"`
}

// URL 是 Hugging Face 上這個 revision 的頁面（設定程式顯示的連結）。
func (m Model) URL() string {
	return "https://huggingface.co/" + m.Repository + "/tree/" + m.Revision
}

// Lock 是 models.lock.json。SHA256 是原始檔案的雜湊（出貨的 pairing.lock 記的 models_lock_sha256）。
type Lock struct {
	Schema int              `json:"schema"`
	Models map[string]Model `json:"models"`
	SHA256 string           `json:"-"`
}

// 發版配對（scripts/pair_models.py）只改這兩個檔案，models.lock 一定要記著。
var pairedFiles = []string{"manifest.json", "vocab.json"}

func validHex(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 64 && err == nil && s == strings.ToLower(s)
}

// safeName：檔名只能是單純的名稱（models.lock 來自編進程式的清單，這裡仍不讓它指到別的目錄）。
func safeName(n string) bool {
	if n == "" || n == "." || n == ".." {
		return false
	}
	for _, c := range n {
		if c == '/' || c == '\\' || c == 0 {
			return false
		}
	}
	return true
}

// ParseLock 讀 models.lock.json，並核對兩個模型都在、欄位齊全。
func ParseLock(b []byte) (Lock, error) {
	var l Lock
	if err := json.Unmarshal(b, &l); err != nil {
		return Lock{}, fmt.Errorf("models.lock.json：%w", err)
	}
	if l.Schema != 1 || len(l.Models) != len(Roles) {
		return Lock{}, errors.New("models.lock.json：格式不符")
	}
	for _, role := range Roles {
		m, ok := l.Models[role]
		if !ok || m.Repository == "" || !validRevision(m.Revision) {
			return Lock{}, fmt.Errorf("models.lock.json：缺少 %s", role)
		}
		for _, n := range append([]string{"model.onnx"}, pairedFiles...) {
			if _, ok := m.Files[n]; !ok {
				return Lock{}, fmt.Errorf("models.lock.json：%s 缺少 %s", role, n)
			}
		}
		for n, f := range m.Files {
			if !safeName(n) || !validHex(f.SHA256) || f.Size < 0 {
				return Lock{}, fmt.Errorf("models.lock.json：%s/%s 不正確", role, n)
			}
		}
	}
	sum := sha256.Sum256(b)
	l.SHA256 = hex.EncodeToString(sum[:])
	return l, nil
}

// validRevision：Hugging Face 的 revision 是提交的雜湊（小寫十六進位）。
func validRevision(r string) bool {
	if len(r) < 7 || len(r) > 64 {
		return false
	}
	for _, c := range r {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// 本機 model.onnx 與 models.lock 的關係（Lock.Match）。
const (
	MatchNone       = ""
	MatchUpstream   = "upstream"   // 就是 models.lock 那一份
	MatchEquivalent = "equivalent" // equivalent.json 記的等價匯出：權重相同、只差匯出資訊
)

// Equivalents 是 models/equivalent.json：與 models.lock 某個 revision 的 model.onnx 等價的其他雜湊。
type Equivalents struct {
	Schema int                        `json:"schema"`
	Models map[string]EquivalentModel `json:"models"`
}

// EquivalentModel 只在 models.lock 的 repository、revision 與 model.onnx 都跟這裡一樣時成立。
type EquivalentModel struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Model      string `json:"model.onnx"`
	Equivalent []File `json:"equivalent"`
}

// ParseEquivalents 讀 equivalent.json。
func ParseEquivalents(b []byte) (Equivalents, error) {
	var e Equivalents
	if err := json.Unmarshal(b, &e); err != nil {
		return Equivalents{}, fmt.Errorf("equivalent.json：%w", err)
	}
	if e.Schema != 1 {
		return Equivalents{}, errors.New("equivalent.json：格式不符")
	}
	for role, m := range e.Models {
		if !validHex(m.Model) || !validRevision(m.Revision) || m.Repository == "" {
			return Equivalents{}, fmt.Errorf("equivalent.json：%s 不正確", role)
		}
		for _, f := range m.Equivalent {
			if !validHex(f.SHA256) || f.Size <= 0 {
				return Equivalents{}, fmt.Errorf("equivalent.json：%s 的等價雜湊不正確", role)
			}
		}
	}
	return e, nil
}

// Match 回報 sha（本機模型 manifest 記的 model.onnx 雜湊）是不是 models.lock 裡 role 的上游模型。
func (l Lock) Match(role, sha string, eq Equivalents) string {
	m, ok := l.Models[role]
	if !ok || sha == "" {
		return MatchNone
	}
	want := m.Files["model.onnx"].SHA256
	if sha == want {
		return MatchUpstream
	}
	e, ok := eq.Models[role]
	if !ok || e.Repository != m.Repository || e.Revision != m.Revision || e.Model != want {
		return MatchNone
	}
	for _, f := range e.Equivalent {
		if f.SHA256 == sha {
			return MatchEquivalent
		}
	}
	return MatchNone
}
