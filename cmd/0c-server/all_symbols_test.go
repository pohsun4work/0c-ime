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

// #282：標點寬度選「所有符號鍵打半形」時，引擎載入 all_symbols_halfwidth，Ctrl＋標點固定開、聲調選填暫停（設定檔關著也一樣），
// health 回報實際值給 0c-update 比較。fork：沒在組字時打的符號也留在組字區，idle_keys 照樣列 [ _。
func TestAllSymbolsHalfwidthSettingInServer(t *testing.T) {
	binary := buildServer(t)
	dir := t.TempDir()
	data, _ := filepath.Abs("../../testdata/toy")
	b, _ := json.Marshal(map[string]any{"selection": "statistical", "shift_symbol_halfwidth": true, "all_symbols_halfwidth": true, "ctrl_punctuation": false, "toneless": true})
	if err := os.WriteFile(filepath.Join(dir, settings.FileName), b, 0600); err != nil {
		t.Fatal(err)
	}
	sock := testPipe("all")
	server, _ := startServer(t, binary, "-data", data, "-user", filepath.Join(dir, "user.json"), "-socket", sock, "-legacy-socket", "")
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
	info := call(ipc.Envelope{Op: ipc.OpHealth}).Info
	if info["all_symbols_halfwidth"] != "true" || info["shift_symbol_halfwidth"] != "true" || info["ctrl_punctuation"] != "true" || info["toneless"] != "false" || !strings.Contains(info["idle_keys"], "[") || !strings.Contains(info["idle_keys"], "_") {
		t.Fatalf("health：%+v", info)
	}
	open := call(ipc.Envelope{Op: ipc.OpOpen, Version: proto.Version})
	for _, tc := range []struct {
		key, text string
		handled   bool
	}{{"[", "[", true}, {"_", "[_", true}, {proto.KeyCtrlComma, "[_，", true}, {"[", "[_，[", true}} {
		req := proto.Request{Type: proto.Key, Key: tc.key}
		snap := call(ipc.Envelope{Op: ipc.OpHandle, Session: open.Session, Req: &req}).Snapshot
		if snap == nil || snap.Handled != tc.handled || snap.Text() != tc.text {
			t.Fatalf("%q：%+v", tc.key, snap)
		}
	}
}
