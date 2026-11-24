package update

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/enginedir"
)

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func quietPrune(t *testing.T) *[]string {
	t.Helper()
	var warned []string
	saved := PruneWarn
	PruneWarn = func(path string, err error) { warned = append(warned, path) }
	t.Cleanup(func() { PruneWarn = saved })
	return &warned
}

// prune 是背景行程（0c-update prune）與排程喚起時做的清理。
func prune(t *testing.T, base string) {
	t.Helper()
	if err := PruneWindows(base); err != nil {
		t.Fatal(err)
	}
}

func windowsDirs(t *testing.T, base, sub string) []string {
	t.Helper()
	return dirNames(t, filepath.Join(base, sub))
}

// #179 Windows：備份只留收據 previous 那一份；版本目錄只留目前、回復目標與引擎指標用到的。
// 舊版累積的（含中斷安裝留下的 .broken-）一次清掉；核心更新的 -c- 目錄與其他名稱不碰。
func TestWindowsPruneKeepsCurrentAndRollbackOnly(t *testing.T) {
	quietPrune(t)
	base := t.TempDir()
	os.WriteFile(filepath.Join(base, UserStore), []byte("private learning"), 0o600)
	r := &fakeRegistrar{}
	first, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.1"), base, "amd64", r)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.2"), base, "amd64", r)
	if err != nil {
		t.Fatal(err)
	}
	// 舊版累積下來的：更早的版本目錄與備份、核心更新目錄、中斷安裝留下的、別的名稱。
	for _, name := range []string{"versions/0.1.0-dev.0-0123456789ab", "versions/0.1.0-dev.0-0123456789ab.broken-17",
		"versions/0.1.0-dev.2-c-0123456789ab", "versions/notes", "backups/windows-111", "backups/windows-222", "backups/manual"} {
		os.MkdirAll(filepath.Join(base, filepath.FromSlash(name), "data"), 0o700)
		os.WriteFile(filepath.Join(base, filepath.FromSlash(name), "data", "lm.bin"), []byte("old"), 0o600)
	}
	os.WriteFile(filepath.Join(base, "backups", "windows-333"), []byte("a file"), 0o600)

	third, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.3"), base, "amd64", r)
	if err != nil {
		t.Fatal(err)
	}
	// 安裝本身不清（第一次升級要刪數 GB，安裝精靈會卡在進度條），交給背景的 PruneWindows。
	if _, err := os.Stat(first); err != nil {
		t.Fatal("install pruned synchronously")
	}
	if _, err := os.Stat(filepath.Join(base, "backups", "windows-111")); err != nil {
		t.Fatal("install pruned backups synchronously")
	}
	prune(t, base)
	current, _ := readReceipt(filepath.Join(base, "install-receipt.json"))
	gotVersions := windowsDirs(t, base, "versions")
	wantVersions := []string{filepath.Base(second), filepath.Base(third), "0.1.0-dev.2-c-0123456789ab", "notes"}
	slices.Sort(wantVersions)
	if !slices.Equal(gotVersions, wantVersions) {
		t.Fatalf("versions = %v, want %v", gotVersions, wantVersions)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("kept a version that is neither current nor the rollback target")
	}
	wantBackups := []string{strings.TrimPrefix(current.Previous, "backups/"), "manual", "windows-333"}
	slices.Sort(wantBackups)
	if got := windowsDirs(t, base, "backups"); !slices.Equal(got, wantBackups) {
		t.Fatalf("backups = %v, want %v", got, wantBackups)
	}

	// 回復與撤回回復照常；回復後剛換下的版本留著，更早的備份清掉。
	if _, err = RollbackWindows(base, r, true); err != nil {
		t.Fatal(err)
	}
	if r.path != filepath.Join(second, "0c_tsf.dll") {
		t.Fatal("not rolled back")
	}
	if _, err := os.Stat(third); err != nil {
		t.Fatal("removed the version just rolled back from")
	}
	if got := windowsDirs(t, base, "backups"); len(got) != len(wantBackups)+1 {
		t.Fatalf("rollback pruned synchronously: %v", got)
	}
	prune(t, base)
	if b, _ := os.ReadFile(filepath.Join(base, UserStore)); string(b) != "private learning" {
		t.Fatal("changed user data")
	}
	if _, err = RollbackWindows(base, r, false); err != nil {
		t.Fatal(err)
	}
	if r.path != filepath.Join(third, "0c_tsf.dll") {
		t.Fatal("cannot undo rollback")
	}
	prune(t, base)
	own := 0
	for _, name := range windowsDirs(t, base, "backups") {
		if windowsBackupName.MatchString(name) {
			own++
		}
	}
	if own != 2 { // previous 那一份，加上一般檔案 windows-333
		t.Fatalf("own backups after rollback = %d", own)
	}

	// 解除安裝：只留回復用的那一份，rollback 可恢復。
	if _, err = UninstallWindows(base, r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("uninstall pruned synchronously")
	}
	prune(t, base)
	if _, err := os.Stat(third); err != nil {
		t.Fatal("uninstall removed the version needed to restore")
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("uninstall kept an older version")
	}
	if _, err = RollbackWindows(base, r, false); err != nil {
		t.Fatal(err)
	}
	if r.path != filepath.Join(third, "0c_tsf.dll") {
		t.Fatal("cannot undo uninstall")
	}
}

