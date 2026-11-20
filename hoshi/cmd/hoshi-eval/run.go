package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/internal/atomicfile"
)

type Run struct {
	Label   string      `json:"label"`
	Server  string      `json:"server"`
	Version string      `json:"version"`
	Data    string      `json:"data"`
	DataSHA string      `json:"data_sha256,omitempty"`
	Repeat  int         `json:"repeat"`
	Started time.Time   `json:"started"`
	Neural  []string    `json:"neural,omitempty"` // 每個設定的引擎結束時印的神經統計
	Sets    []SetResult `json:"sets"`
}

type SetResult struct {
	Name    string       `json:"name"`
	Profile string       `json:"profile"`
	Match   string       `json:"match,omitempty"`
	Cases   []CaseResult `json:"cases"`
}

type CaseResult struct {
	Case
	Got string `json:"got"`
	// Fallback：這一句打完時神經選字不在可用狀態（逾時被停掉、還在重新載入），結果是統計選字。
	Fallback string `json:"fallback,omitempty"`
	// Unstable：重跑時畫面跟第一次不一樣（記下不一樣的那次）。
	Unstable string `json:"unstable,omitempty"`
	Steps    []Step `json:"steps"`
}

// Step 是一個按鍵。沒送進引擎的鍵（外殼直接交給應用程式）Sent 為 false、沒有耗時。
type Step struct {
	Key      string `json:"key"`
	Sent     bool   `json:"sent,omitempty"`
	Missed   bool   `json:"missed,omitempty"`    // {pick:…} 沒找到那一項
	EngineUS int64  `json:"engine_us,omitempty"` // 含等核心的鎖；{pick:…} 翻頁再選時取最慢的一次
	Text     string `json:"text"`                // 這一鍵之後畫面上的文字
}

func runCommand(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	server := fs.String("server", "", "0c-server 執行檔（官方：已安裝版本目錄裡的，或從官方 tag 建置；fork：go build ./cmd/0c-server）")
	data := fs.String("data", "", "資料包目錄；旁邊要有配對的 neural/（例如已安裝版本的 data）")
	profiles := fs.String("profiles", "hoshi/eval/profiles", "設定檔目錄（語料檔頭的 profile 對應 <名稱>.json）")
	out := fs.String("out", "", "結果 JSON")
	label := fs.String("label", "", "報告上的名稱（預設用引擎版本）")
	threads := fs.Int("neural-threads", 0, "神經模型 CPU threads（0 用引擎預設）")
	statistical := fs.Bool("allow-statistical", false, "神經選字載入失敗時照樣用統計選字跑（預設中止）")
	repeat := fs.Int("repeat", 3, "每句跑幾次；耗時取每鍵最小值，濾掉電腦忙造成的突波")
	fs.Parse(args)
	if *server == "" || *data == "" || *out == "" || fs.NArg() == 0 || *repeat < 1 {
		return fmt.Errorf("要指定 -server、-data、-out 與語料檔（-repeat 至少 1）")
	}
	var corpora []Corpus
	for _, path := range fs.Args() {
		c, err := readCorpus(path)
		if err != nil {
			return err
		}
		corpora = append(corpora, c)
	}
	run := Run{Label: *label, Server: *server, Data: *data, Repeat: *repeat, Started: time.Now()}
	// 每個設定開一個引擎：設定檔在引擎啟動時讀。
	for _, profile := range profileOrder(corpora) {
		eng, err := startEngine(*server, *data, filepath.Join(*profiles, profile+".json"), *threads, *statistical)
		if err != nil {
			return fmt.Errorf("設定 %s：%w", profile, err)
		}
		run.Version, run.DataSHA = eng.info["version"], eng.info["data_sha256"]
		for _, c := range corpora {
			if c.Profile != profile {
				continue
			}
			set := SetResult{Name: c.Name, Profile: c.Profile, Match: c.Match}
			for i, k := range c.Cases {
				r, err := eng.run(k)
				for j := 1; j < *repeat && err == nil; j++ {
					var again CaseResult
					if again, err = eng.run(k); err == nil {
						r.merge(again)
					}
				}
				if err != nil {
					eng.stop()
					return fmt.Errorf("%s %s：%w", c.Name, k.ID, err)
				}
				set.Cases = append(set.Cases, r)
				if (i+1)%100 == 0 {
					fmt.Fprintf(os.Stderr, "%s：%d/%d\n", c.Name, i+1, len(c.Cases))
				}
			}
			run.Sets = append(run.Sets, set)
		}
		run.Neural = append(run.Neural, profile+"："+eng.stop())
	}
	if run.Label == "" {
		run.Label = run.Version
	}
	return atomicfile.WriteJSON(*out, run)
}

