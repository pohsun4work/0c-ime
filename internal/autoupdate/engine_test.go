package autoupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/engine"
	"github.com/0c-biggo/0c-ime/core/lexicon"
	"github.com/0c-biggo/0c-ime/core/lm"
	"github.com/0c-biggo/0c-ime/internal/enginedir"
	"github.com/0c-biggo/0c-ime/internal/update"
)

// TestMain：PrepareCore 以 -check-data 執行套件裡的引擎，測試套件的引擎就是這個測試執行檔（windowsArchiveWith），
// 字典的 report.txt 是 broken 時當成檢查失敗。
func TestMain(m *testing.M) {
	if len(os.Args) == 4 && os.Args[1] == "-check-data" && os.Args[2] == "-data" {
		if b, _ := os.ReadFile(filepath.Join(os.Args[3], "report.txt")); string(b) == "broken" {
			fmt.Fprintln(os.Stderr, "broken")
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// windowsArchive 產生一個最小的 Windows 套件 ZIP；引擎是這個測試執行檔，-check-data 依 fail 決定成敗。
func windowsArchive(t *testing.T, version string, fail bool) (string, string) {
	t.Helper()
	return windowsArchiveWith(t, version, fail, nil)
}

// windowsArchiveWith 同上，另外加入 extra 指定的檔案（路徑 → 內容）。
func windowsArchiveWith(t *testing.T, version string, fail bool, extra map[string]string) (string, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	server, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	report := "r"
	if fail {
		report = "broken"
	}
	files := map[string][]byte{"0c-server.exe": server, "0c-update.exe": []byte("u"), "0c_tsf.dll": []byte("dll"), "0c-ui.exe": []byte("ui"),
		"data/manifest.json": []byte("{}"), "data/lexicon.bin": []byte("l"), "data/lm.bin": []byte("m"), "data/NOTICE.txt": []byte("n"),
		"data/report.txt": []byte(report), "neural/select/manifest.json": []byte("{}")}
	for name, body := range extra {
		files[name] = []byte(body)
	}
	r := update.Release{Schema: 1, Product: "0c", Version: version, Channel: "public-beta", OS: "windows", Arch: "amd64", Files: map[string]string{}}
	for name, body := range files {
		sum := sha256.Sum256(body)
		r.Files[name] = hex.EncodeToString(sum[:])
	}
	files["release.json"], _ = json.Marshal(r)
	archive := filepath.Join(t.TempDir(), "bundle.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, body := range files {
		w, err := z.CreateHeader(&zip.FileHeader{Name: "0c-" + version + "/" + name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	sum, _ := update.Hash(archive)
	return archive, sum
}

var windowsCore = CoreLayout{Server: "0c-server.exe", Update: "0c-update.exe"}

func TestPrepareCoreSwitchRevertAndPrune(t *testing.T) {
	base := t.TempDir()
	archive, sum := windowsArchive(t, "0.1.0-dev.18", false)
	target := Target{Version: "0.1.0-dev.18", Kind: "core", SHA256: sum}
	dir, err := PrepareCore(base, archive, target, "windows", "amd64", windowsCore)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"0c-server.exe", "0c-update.exe", "data/manifest.json", "neural/select/manifest.json"} {
		if _, err := os.Stat(filepath.Join(base, dir, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, dir, "0c_tsf.dll")); err == nil {
		t.Fatal("shell files must not be copied into a core directory")
	}
	if again, err := PrepareCore(base, archive, target, "windows", "amd64", windowsCore); err != nil || again != dir {
		t.Fatalf("prepare must be idempotent: %v %v", again, err)
	}
	if err := SwitchPointer(base, dir, target.Version); err != nil || PointerVersion(base) != "0.1.0-dev.18" {
		t.Fatalf("switch: %v %q", err, PointerVersion(base))
	}
	if err := RevertPointer(base, dir); err != nil {
		t.Fatal(err)
	}
	if c, _ := enginedir.Read(base); c != nil {
		t.Fatal("revert to no pointer must remove engine.json")
	}
	// 切換中途被中止後重跑：原指標保留最初那一個；回復可以重複執行。
	old := "versions/0.1.0-dev.17-c-bbbbbbbbbbbb"
	os.MkdirAll(filepath.Join(base, old, "data"), 0o700)
	os.WriteFile(filepath.Join(base, old, enginedir.Executable()), []byte("x"), 0o700)
	os.WriteFile(filepath.Join(base, old, "data/manifest.json"), []byte("{}"), 0o600)
	if err := SwitchPointer(base, old, "0.1.0-dev.17"); err != nil {
		t.Fatal(err)
	}
	SwitchPointer(base, dir, target.Version)
	SwitchPointer(base, dir, target.Version) // 重跑不可蓋掉原指標
	for i := 0; i < 2; i++ {
		if err := RevertPointer(base, dir); err != nil {
			t.Fatal(err)
		}
		if c, _ := enginedir.Read(base); c == nil || c.Directory != old {
			t.Fatalf("revert #%d: %+v", i+1, c)
		}
	}
	enginedir.Remove(base)
	os.MkdirAll(filepath.Join(base, "versions/0.1.0-dev.17-c-aaaaaaaaaaaa"), 0o700)
	os.MkdirAll(filepath.Join(base, "versions/0.1.0-dev.17-abcdef"), 0o700) // 完整安裝的目錄不歸這裡管
	SwitchPointer(base, dir, target.Version)
	PruneCore(base)
	if _, err := os.Stat(filepath.Join(base, "versions/0.1.0-dev.17-c-aaaaaaaaaaaa")); err == nil {
		t.Fatal("stale core directory must be pruned")
	}
	for _, keep := range []string{dir, "versions/0.1.0-dev.17-abcdef"} {
		if _, err := os.Stat(filepath.Join(base, keep)); err != nil {
			t.Fatalf("%s must be kept", keep)
		}
	}
}

// 設定程式是核心版本目錄的選用成員：複製並記錄雜湊；舊版佈局（不認得它）與沒有它的套件都照常準備。
func TestPrepareCoreCopiesSettingsProgramAndRecordsHashes(t *testing.T) {
	withSettings := CoreLayout{Server: "0c-server.exe", Update: "0c-update.exe", Settings: "0c-settings.exe"}
	archive, sum := windowsArchiveWith(t, "0.1.0-dev.43", false, map[string]string{"0c-settings.exe": "settings-program"})
	target := Target{Version: "0.1.0-dev.43", Kind: "core", SHA256: sum}
	want := sha256.Sum256([]byte("settings-program"))

	base := t.TempDir()
	dir, err := PrepareCore(base, archive, target, "windows", "amd64", withSettings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, dir, "0c-settings.exe")); err != nil {
		t.Fatalf("設定程式應複製：%v", err)
	}
	hashes, err := enginedir.ReadFiles(filepath.Join(base, dir))
	if err != nil || hashes["0c-settings.exe"] != hex.EncodeToString(want[:]) || hashes["0c-server.exe"] == "" || hashes["0c-update.exe"] == "" {
		t.Fatalf("應記錄最上層檔案的雜湊：%v %v", hashes, err)
	}
	for name := range hashes {
		if name != "0c-server.exe" && name != "0c-update.exe" && name != "0c-settings.exe" {
			t.Errorf("不該記錄 %s（data／neural 檔案太多，不逐一記錄）", name)
		}
	}
	if got, ok := enginedir.Dir(base); ok {
		t.Fatalf("尚未切換指標，不該有目前版本目錄：%s", got)
	}
	if err := SwitchPointer(base, dir, target.Version); err != nil {
		t.Fatal(err)
	}
	if got, ok := enginedir.Dir(base); !ok || got != filepath.Join(base, dir) {
		t.Fatalf("Dir：%q %v", got, ok)
	}

	// 舊版佈局不認得設定程式：略過它，其餘照常，也不在記錄裡出現。
	old := t.TempDir()
	dir, err = PrepareCore(old, archive, target, "windows", "amd64", windowsCore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(old, dir, "0c-settings.exe")); err == nil {
		t.Fatal("不認得設定程式的佈局不得複製它")
	}
	if hashes, _ := enginedir.ReadFiles(filepath.Join(old, dir)); hashes["0c-settings.exe"] != "" || hashes["0c-server.exe"] == "" {
		t.Fatalf("%v", hashes)
	}

	// 套件沒有設定程式：照常準備，只是沒有它。
	plain, plainSum := windowsArchive(t, "0.1.0-dev.42", false)
	dir, err = PrepareCore(t.TempDir(), plain, Target{Version: "0.1.0-dev.42", Kind: "core", SHA256: plainSum}, "windows", "amd64", withSettings)
	if err != nil || dir == "" {
		t.Fatal(err)
	}
}

// 0c-mcp（#126）同樣是選用成員：複製並記錄雜湊，切換後 Windows 由它換上 bin\0c-mcp.exe；不認得它的佈局略過。
func TestPrepareCoreCopiesMCPAndRecordsItsHash(t *testing.T) {
	withMCP := CoreLayout{Server: "0c-server.exe", Update: "0c-update.exe", MCP: "0c-mcp.exe"}
	archive, sum := windowsArchiveWith(t, "0.2.0-dev.1", false, map[string]string{"0c-mcp.exe": "mcp-bridge"})
	target := Target{Version: "0.2.0-dev.1", Kind: "core", SHA256: sum}
	want := sha256.Sum256([]byte("mcp-bridge"))
	base := t.TempDir()
	dir, err := PrepareCore(base, archive, target, "windows", "amd64", withMCP)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(base, dir, "0c-mcp.exe")); err != nil || string(b) != "mcp-bridge" {
		t.Fatalf("0c-mcp 應複製到核心目錄最上層：%q %v", b, err)
	}
	if hashes, err := enginedir.ReadFiles(filepath.Join(base, dir)); err != nil || hashes["0c-mcp.exe"] != hex.EncodeToString(want[:]) {
		t.Fatalf("應記錄 0c-mcp 的雜湊：%v %v", hashes, err)
	}
	old := t.TempDir()
	if dir, err = PrepareCore(old, archive, target, "windows", "amd64", windowsCore); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(old, dir, "0c-mcp.exe")); err == nil {
		t.Fatal("不認得 0c-mcp 的佈局不得複製它")
	}
}

// 0c-updatew（#289，Windows 排程用的無主控台啟動器）同樣是選用成員：複製並記錄雜湊，切換後由它換上穩定副本。
func TestPrepareCoreCopiesLauncherAndRecordsItsHash(t *testing.T) {
	withLauncher := CoreLayout{Server: "0c-server.exe", Update: "0c-update.exe", Launcher: "0c-updatew.exe"}
	archive, sum := windowsArchiveWith(t, "0.2.0-dev.1", false, map[string]string{"0c-updatew.exe": "launcher"})
	target := Target{Version: "0.2.0-dev.1", Kind: "core", SHA256: sum}
	want := sha256.Sum256([]byte("launcher"))
	base := t.TempDir()
	dir, err := PrepareCore(base, archive, target, "windows", "amd64", withLauncher)
	if err != nil {
		t.Fatal(err)
	}
	if hashes, err := enginedir.ReadFiles(filepath.Join(base, dir)); err != nil || hashes["0c-updatew.exe"] != hex.EncodeToString(want[:]) {
		t.Fatalf("應記錄 0c-updatew 的雜湊：%v %v", hashes, err)
	}
	old := t.TempDir()
	if dir, err = PrepareCore(old, archive, target, "windows", "amd64", windowsCore); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(old, dir, "0c-updatew.exe")); err == nil {
		t.Fatal("不認得 0c-updatew 的佈局不得複製它")
	}
}

func TestReadFilesIsNilForDirectoriesPreparedByOlderUpdaters(t *testing.T) {
	if hashes, err := enginedir.ReadFiles(t.TempDir()); hashes != nil || err != nil {
		t.Fatalf("%v %v", hashes, err)
	}
}

func TestPrepareCoreRejectsBrokenEngineAndWrongVersion(t *testing.T) {
	base := t.TempDir()
	archive, sum := windowsArchive(t, "0.1.0-dev.18", true)
	if _, err := PrepareCore(base, archive, Target{Version: "0.1.0-dev.18", SHA256: sum}, "windows", "amd64", windowsCore); err == nil {
		t.Fatal("an engine that fails -check-data must not be prepared")
	}
	entries, _ := os.ReadDir(filepath.Join(base, "versions"))
	if len(entries) != 0 {
		t.Fatalf("failed prepare must leave nothing: %v", entries)
	}
	archive, sum = windowsArchive(t, "0.1.0-dev.18", false)
	if _, err := PrepareCore(base, archive, Target{Version: "0.1.0-dev.19", SHA256: sum}, "windows", "amd64", windowsCore); err == nil {
		t.Fatal("bundle version must match the signed feed")
	}
}

func toyEngine(t *testing.T) *engine.Engine {
	lf, _ := os.Open("../../testdata/toy/lexicon.tsv")
	defer lf.Close()
	lex, err := lexicon.Load(lf)
	if err != nil {
		t.Fatal(err)
	}
	mf, _ := os.Open("../../testdata/toy/lm.tsv")
	defer mf.Close()
	sys, err := lm.Load(mf)
	if err != nil {
		t.Fatal(err)
	}
	return engine.New(lex, sys)
}

func TestHealthAndDrainOverPipe(t *testing.T) {
	pipe := fmt.Sprintf(`\\.\pipe\0c-test-autoupdate-%d`, os.Getpid())
	dial := func(d time.Duration) (net.Conn, error) { return winio.DialPipe(pipe, &d) }
	if _, running, err := Health(dial); running || err != nil {
		t.Fatal("no engine must report not running without error")
	}
	if err := Drain(context.Background(), dial, time.Second); err != nil {
		t.Fatal(err)
	}
	l, err := winio.ListenPipe(pipe, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := &ipc.Server{Engine: toyEngine(t), Info: map[string]string{"version": "0.1.0-dev.18"}, DrainTimeout: 2 * time.Second}
	srv.OnDrained = func() { time.Sleep(300 * time.Millisecond); l.Close() } // 模擬存檔需要時間
	go srv.Serve(l)
	info, running, err := Health(dial)
	if !running || err != nil || info["version"] != "0.1.0-dev.18" || info["idle_seconds"] != "0" {
		t.Fatalf("%v %v %v", info, running, err)
	}
	if err := Drain(context.Background(), dial, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, running, _ := Health(dial); running {
		t.Fatal("drain must return only after the engine released its name")
	}
}