// 引擎指標（核心更新）與回復目標記錄的引擎目錄都要留著；讀不到回復目標時這次不清版本目錄。
func TestWindowsPruneKeepsEngineDirectoriesAndStopsWhenUnsure(t *testing.T) {
	quietPrune(t)
	base := t.TempDir()
	r := &fakeRegistrar{}
	if _, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.1"), base, "amd64", r); err != nil {
		t.Fatal(err)
	}
	pointed := "versions/0.1.0-dev.0-aaaaaaaaaaaa"
	recorded := "versions/0.1.0-dev.0-bbbbbbbbbbbb"
	unused := "versions/0.1.0-dev.0-cccccccccccc"
	for _, dir := range []string{pointed, recorded, unused} {
		os.MkdirAll(filepath.Join(base, filepath.FromSlash(dir)), 0o700)
	}
	receipt, _ := readReceipt(filepath.Join(base, "install-receipt.json"))
	receipt.Engine = recorded
	writeJSON(filepath.Join(base, "install-receipt.json"), receipt)
	if err := enginedir.Write(base, enginedir.Current{Directory: pointed, Version: "0.1.0-dev.0"}); err != nil {
		t.Fatal(err)
	}
	prune(t, base)
	for dir, want := range map[string]bool{pointed: true, recorded: true, unused: false, receipt.Directory: true} {
		if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(dir))); (err == nil) != want {
			t.Fatalf("%s exists=%v, want %v", dir, err == nil, want)
		}
	}
	// 回復目標的收據讀不出來：不知道它用哪個版本目錄，版本目錄全部保留。
	os.MkdirAll(filepath.Join(base, filepath.FromSlash(unused)), 0o700)
	os.WriteFile(filepath.Join(base, filepath.FromSlash(receipt.Previous), "previous.json"), []byte("{broken"), 0o600)
	prune(t, base)
	if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(unused))); err != nil {
		t.Fatal("pruned versions without knowing the rollback target")
	}
}

func TestWindowsOwnedVersionNames(t *testing.T) {
	for name, want := range map[string]bool{
		"0.2.2-0123456789ab":                    true,
		"0.1.0-dev.63.144-2e497eb26a1a":         true,
		"0.1.0-dev.61-549eabb97acb.broken-1792": true,
		"0.2.2-c-0123456789ab":                  false, // 核心更新，由 PruneCore 管理
		"0.2.2-0123456789AB":                    false,
		"0.2.2-0123456789a":                     false,
		"notes":                                 false,
		".stage-123":                            false,
		"0.2.2-0123456789ab.bak":                false,
	} {
		if got := windowsOwnedVersion(name); got != want {
			t.Errorf("windowsOwnedVersion(%q) = %v, want %v", name, got, want)
		}
	}
}

// Windows：執行中的 exe、已載入的 DLL 不擋目錄改名（.14 實測），所以先刪 DLL、再刪執行檔；刪不掉就保留其餘檔案。
func TestWindowsPruneKeepsVersionsWithProgramsInUse(t *testing.T) {
	warned := quietPrune(t)
	base := t.TempDir()
	r := &fakeRegistrar{}
	if _, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.1"), base, "amd64", r); err != nil {
		t.Fatal(err)
	}
	loaded := filepath.Join(base, "versions", "0.1.0-dev.0-aaaaaaaaaaaa")
	running := filepath.Join(base, "versions", "0.1.0-dev.0-bbbbbbbbbbbb")
	idle := filepath.Join(base, "versions", "0.1.0-dev.0-cccccccccccc")
	for _, dir := range []string{loaded, running, idle} {
		for _, name := range []string{"0c_tsf.dll", "x86/0c_tsf.dll", "0c-server.exe", "0c-ui.exe", "data/lm.bin"} {
			os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700)
			os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600)
		}
	}
	inUse := map[string]bool{filepath.Join(loaded, "x86", "0c_tsf.dll"): true, filepath.Join(running, "0c-server.exe"): true}
	removeProgram = func(p string) error {
		if inUse[p] {
			return os.ErrPermission
		}
		return os.Remove(p)
	}
	t.Cleanup(func() { removeProgram = os.Remove })
	receipt, _ := readReceipt(filepath.Join(base, "install-receipt.json"))
	prune(t, base)
	if _, err := os.Stat(idle); !os.IsNotExist(err) {
		t.Fatal("idle version kept")
	}
	for _, p := range []string{filepath.Join(loaded, "0c-server.exe"), filepath.Join(loaded, "data/lm.bin"), filepath.Join(running, "0c-server.exe"), filepath.Join(running, "data/lm.bin")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal("removed a file of a version in use:", p)
		}
	}
	if _, err := os.Stat(filepath.Join(running, "0c_tsf.dll")); !os.IsNotExist(err) {
		t.Fatal("the DLL goes first, so no application loads the old version again")
	}
	if !slices.Contains(*warned, loaded) || !slices.Contains(*warned, running) {
		t.Fatalf("kept versions not reported: %v", *warned)
	}
	// 程式結束、登出之後，下一次就清掉。
	removeProgram = os.Remove
	prune(t, base)
	for _, dir := range []string{loaded, running} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("version kept after its programs stopped:", dir)
		}
	}
	if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(receipt.Directory), "0c_tsf.dll")); err != nil {
		t.Fatal("removed the installed version")
	}
}

