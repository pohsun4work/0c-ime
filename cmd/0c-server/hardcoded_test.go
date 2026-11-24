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

// P4.5：符號寬度、Ctrl＋標點、混打寫死。舊設定檔裡關掉它們（或打開不打聲調）的欄位照常載入但不起作用，
// health 也不再回報；沒在組字時打的符號留在組字區，英文照混打送出。
func TestRetiredSymbolSettingsDoNotApplyInServer(t *testing.T) {
	binary := buildServer(t)
	dir := t.TempDir()
	data, _ := filepath.Abs("../../testdata/toy")
	b, _ := json.Marshal(map[string]any{"selection": "statistical", "punctuation": false, "shift_symbol_halfwidth": false,
		"all_symbols_halfwidth": false, "ctrl_punctuation": false, "toneless": true, "mixed_input": false})
	if err := os.WriteFile(filepath.Join(dir, settings.FileName), b, 0600); err != nil {
		t.Fatal(err)
	}
	sock := testPipe("hardcoded")
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
	for _, retired := range []string{"all_symbols_halfwidth", "shift_symbol_halfwidth", "ctrl_punctuation", "toneless", "mixed_input"} {
		if _, ok := info[retired]; ok {
			t.Fatalf("health 還回報 %s：%+v", retired, info)
		}
	}
	if info["ctrl_punctuation_keys"] != ",.';?!:()[]" || !strings.Contains(info["idle_keys"], "[") || !strings.Contains(info["idle_keys"], "(") {
		t.Fatalf("health：%+v", info)
	}
	for _, tc := range []struct {
		keys []string
		want string
	}{
		{[]string{"[", "_", proto.KeyCtrlComma, "("}, "[_，("},
		{[]string{proto.KeyCtrlParenLeft}, "（"},
		{[]string{proto.KeyCtrlBracketLeft}, "【"},
		{[]string{"h", "e", "l", "l", "o"}, "hello"},
	} {
		id := call(ipc.Envelope{Op: ipc.OpOpen, Version: proto.Version}).Session
		for _, k := range tc.keys {
			if snap := call(ipc.Envelope{Op: ipc.OpHandle, Session: id, Req: &proto.Request{Type: proto.Key, Key: k}}).Snapshot; snap == nil || !snap.Handled {
				t.Fatalf("%q 的 %q 沒有處理：%+v", tc.keys, k, snap)
			}
		}
		out := call(ipc.Envelope{Op: ipc.OpHandle, Session: id, Req: &proto.Request{Type: proto.Commit}}).Snapshot
		if out == nil || out.Commit != tc.want {
			t.Fatalf("%q：%+v，要 %s", tc.keys, out, tc.want)
		}
	}
}
