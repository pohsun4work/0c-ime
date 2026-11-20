package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0c-biggo/0c-ime/internal/autoupdate"
)

// 官方的測試用假的更新器檔案換穩定副本，不該被 fork 改寫 state 或備用的上一版；fork 的測試直接呼叫 claimForkUpdates。
func init() { claimUpdates = func(string, string) {} }

func TestReplaceStableUpdaterClaimsUpdates(t *testing.T) {
	defer func(saved func(string, string)) { claimUpdates = saved }(claimUpdates)
	for _, tc := range []struct {
		name    string
		missing bool
	}{{"換好穩定副本", false}, {"穩定副本換不成", true}} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			src := filepath.Join(t.TempDir(), "0c-update.exe")
			if !tc.missing {
				if err := os.WriteFile(src, []byte("fork"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			var got []string
			claimUpdates = func(src, base string) { got = []string{src, base} }
			if err := replaceStableUpdater(src, base); (err != nil) != tc.missing {
				t.Fatalf("err = %v", err)
			}
			if len(got) != 2 || got[0] != src || got[1] != base {
				t.Fatalf("不論換不換得成都要接著處理更新來源，got %v", got)
			}
		})
	}
}

func TestClaimForkUpdatesPinsFeed(t *testing.T) {
	for _, tc := range []struct{ name, feed, want string }{
		{"沒記網址", "", DefaultFeed},
		{"記著官方網址", upstreamFeed, DefaultFeed},
		{"記著 fork 的舊網址", forkReleases + "download/v0.2.13-hoshi.1/public-beta.json", DefaultFeed},
		{"使用者自己指定的網址不動", "https://example.test/feed.json", "https://example.test/feed.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			s, err := autoupdate.Load(base)
			if err != nil {
				t.Fatal(err)
			}
			s.Feed, s.FeedSerial, s.CheckFailures = tc.feed, 48, 2
			if err := autoupdate.Save(base, s); err != nil {
				t.Fatal(err)
			}
			claimForkUpdates(filepath.Join(base, "missing.exe"), base)
			s, err = autoupdate.Load(base)
			if err != nil {
				t.Fatal(err)
			}
			if s.Feed != tc.want {
				t.Fatalf("Feed = %q，要 %q", s.Feed, tc.want)
			}
			if tc.want == DefaultFeed && (s.FeedSerial != 0 || s.CheckFailures != 0) {
				t.Fatalf("改記 fork 索引時官方的序號與失敗次數要歸零：serial=%d failures=%d", s.FeedSerial, s.CheckFailures)
			}
			if tc.want != DefaultFeed && s.FeedSerial != 48 {
				t.Fatalf("沒改網址時序號不動：serial=%d", s.FeedSerial)
			}
		})
	}
}

func TestClaimForkUpdatesPinsFeedOnFirstInstall(t *testing.T) {
	base := t.TempDir()
	claimForkUpdates(filepath.Join(base, "missing.exe"), base)
	s, err := autoupdate.Load(base)
	if err != nil {
		t.Fatal(err)
	}
	if s.Feed != DefaultFeed {
		t.Fatalf("第一次安裝（還沒有 state）也要記下 fork 的索引，Feed = %q", s.Feed)
	}
}

func TestReleaseForkFeed(t *testing.T) {
	for _, tc := range []struct{ name, feed, want string }{
		{"記著 fork 索引時改回內建", DefaultFeed, ""},
		{"記著 fork 的舊網址也改回內建", forkReleases + "download/v0.2.13-hoshi.1/public-beta.json", ""},
		{"使用者自己指定的網址不動", "https://example.test/feed.json", "https://example.test/feed.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			s, err := autoupdate.Load(base)
			if err != nil {
				t.Fatal(err)
			}
			s.Feed, s.FeedSerial = tc.feed, 1795000000
			if err := autoupdate.Save(base, s); err != nil {
				t.Fatal(err)
			}
			releaseForkFeed(base)
			if s, err = autoupdate.Load(base); err != nil {
				t.Fatal(err)
			}
			if s.Feed != tc.want {
				t.Fatalf("Feed = %q，要 %q", s.Feed, tc.want)
			}
			if tc.want == "" && s.FeedSerial != 0 {
				t.Fatalf("改回內建索引時 fork 的序號要歸零，官方索引的序號比它小：serial=%d", s.FeedSerial)
			}
		})
	}
}

func TestClaimForkUpdatesReplacesForeignPrevious(t *testing.T) {
	defer func(keep bool, foreign func(string) bool) { keepsPreviousUpdater, foreignUpdater = keep, foreign }(keepsPreviousUpdater, foreignUpdater)
	keepsPreviousUpdater = true
	foreignUpdater = func(path string) bool {
		b, err := os.ReadFile(path)
		return err == nil && string(b) == "official"
	}
	for _, tc := range []struct{ name, prev, want string }{
		{"官方的上一版換成這一版", "official", "fork"},
		{"fork 的上一版留著", "fork-old", "fork-old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			src := filepath.Join(t.TempDir(), "0c-update.exe")
			if err := os.WriteFile(src, []byte("fork"), 0o700); err != nil {
				t.Fatal(err)
			}
			prev := previousUpdater(base)
			if err := os.MkdirAll(filepath.Dir(prev), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(prev, []byte(tc.prev), 0o700); err != nil {
				t.Fatal(err)
			}
			claimForkUpdates(src, base)
			if b, err := os.ReadFile(prev); err != nil || string(b) != tc.want {
				t.Fatalf("上一版內容 = %q（%v），要 %q", b, err, tc.want)
			}
		})
	}
}