// #179 背景清理：正在安裝、回復（安裝鎖存在）時不清，也不動那把鎖，交給下一次；沒東西可清時不拿鎖。
func TestWindowsPruneGivesUpWhileInstallLocked(t *testing.T) {
	quietPrune(t)
	base := t.TempDir()
	r := &fakeRegistrar{}
	first, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.1"), base, "amd64", r)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = InstallWindows(bundleFixture(t, "0.1.0-dev.2"), base, "amd64", r); err != nil {
		t.Fatal(err)
	}
	if _, _, err = InstallWindows(bundleFixture(t, "0.1.0-dev.3"), base, "amd64", r); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(base, "versions", "0.1.0-dev.0-0123456789ab")
	os.MkdirAll(old, 0o700)
	lock := filepath.Join(base, "update.lock")
	if err := os.WriteFile(lock, []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := PruneWindows(base); !errors.Is(err, ErrInstallLocked) {
		t.Fatalf("PruneWindows while locked = %v", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("pruned while another update holds the lock")
	}
	if b, err := os.ReadFile(lock); err != nil || string(b) != "4242\n" {
		t.Fatal("touched the lock of another update")
	}
	os.Remove(lock)
	prune(t, base)
	for _, dir := range []string{old, first} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("not pruned after the lock was released:", dir)
		}
	}
	// 沒東西可清：不拿鎖（排程每次喚起都會呼叫），安裝鎖在也照常回傳。
	os.WriteFile(lock, []byte("4242\n"), 0o600)
	if err := PruneWindows(base); err != nil {
		t.Fatalf("nothing to prune but PruneWindows = %v", err)
	}
	// 沒有安裝：什麼都不做，也不建立安裝目錄。
	none := filepath.Join(t.TempDir(), "Zuin")
	if err := PruneWindows(none); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(none); !os.IsNotExist(err) {
		t.Fatal("created the installation directory")
	}
}

// 鎖內只改名移開，放開鎖之後才刪：刪數 GB 時不擋安裝，被中斷也不會留下安裝鎖。上次沒刪完的 .removing-* 一併刪。
func TestWindowsPruneDeletesAfterReleasingLock(t *testing.T) {
	quietPrune(t)
	base := t.TempDir()
	r := &fakeRegistrar{}
	if _, _, err := InstallWindows(bundleFixture(t, "0.1.0-dev.1"), base, "amd64", r); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"versions/0.1.0-dev.0-0123456789ab", "versions/.removing-0.1.0-dev.0-aaaaaaaaaaaa",
		"versions/.removing-notes", "backups/windows-111", "backups/.removing-windows-222"} {
		os.MkdirAll(filepath.Join(base, filepath.FromSlash(name), "data"), 0o700)
	}
	var removed []string
	removeAll = func(p string) error {
		if _, err := os.Stat(filepath.Join(base, "update.lock")); err == nil {
			t.Error("deleting while holding the install lock:", p)
		}
		removed = append(removed, filepath.Base(p))
		return os.RemoveAll(p)
	}
	t.Cleanup(func() { removeAll = os.RemoveAll })
	prune(t, base)
	slices.Sort(removed)
	want := []string{".removing-0.1.0-dev.0-0123456789ab", ".removing-0.1.0-dev.0-aaaaaaaaaaaa", ".removing-windows-111", ".removing-windows-222"}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed %v, want %v", removed, want)
	}
	if _, err := os.Stat(filepath.Join(base, "versions", ".removing-notes")); err != nil {
		t.Fatal("removed a directory that is not ours")
	}
	if _, err := os.Stat(filepath.Join(base, "update.lock")); !os.IsNotExist(err) {
		t.Fatal("left the install lock")
	}
}