// merge 併進同一句重跑的結果：每一鍵的耗時取最小值。
func (r *CaseResult) merge(o CaseResult) {
	if o.Got != r.Got && r.Unstable == "" {
		r.Unstable = o.Got
	}
	if r.Fallback == "" {
		r.Fallback = o.Fallback
	}
	for i := range min(len(r.Steps), len(o.Steps)) {
		if r.Steps[i].Sent == o.Steps[i].Sent {
			r.Steps[i].EngineUS = min(r.Steps[i].EngineUS, o.Steps[i].EngineUS)
		}
	}
}

func profileOrder(cs []Corpus) []string {
	var order []string
	seen := map[string]bool{}
	for _, c := range cs {
		if !seen[c.Profile] {
			seen[c.Profile] = true
			order = append(order, c.Profile)
		}
	}
	return order
}

type engine struct {
	cmd      *exec.Cmd
	conn     net.Conn
	dir      string
	stderr   *bytes.Buffer
	info     map[string]string
	protocol int
	neural   bool // 開始時神經選字就緒
	exited   chan struct{}
}

// startEngine 在暫存目錄開一個獨立的引擎：空白的個人資料、設定檔照 profile 但一律不學習
// （personal_learning=false；神經選字照常，private 會連神經一起關掉所以不用），自己的管道，不碰使用者的引擎。
func startEngine(server, data, profile string, threads int, statistical bool) (*engine, error) {
	settings, err := os.ReadFile(profile)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(settings, &m); err != nil {
		return nil, fmt.Errorf("%s：%w", profile, err)
	}
	m["personal_learning"] = false
	if settings, err = json.Marshal(m); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "hoshi-eval-")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), settings, 0o600); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	sock := socketPath(dir)
	// -idle-exit：hoshi-eval 被強制結束時，引擎沒有連線一分鐘後自己收掉。
	args := []string{"-data", data, "-user", filepath.Join(dir, "user.json"), "-socket", sock, "-install-watch", "off", "-idle-exit", "1m"}
	if threads > 0 {
		args = append(args, "-neural-threads", strconv.Itoa(threads))
	}
	e := &engine{cmd: exec.Command(server, args...), dir: dir, stderr: &bytes.Buffer{}, exited: make(chan struct{})}
	e.cmd.Stderr = e.stderr
	if err := e.cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	go func() { e.cmd.Wait(); close(e.exited) }()
	fail := func(err error) (*engine, error) {
		e.stop()
		return nil, fmt.Errorf("%w\n引擎輸出：\n%s", err, e.stderr.String())
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		if e.conn, err = dial(sock); err == nil {
			break
		}
		select {
		case <-e.exited:
			return fail(errors.New("引擎還沒開好管道就結束了"))
		default:
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("30 秒內連不上引擎：%w", err))
		}
	}
	if e.info, err = e.waitNeural(60 * time.Second); err != nil {
		return fail(err)
	}
	state := e.info["selection_state"]
	if state != "ready" && !statistical {
		return fail(fmt.Errorf("神經選字沒有就緒（%s），結果會是統計選字；確定要這樣跑時加 -allow-statistical", state))
	}
	e.neural = state == "ready"
	e.protocol, _ = strconv.Atoi(e.info["protocol"])
	return e, nil
}

