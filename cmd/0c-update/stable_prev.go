package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0c-biggo/0c-ime/internal/update"
	"github.com/0c-biggo/0c-ime/internal/updatew"
)

// previousUpdater 是上一版更新器的位置，跟穩定副本同一個目錄。
func previousUpdater(base string) string { return filepath.Join(base, "bin", updatew.PreviousName) }

// replaceStableCopy 把排程用的穩定副本換成 src。換下來的那一份改名為 0c-update.prev.exe（只留一份），
// 讓啟動器 0c-updatew 在新版連續異常時有東西可以退回（internal/updatew）；換下來的是啟動器已判定異常的那一份時不保留，留著原本可用的上一版。
// fork 由 replaceStableUpdater（fork_updates.go）包一層。
func replaceStableCopy(src, base string) error {
	return replaceKeepingPrevious(src, stableUpdater(base), previousUpdater(base), func(hash string) bool { return !updatew.Failing(base, hash) })
}

// replaceKeepingPrevious 以 src 換掉 dst，dst 原本的內容改名成 prev（keep 回報 false 時不保留）。
// 內容相同時什麼都不動：不能讓同一份檔案把真正的上一版擠掉。Windows 上執行中的檔案不能覆寫、可以改名，所以全程只用改名：
// 正在執行的穩定副本（通常就是做這件事的程式自己）直接改名成 prev，佔著 prev 的舊檔（可能正被啟動器執行）先改名到旁邊。
func replaceKeepingPrevious(src, dst, prev string, keep func(hash string) bool) error {
	if a, err := filepath.Abs(src); err == nil {
		if b, err := filepath.Abs(dst); err == nil && strings.EqualFold(a, b) {
			return nil
		}
	}
	have, _ := update.Hash(dst)
	want, err := update.Hash(src)
	if err != nil {
		return err
	}
	if have == want {
		removeOldCopies(dst)
		return nil
	}
	if have == "" || !keep(have) {
		return replaceUpdater(src, dst)
	}
	tmp, err := copyBeside(src, dst)
	if err != nil {
		return err
	}
	removeOldCopies(dst)
	removeOldCopies(prev)
	aside := ""
	if _, err := os.Lstat(prev); err == nil {
		aside = fmt.Sprintf("%s.old-%d", prev, time.Now().UnixNano())
		if err := os.Rename(prev, aside); err != nil {
			os.Remove(tmp)
			return replaceUpdater(src, dst) // 留不了上一版也要換上新版
		}
	}
	restorePrev := func() {
		if aside != "" {
			os.Rename(aside, prev)
		}
	}
	if err := os.Rename(dst, prev); err != nil {
		restorePrev()
		os.Remove(tmp)
		return replaceUpdater(src, dst) // 留不了上一版也要換上新版
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(prev, dst)
		restorePrev()
		os.Remove(tmp)
		return err
	}
	if aside != "" {
		os.Remove(aside) // 仍在執行時刪不掉，下次 removeOldCopies(prev) 清
	}
	return nil
}
