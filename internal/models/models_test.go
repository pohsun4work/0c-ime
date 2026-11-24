package models

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	upstream "github.com/0c-biggo/0c-ime/models"
)

func sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func upstreamFile(t *testing.T, role, name string) []byte {
	return read(t, filepath.Join("..", "..", "models", "upstream", role, name))
}

// 編進程式的 models.lock 與 0c-ime 的相同（pairing.lock 的 models_lock_sha256），上游 metadata 也對得上它。
func TestEmbeddedLockMatchesUpstream(t *testing.T) {
	l, err := ParseLock(upstream.Lock)
	if err != nil {
		t.Fatal(err)
	}
	if l.SHA256 != "84f5195065b5b4e3124d2af0e09c874b08a0be9eec0db354f61f7e3647adfdb7" {
		t.Fatalf("models.lock 雜湊 %s", l.SHA256)
	}
	if l.Models[RoleSelect].Files["model.onnx"].SHA256 != "4a80caf18acbaddde10c908dcbf35919908d616e94618cdde71fc3409c969261" ||
		l.Models[RolePunct].Files["model.onnx"].SHA256 != "4db3ed40df777347d9c455da01285b01de192d4c210c5f7cd9f0646c727880d8" {
		t.Fatal("models.lock 的權重雜湊不是 Hugging Face 公開版")
	}
	for _, role := range Roles {
		for _, n := range pairedFiles {
			b := upstreamFile(t, role, n)
			if f := l.Models[role].Files[n]; sum(b) != f.SHA256 || int64(len(b)) != f.Size {
				t.Fatalf("models/upstream/%s/%s 與 models.lock 不符", role, n)
			}
		}
	}
}

// equivalent.json：0.2.2 以前官網套件內建的 model.onnx 與 models.lock 等價；models.lock 換了 revision 就不再成立
// （0.2.12 select 換成 d512-cv2-20261111 後，equivalent.json 只剩 punct）。
func TestEquivalentHashesFollowTheLock(t *testing.T) {
	l, err := ParseLock(upstream.Lock)
	if err != nil {
		t.Fatal(err)
	}
	eq, err := ParseEquivalents(upstream.Equivalent)
	if err != nil {
		t.Fatal(err)
	}
	const oldSelect = "cef689f1e706537e54eaed4241a776f1ea41b4cad2b504061d1e27cd0c1ca342"
	const oldPunct = "2d98c7c6c75dc9d46081e967dbd2fccef67f73a1ed9b2da44d7e02002e760cf5"
	for _, c := range []struct{ role, sha, want string }{
		{RoleSelect, l.Models[RoleSelect].Files["model.onnx"].SHA256, MatchUpstream},
		{RolePunct, l.Models[RolePunct].Files["model.onnx"].SHA256, MatchUpstream},
		{RoleSelect, oldSelect, MatchNone},                                                          // select 換了 revision
		{RoleSelect, "71af1f0e77b2752cc71cbc04bf19c8e8f2e765662246f766e65a2822bab4e66e", MatchNone}, // 0.2.3–0.2.11 的公開版
		{RolePunct, oldPunct, MatchEquivalent},
		{RolePunct, oldSelect, MatchNone}, // 不跨角色
		{RoleSelect, strings.Repeat("c", 64), MatchNone},
		{RoleSelect, "", MatchNone},
	} {
		if got := l.Match(c.role, c.sha, eq); got != c.want {
			t.Fatalf("%s %s：%q，預期 %q", c.role, c.sha[:min(8, len(c.sha))], got, c.want)
		}
	}
	// 沒有等價清單時只認 models.lock 本身
	if l.Match(RolePunct, oldPunct, Equivalents{}) != MatchNone {
		t.Fatal("沒有等價清單也認了舊雜湊")
	}
	// 上游換了 revision：舊的等價紀錄不再適用
	bumped := l
	bumped.Models = map[string]Model{RoleSelect: l.Models[RoleSelect], RolePunct: l.Models[RolePunct]}
	m := bumped.Models[RolePunct]
	m.Revision = strings.Repeat("e", 40)
	bumped.Models[RolePunct] = m
	if bumped.Match(RolePunct, oldPunct, eq) != MatchNone || l.Match(RolePunct, oldPunct, eq) != MatchEquivalent {
		t.Fatal("換了 revision 的角色仍認舊雜湊")
	}
	if _, err := ParseEquivalents([]byte(`{"schema":1,"models":{"select":{"repository":"r","revision":"abcdef0","model.onnx":"x"}}}`)); err == nil {
		t.Fatal("不正確的雜湊沒有被拒絕")
	}
}