func (e *engine) call(env ipc.Envelope) (ipc.Reply, error) {
	var r ipc.Reply
	e.conn.SetDeadline(time.Now().Add(time.Minute)) // 引擎卡住時報錯，不要整個評測跟著卡住
	if err := ipc.WriteFrame(e.conn, env); err != nil {
		return r, err
	}
	if err := ipc.ReadFrame(e.conn, &r); err != nil {
		return r, err
	}
	if !r.OK {
		return r, errors.New(r.Error)
	}
	return r, nil
}

func (e *engine) health() (map[string]string, error) {
	r, err := e.call(ipc.Envelope{Op: ipc.OpHealth})
	return r.Info, err
}

// waitNeural 等神經選字載入完成，回傳最後一次的 health info。
func (e *engine) waitNeural(limit time.Duration) (map[string]string, error) {
	for deadline := time.Now().Add(limit); ; time.Sleep(200 * time.Millisecond) {
		info, err := e.health()
		if err != nil {
			return nil, err
		}
		if s := info["selection_state"]; s != "loading" && s != "fallback" || time.Now().After(deadline) {
			return info, nil
		}
	}
}

var neuralStats = regexp.MustCompile(`神經統計[^\n]*`)

// stop 請引擎存檔結束（drain），收掉暫存目錄；回傳引擎印的神經統計。
func (e *engine) stop() string {
	if e.conn != nil {
		e.call(ipc.Envelope{Op: ipc.OpDrain})
		e.conn.Close()
	}
	select {
	case <-e.exited:
	case <-time.After(10 * time.Second):
		e.cmd.Process.Kill()
		<-e.exited
	}
	os.RemoveAll(e.dir)
	return neuralStats.FindString(e.stderr.String())
}

// run 跑一句。神經選字在這一句裡被停掉（逾時）時叫回來再跑一次，還是不行就照統計選字的結果記下。
func (e *engine) run(c Case) (CaseResult, error) {
	for try := 0; ; try++ {
		r, err := e.replay(c)
		if err != nil {
			return r, err
		}
		info, err := e.health()
		if err != nil {
			return r, err
		}
		if info["selection_state"] == "ready" {
			return r, nil
		}
		r.Fallback = info["selection_state"]
		if try > 0 || !e.neural {
			return r, nil
		}
		if err := e.revive(); err != nil {
			return r, err
		}
	}
}

// revive 叫回被停掉的神經選字：引擎只在要評分、而且過了冷卻時間時才重新啟動它，所以停著的時候每秒打一次要評分的詞；
// 載入中不打，免得剛載好就又逾時被停掉。
func (e *engine) revive() error {
	poke := Case{Keys: "su3cl3"} // 「你好」：兩個音節才會評分
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(time.Second) {
		info, err := e.health()
		if err != nil {
			return err
		}
		switch info["selection_state"] {
		case "ready":
			return nil
		case "fallback":
			if _, err := e.replay(poke); err != nil {
				return err
			}
		}
	}
	return errors.New("神經選字被停掉後一分鐘內沒有恢復（引擎一小時最多重新啟動 30 次）")
}

