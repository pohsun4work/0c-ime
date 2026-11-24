package main

import (
	"archive/zip"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0c-biggo/0c-ime/internal/update"
)

func writeZip(t *testing.T, path string, release map[string]any) {
	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	w, _ := z.Create("0c-x/release.json")
	b, _ := json.Marshal(release)
	w.Write(b)
	z.Close()
	f.Close()
}

func TestFeedSignsRealBundleMetadata(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "k")
	if err := keygen([]string{"-private", key}); err != nil {
		t.Fatal(err)
	}
	priv, err := loadKey(key)
	if err != nil {
		t.Fatal(err)
	}
	amd64 := filepath.Join(dir, "w.zip")
	arm64 := filepath.Join(dir, "a.zip")
	writeZip(t, amd64, map[string]any{"product": "0c", "version": "0.1.0-dev.18", "channel": "internal-preview", "os": "windows", "arch": "amd64"})
	writeZip(t, arm64, map[string]any{"product": "0c", "version": "0.1.0-dev.18", "channel": "internal-preview", "os": "windows", "arch": "arm64"})
	out := filepath.Join(dir, "latest.json")
	if err := feed([]string{"-key", key, "-serial", "3", "-out", out, "-kind", "shell",
		"-asset", amd64 + "=https://s3.example/w.zip", "-asset", arm64 + "=https://s3.example/a.zip"}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(out)
	sig, _ := os.ReadFile(out + ".sig")
	f, err := update.VerifySignedFeed(body, sig, []ed25519.PublicKey{priv.Public().(ed25519.PublicKey)}, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != "0.1.0-dev.18" || len(f.Assets) != 2 || f.Assets[0].OS != "windows" || f.Assets[0].Arch != "amd64" ||
		f.Assets[1].Arch != "arm64" || f.Assets[0].Kind != "shell" || f.Assets[1].Kind != "shell" {
		t.Fatalf("feed %+v", f)
	}
	// 只收 Windows 套件：其他平台的 release.json 與不是 ZIP 的檔案都拒絕。
	other := filepath.Join(dir, "l.zip")
	writeZip(t, other, map[string]any{"product": "0c", "version": "0.1.0-dev.18", "channel": "internal-preview", "os": "linux", "arch": "amd64"})
	tarball := filepath.Join(dir, "l.tar.gz")
	os.WriteFile(tarball, []byte("not a zip"), 0o600)
	for _, bad := range []string{other, tarball} {
		if err := feed([]string{"-key", key, "-serial", "4", "-out", out, "-asset", bad + "=https://s3.example/x"}); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
	if err := keygen([]string{"-private", key}); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("keygen must not overwrite an existing key: %v", err)
	}
}

// 續簽：只接受信任金鑰簽的索引（過期也可），序號加一、內容不變；有效期還長時不動。
func TestRenewKeepsContentAndBumpsSerial(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	issued := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	f := &update.SignedFeed{Schema: update.SignedFeedSchema, Product: "0c", Version: "0.1.0-dev.20", Channel: "public-beta",
		Serial: 3, Issued: issued, Expires: issued.Add(14 * 24 * time.Hour), Rollout: 100, Withdrawn: []string{"0.1.0-dev.17"},
		Assets: []update.SignedAsset{{Asset: update.Asset{OS: "windows", Arch: "amd64", URL: "https://s3.example/w.zip", SHA256: strings.Repeat("a", 64), Size: 10}, Kind: "core"}}}
	body, sig, err := update.SignFeed(f, priv)
	if err != nil {
		t.Fatal(err)
	}
	keys := []ed25519.PublicKey{pub}
	if g, _, err := renewFeed(body, sig, keys, issued.Add(24*time.Hour), 7*24*time.Hour); err != nil || g != nil {
		t.Fatalf("剩 13 天不應續簽：%v %v", g, err)
	}
	later := issued.Add(20 * 24 * time.Hour) // 已過期
	g, previous, err := renewFeed(body, sig, keys, later, 7*24*time.Hour)
	if err != nil || g == nil {
		t.Fatalf("過期應可續簽：%v", err)
	}
	if g.Serial != 4 || !g.Issued.Equal(later) || !previous.Equal(f.Expires) || g.Version != f.Version || g.Rollout != 100 ||
		len(g.Assets) != 1 || g.Assets[0] != f.Assets[0] || len(g.Withdrawn) != 1 {
		t.Fatalf("續簽改了內容：%+v", g)
	}
	g.Expires = later.Add(14 * 24 * time.Hour)
	nb, ns, err := update.SignFeed(g, priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update.VerifySignedFeed(nb, ns, keys, later, 3); err != nil {
		t.Fatalf("續簽後客戶端應接受：%v", err)
	}
	_, other, _ := ed25519.GenerateKey(nil)
	_, badSig, _ := update.SignFeed(f, other)
	if _, _, err := renewFeed(body, badSig, keys, later, 7*24*time.Hour); err == nil {
		t.Fatal("接受了非信任金鑰簽的索引")
	}
	if _, _, err := renewFeed(append([]byte(" "), body...), sig, keys, later, 7*24*time.Hour); err == nil {
		t.Fatal("接受了被改過的索引")
	}
}
