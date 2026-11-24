package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/internal/settings"
)

// #258：settings.json 的 shift_symbol_halfwidth 決定 Shift＋符號打半形；health 回報引擎載入的值，
// 設定程式據此比較「已儲存，尚未套用」。fork：缺欄位時是開（所有符號鍵打半形），舊設定檔第一次啟動時換成開；
// 沒在組字時打的符號開關都留在組字區，idle_keys 照樣列 (。
func TestShiftSymbolHalfwidthSettingInServer(t *testing.T) {
	binary := buildServer(t)
	dir := t.TempDir()
	data, _ := filepath.Abs("../../testdata/toy")
	off := map[string]any{"selection": "statistical", "shift_symbol_halfwidth": false}
	for _, tc := range []struct {
		file    map[string]any
		adopted bool
		on      bool
	}{{off, false, true}, {off, true, false}, {map[string]any{"selection": "statistical"}, true, true}} {
		b, _ := json.Marshal(tc.file)
		if err := os.WriteFile(filepath.Join(dir, settings.FileName), b, 0600); err != nil {
			t.Fatal(err)
		}
		hoshiSymbolsAdopted(t, dir, tc.adopted)
		sock := testPipe("shift")
		server, _ := startServer(t, binary, "-data", data, "-user", filepath.Join(dir, "user.json"), "-socket", sock, "-legacy-socket", "")
		func() {
			defer stopServer(t, server)
			waitDial(t, sock)
			conn, err := dial(sock)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			call := func(env ipc.Envelope) ipc.Reply {
				t.Helper()
				if err := ipc.WriteFrame(conn, env); err != nil {
					t.Fatal(err)
				}
				var r ipc.Reply
				if err := ipc.ReadFrame(conn, &r); err != nil {
					t.Fatal(err)
				}
				return r
			}
			want := map[bool]string{false: "false", true: "true"}[tc.on]
			health := call(ipc.Envelope{Op: ipc.OpHealth})
			if health.Info["shift_symbol_halfwidth"] != want || !strings.Contains(health.Info["idle_keys"], "(") {
				t.Fatalf("health：%+v", health.Info)
			}
			open := call(ipc.Envelope{Op: ipc.OpOpen, Version: proto.Version})
			req := proto.Request{Type: proto.Key, Key: "("}
			snap := call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &req}).Snapshot
			if snap == nil || !snap.Handled || snap.Text() != map[bool]string{false: "（", true: "("}[tc.on] {
				t.Fatalf("shift_symbol_halfwidth=%v (：%+v", tc.on, snap)
			}
		}()
	}
}

// hoshiSymbolsAdopted 設定 dir 的設定檔是否已經換過 fork 的符號預設（P4）；沒換過時引擎啟動會先換成所有符號鍵打半形。
func hoshiSymbolsAdopted(t *testing.T, dir string, adopted bool) {
	t.Helper()
	marker := settings.HoshiSymbolsMarker(filepath.Join(dir, settings.FileName))
	if !adopted {
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}