// replay 開一個新的 session 照外殼的方式送按鍵，打完不送出、關掉 session。
func (e *engine) replay(c Case) (CaseResult, error) {
	keys, err := parseKeys(c.Keys)
	if err != nil {
		return CaseResult{}, err
	}
	r, err := e.call(ipc.Envelope{Op: ipc.OpOpen, Version: e.protocol})
	if err != nil {
		return CaseResult{}, err
	}
	session := r.Session
	defer e.call(ipc.Envelope{Op: ipc.OpClose, Session: session})
	sh := newShell(r.Info, func(req proto.Request) (proto.Snapshot, int64, error) {
		r, err := e.call(ipc.Envelope{Op: ipc.OpHandle, Session: session, Req: &req})
		if err != nil {
			return proto.Snapshot{}, 0, err
		}
		if r.Snapshot == nil {
			return proto.Snapshot{}, 0, errors.New("引擎沒有回快照")
		}
		return *r.Snapshot, r.EngineUS, nil
	})
	if _, err := sh.send(proto.Request{Type: proto.Context, ContextEnabled: true}); err != nil {
		return CaseResult{}, err
	}
	res := CaseResult{Case: c}
	for _, k := range keys {
		st, err := sh.press(k)
		if err != nil {
			return res, err
		}
		res.Steps = append(res.Steps, st)
	}
	res.Got = sh.text()
	return res, nil
}

// shellCtrlKeys 是 Windows 外殼認得的 Ctrl＋標點（platforms/windows/tsf/src/keys.rs 的 ctrl_punct_name）；
// 不在這裡的 Ctrl 鍵引擎認得也送不到。
const shellCtrlKeys = ",.';?!:()"

// shell 模擬 Windows 外殼（platforms/windows/tsf/src/service.rs 的 wants_key、commit_before_host_key、
// confirm_document_context）決定哪些鍵送進引擎、哪些交給應用程式，並模擬應用程式裡的文字與游標。
type shell struct {
	handle      func(proto.Request) (proto.Snapshot, int64, error)
	idle        string
	idleFull    string
	ctrlOn      bool
	ctrlKeys    string
	shiftSpace  string
	snap        proto.Snapshot
	doc         []rune
	caret       int
	contextID   uint64 // 最近送出的字等著外殼確認的代號
	contextText string
}

func newShell(info map[string]string, handle func(proto.Request) (proto.Snapshot, int64, error)) *shell {
	return &shell{handle: handle, idle: info["idle_keys"], idleFull: info["idle_keys_full_width"],
		ctrlOn: info["ctrl_punctuation"] == "true", ctrlKeys: info["ctrl_punctuation_keys"],
		shiftSpace: info["input_style_shift_space"], snap: proto.Snapshot{Mode: proto.ModeZhuyin}}
}

// send 送一個請求進引擎，收下快照與送出的字。
func (s *shell) send(req proto.Request) (int64, error) {
	snap, us, err := s.handle(req)
	if err != nil {
		return 0, err
	}
	s.snap = snap
	if snap.Commit != "" {
		s.insert(snap.Commit)
		s.contextID, s.contextText = snap.ContextID, snap.Commit
	}
	return us, nil
}

func (s *shell) composing() bool { return len(s.snap.Preedit) > 0 || s.snap.Candidates != nil }

func (s *shell) wants(key string) bool {
	english := s.snap.Mode == proto.ModeEnglish
	if c, ok := ctrlChar(key); ok {
		// 最早的六個鍵不看 ctrl_punctuation_keys（keys.rs 的 ctrl_punct_known）
		return !english && s.ctrlOn && strings.IndexByte(shellCtrlKeys, c) >= 0 && strings.IndexByte(",.';?!"+s.ctrlKeys, c) >= 0
	}
	if key == proto.KeyShiftSpace || s.snap.FullWidth && fullWidthExtra(key) {
		return true
	}
	if s.composing() {
		return !english || key != proto.KeyDown
	}
	if english {
		return false
	}
	if _, ok := proto.KeypadChar(key); ok {
		return s.snap.FullWidth
	}
	if key == " " || namedKeys[key] {
		return false
	}
	if s.snap.FullWidth {
		return strings.Contains(s.idleFull, key)
	}
	return strings.Contains(s.idle, key)
}

// fullWidthExtra 是全形時有沒有在組字都送進引擎的鍵（session.rs 的 full_width_extra_key）。
func fullWidthExtra(key string) bool {
	return len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || strings.Contains(` \'"<>?`, key))
}

