package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/internal/settings"
)

// #168：settings.json 的 ctrl_punctuation（缺欄位時預設開）決定 Ctrl＋標點打全形；health 與 open 的 info 回報
// 引擎載入的值，外殼據此決定要不要送那六個鍵，設定程式據此比較「已儲存，尚未套用」。
func TestCtrlPunctuationSettingInServer(t *testing.T) {
	binary := buildServer(t)
	dir := t.TempDir()
	data, _ := filepath.Abs("../../testdata/toy")
	for _, tc := range []struct {
		file map[string]any
		on   bool
	}{
		{map[string]any{"selection": "statistical"}, true},
		// fork：預設所有符號鍵打半形，會固定開著 Ctrl＋標點；標點寬度關掉才看得到這個設定本身
		{map[string]any{"selection": "statistical", "ctrl_punctuation": false, "shift_symbol_halfwidth": false}, false},
	} {
		b, _ := json.Marshal(tc.file)
		if err := os.WriteFile(filepath.Join(dir, settings.FileName), b, 0600); err != nil {
			t.Fatal(err)
		}
		hoshiSymbolsAdopted(t, dir, true)
		sock := testPipe("ctrl")
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
			if health := call(ipc.Envelope{Op: ipc.OpHealth}); health.Info["ctrl_punctuation"] != want {
				t.Fatalf("health：%+v", health.Info)
			}
			open := call(ipc.Envelope{Op: ipc.OpOpen, Version: proto.Version})
			if open.Info["ctrl_punctuation"] != want {
				t.Fatalf("open：%+v", open.Info)
			}
			// #231：認得哪些鍵不跟著設定，新外殼據此決定要不要送 Ctrl+: ( )；fork 加了 [ ]。
			if open.Info["ctrl_punctuation_keys"] != ",.';?!:()[]" {
				t.Fatalf("open 的 ctrl_punctuation_keys：%+v", open.Info)
			}
			req := proto.Request{Type: proto.Key, Key: proto.KeyCtrlParenLeft}
			snap := call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &req}).Snapshot
			if snap == nil || snap.Handled != tc.on || (tc.on && snap.Text() != "（") || (!tc.on && snap.Text() != "") {
				t.Fatalf("ctrl_punctuation=%v Ctrl+(：%+v", tc.on, snap)
			}
			call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &proto.Request{Type: proto.Key, Key: proto.KeyEscape}})
			req = proto.Request{Type: proto.Key, Key: proto.KeyCtrlComma}
			snap = call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &req}).Snapshot
			if snap == nil || snap.Handled != tc.on || (tc.on && snap.Text() != "，") || (!tc.on && snap.Text() != "") {
				t.Fatalf("ctrl_punctuation=%v：%+v", tc.on, snap)
			}
			call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &proto.Request{Type: proto.Key, Key: proto.KeyEscape}})
			req = proto.Request{Type: proto.Key, Key: proto.KeyCtrlBracketLeft}
			snap = call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &req}).Snapshot
			if snap == nil || snap.Handled != tc.on || (tc.on && snap.Text() != "【") || (!tc.on && snap.Text() != "") {
				t.Fatalf("ctrl_punctuation=%v Ctrl+[：%+v", tc.on, snap)
			}
		}()
	}
}
