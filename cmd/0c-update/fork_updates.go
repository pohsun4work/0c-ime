package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/0c-biggo/0c-ime/internal/autoupdate"
	"github.com/0c-biggo/0c-ime/internal/update"
)

const (
	upstreamFeed = "https://s3.biggo.com/0c-releases/feeds/public-beta.json"
	// forkReleases 認得 fork 換過的索引網址：DefaultFeed 改了以後，記著舊網址的電腦照樣改記新的。
	forkReleases = "https://github.com/pohsun4work/0c-ime/releases/"
)

// replaceStableUpdater 換上排程用的穩定副本，再讓這台電腦只從 fork 更新。換不成時官方的更新器還在，照樣要做。
func replaceStableUpdater(src, base string) error {
	err := replaceStableCopy(src, base)
	claimUpdates(src, base)
	return err
}

// claimUpdates 讓官方的測試可以換掉 fork 的這一步（fork_updates_test.go）。
var claimUpdates = claimForkUpdates

// foreignUpdater 回報 path 是不是不信任 fork 金鑰的（官方的）更新器；檔案不在或讀不到時不算。
var foreignUpdater = func(path string) bool {
	ok, err := update.TrustsReleaseKeys(path)
	return err == nil && !ok
}

// claimForkUpdates 擋掉兩條會換回官方版的路：
//   - state 沒記更新索引、記著官方的或 fork 的舊網址：改記 DefaultFeed。官方更新器被叫起來也先讀 state 的網址，
//     用官方金鑰驗不過，什麼都不裝。
//   - 備用的上一版（bin\0c-update.prev.exe）是官方的：換成 src。啟動器在新版連續異常、或查詢一直失敗
//     而上一版查得到時會改跑它；跟穩定副本相同時啟動器不再退回。
//
// 失敗只提示，不讓安裝失敗。
func claimForkUpdates(src, base string) {
	if err := moveFeed(base, DefaultFeed, func(feed string) bool {
		return feed != DefaultFeed && (feed == "" || feed == upstreamFeed || strings.HasPrefix(feed, forkReleases))
	}); err != nil {
		fmt.Fprintln(os.Stderr, "0c-update: 沒有記下 fork 的更新索引：", err)
	}
	if prev := previousUpdater(base); foreignUpdater(prev) {
		if err := replaceUpdater(src, prev); err != nil {
			fmt.Fprintln(os.Stderr, "0c-update: 備用的上一版更新器沒有換成這一版：", err)
		}
	}
}

// releaseForkFeed：解除安裝時把記下的 fork 索引改回內建。state.json 解除安裝後還在，之後改裝官方版時，
// 官方的更新器才查得到官方的索引。
func releaseForkFeed(base string) {
	if err := moveFeed(base, "", func(feed string) bool { return strings.HasPrefix(feed, forkReleases) }); err != nil {
		fmt.Fprintln(os.Stderr, "0c-update: 沒有清掉 fork 的更新索引：", err)
	}
}

// moveFeed 在 state 記的索引符合 from 時改用 to；不必改時不上鎖、不寫檔，讀不到 state 時不改。
func moveFeed(base, to string, from func(string) bool) error {
	if s, err := autoupdate.Load(base); err != nil || !from(s.Feed) {
		return nil
	}
	_, err := autoupdate.EditState(base, func(s *autoupdate.State) error {
		if from(s.Feed) {
			pointFeed(s, to)
		}
		return nil
	})
	return err
}