func (s *shell) press(key string) (Step, error) {
	if key == proto.KeyShiftSpace && s.shiftSpace == "disabled" {
		key = " " // 風格停用 Shift＋空白時外殼當成一般空白
	}
	if text, ok := strings.CutPrefix(key, pickPrefix); ok {
		return s.pick(key, text)
	}
	st := Step{Key: key}
	if !s.wants(key) {
		// 引擎不收的 Ctrl 組合鍵：外殼先送出組字，再交給應用程式
		if _, ctrl := ctrlChar(key); ctrl && s.composing() {
			us, err := s.send(proto.Request{Type: proto.Commit})
			if err != nil {
				return st, err
			}
			st.Sent, st.EngineUS = true, us
		}
		s.toApp(key)
		st.Text = s.text()
		return st, nil
	}
	if err := s.confirmContext(); err != nil {
		return st, err
	}
	us, err := s.send(proto.Request{Type: proto.Key, Key: key})
	if err != nil {
		return st, err
	}
	st.Sent, st.EngineUS = true, us
	if !s.snap.Handled {
		s.toApp(key)
	}
	st.Text = s.text()
	return st, nil
}

// confirmContext：新組字的第一鍵之前，游標前面還是剛送出的字，才請引擎拿它當前文。
func (s *shell) confirmContext() error {
	if s.composing() || s.contextID == 0 {
		return nil
	}
	id := s.contextID
	s.contextID = 0
	if !strings.HasSuffix(string(s.doc[:s.caret]), s.contextText) {
		return nil
	}
	_, err := s.send(proto.Request{Type: proto.Context, On: true, ContextID: id})
	return err
}

func (s *shell) pick(key, text string) (Step, error) {
	st := Step{Key: key}
	for tries := 0; s.snap.Candidates != nil && tries <= s.snap.Candidates.Pages; tries++ {
		c := s.snap.Candidates
		i := slices.IndexFunc(c.Items, func(it proto.CandidateItem) bool { return it.Text == text })
		req := proto.Request{Type: proto.Key, Key: proto.KeyPageDown}
		if i >= 0 {
			req = proto.Request{Type: proto.Select, Index: c.Items[i].ID}
		} else if c.Page+1 >= c.Pages {
			break
		}
		us, err := s.send(req)
		if err != nil {
			return st, err
		}
		st.Sent, st.EngineUS = true, max(st.EngineUS, us)
		if i >= 0 {
			st.Text = s.text()
			return st, nil
		}
	}
	st.Missed, st.Text = true, s.text()
	return st, nil
}

// toApp 是應用程式收到原始按鍵，照一般文字框改文字、移游標。
func (s *shell) toApp(key string) {
	switch key {
	case proto.KeyEnter:
		s.insert("\n")
	case proto.KeyBackspace:
		if s.caret > 0 {
			s.caret--
			s.doc = slices.Delete(s.doc, s.caret, s.caret+1)
		}
	case proto.KeyDelete:
		if s.caret < len(s.doc) {
			s.doc = slices.Delete(s.doc, s.caret, s.caret+1)
		}
	case proto.KeyLeft:
		s.caret = max(0, s.caret-1)
	case proto.KeyRight:
		s.caret = min(len(s.doc), s.caret+1)
	case proto.KeyHome:
		for s.caret > 0 && s.doc[s.caret-1] != '\n' {
			s.caret--
		}
	case proto.KeyEnd:
		for s.caret < len(s.doc) && s.doc[s.caret] != '\n' {
			s.caret++
		}
	default:
		if c, ok := proto.KeypadChar(key); ok {
			s.insert(string(c))
		} else if len(key) == 1 {
			s.insert(key)
		}
	}
}

func (s *shell) insert(text string) {
	r := []rune(text)
	s.doc = slices.Insert(s.doc, s.caret, r...)
	s.caret += len(r)
}

func (s *shell) text() string {
	return string(s.doc[:s.caret]) + s.snap.Text() + string(s.doc[s.caret:])
}
