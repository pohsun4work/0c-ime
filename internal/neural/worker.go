// Package neural 是本機 ONNX worker 的有界宿主。任何錯誤停用 worker，保留統計引擎。
package neural

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/0c-biggo/0c-ime/core/rerank"
	"github.com/0c-biggo/0c-ime/core/zhuyin"
)

const frameLimit = 65536

var ErrUnavailable = errors.New("neural worker unavailable")
var ErrTimeout = errors.New("neural deadline exceeded")

// StartupError 是 worker 沒有回報就緒的原因，給狀態摘要與設定程式顯示（#80：精簡版 Windows 缺 C++ 執行階段時，
// 使用者原本只看到「模型無法使用」）。Reason 是固定代碼，不含輸出內容。
type StartupError struct {
	Reason, what string
	err          error // 啟動行程本身的錯誤（例如 Python 不存在）；只給紀錄，不含 worker 的輸出
}

func (e *StartupError) Error() string {
	if e.err != nil {
		return e.what + " startup failed: " + e.Reason + ": " + e.err.Error()
	}
	return e.what + " startup failed: " + e.Reason
}

func (e *StartupError) Unwrap() error { return e.err }

const (
	ReasonMissingRuntime = "missing_runtime" // Python 擴充模組載入失敗：缺少系統 DLL
	ReasonTimeout        = "timeout"         // 沒有在時限內回報就緒
	ReasonFailed         = "failed"          // 其他原因：結束、回應不符
	// ReasonRuntimeMissing：找不到內附的 Python，或它匯入不到 numpy、onnxruntime（安裝不完整）。
	// health 的狀態直接報 runtime_missing，不當成模型故障。
	ReasonRuntimeMissing = "runtime_missing"
)

// startupStderr 只收集 worker 回報就緒前的 stderr：此時還沒送出任何輸入文字，內容只會是載入錯誤；
// 就緒後的輸出一律丟棄（traceback 可能含使用者文字）。只留前 8 KiB，用來判斷原因，不寫入紀錄。
type startupStderr struct {
	mu   sync.Mutex
	buf  []byte
	done bool
}

func (s *startupStderr) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.done {
		s.buf = append(s.buf, p[:min(len(p), 8192-len(s.buf))]...)
	}
	return len(p), nil
}

// finish 停止收集並回傳已收集的內容。
func (s *startupStderr) finish() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	text := string(s.buf)
	s.buf = nil
	return text
}

// awaitExit 等 worker 結束、stderr 讀完再判斷原因；有子行程握著管線時不無限等待。
func awaitExit(done <-chan struct{}) {
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

// startupError 依 exchange 的錯誤與 worker 的 stderr 分類；done 關閉後 stderr 才完整。
func startupError(what string, err error, stderr string) *StartupError {
	switch {
	case strings.Contains(stderr, "DLL load failed"):
		return &StartupError{Reason: ReasonMissingRuntime, what: what}
	case strings.Contains(stderr, "No module named 'numpy'") || strings.Contains(stderr, "No module named 'onnxruntime'"):
		return &StartupError{Reason: ReasonRuntimeMissing, what: what}
	case errors.Is(err, ErrTimeout):
		return &StartupError{Reason: ReasonTimeout, what: what}
	}
	return &StartupError{Reason: ReasonFailed, what: what}
}

// launchError 分類啟動 worker 行程本身（exec）的錯誤：Python 執行檔不存在（安裝不完整）是 runtime_missing；其他錯誤照原樣回傳，歸為 failed。只用在 cmd.Start：manifest、model.onnx、
// vocab.json 讀不到同樣是 fs.ErrNotExist，那是模型檔的問題，不是缺推論環境（#181）。
func launchError(what string, err error) error {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return &StartupError{Reason: ReasonRuntimeMissing, what: what, err: err}
	}
	return err
}

// startupReason 回傳 StartupError 的代碼；其他錯誤（含模型檔讀不到）歸為 failed。
func startupReason(err error) string {
	var startup *StartupError
	if errors.As(err, &startup) {
		return startup.Reason
	}
	return ReasonFailed
}

// RuntimeMissingRetry 是缺少推論環境時重試啟動 worker 的間隔：重新安裝補齊後不必重啟引擎，下一次重試
// 就會載入；間隔夠長，缺元件時不會每幾秒啟動一次 Python。
const RuntimeMissingRetry = 10 * time.Minute

// retryCooldown 是啟動失敗後到下一次重試的冷卻。
func retryCooldown(reason string, normal time.Duration) time.Duration {
	if reason == ReasonRuntimeMissing {
		return RuntimeMissingRetry
	}
	return normal
}

type Config struct {
	Python, Script, Package, DictionarySHA256 string
	Threads                                   int
	Timeout                                   time.Duration
}
type Manifest struct {
	AllowPartial     bool              `json:"allow_partial"`
	Schema           string            `json:"schema"`
	ContextPolicy    string            `json:"context_policy"`
	DictionarySHA256 string            `json:"dictionary_sha256"`
	Weight           float64           `json:"weight"`
	Margin           float64           `json:"margin"`
	HintGap          float64           `json:"hint_gap"`
	TextWeight       float64           `json:"text_weight"`
	TextMargin       float64           `json:"text_margin"`
	NeutralExpand    float64           `json:"neutral_expand_penalty"`
	NeutralTolerant  []string          `json:"neutral_tolerant_last"`
	Files            map[string]string `json:"files"`
	// TonePolicy 只在 v2 套件出現（zuin-tone-mask-v1），loadManifest 依 rerank.ValidTonePolicy 核對。
	// 桌面目前不依它開未知聲調，見 desktopUnknownTones。
	TonePolicy *rerank.TonePolicy `json:"tone_policy,omitempty"`

	bases map[zhuyin.Syllable]bool // vocab.json 的音節基底，loadManifest 核對雜湊後讀出
	chars map[rune]bool            // vocab.json 的字表，同上；含表外字的候選在本機拿掉，不讓整批被拒
}

// desktopUnknownTones：桌面還沒評估過未知聲調，v2 套件照常載入（打了聲調的段落照樣神經選字；tone0-only 與 PF512
// 只差未知聲調那一列，打聲調的分數相同），但不開 rerank.Options.UnknownTones，沒打聲調的段落照統計引擎。
// 手機外殼自己用 rerank.UnknownTonesDeclared 決定。桌面評估過要開時，改成 rerank.UnknownTonesDeclared(m.Schema, m.TonePolicy)。
func (m Manifest) desktopUnknownTones() bool { return false }

// options 是套件宣告的融合設定；Worker 與 Supervisor 共用。權重照 manifest（PF512 為 2.0），不設 ContextlessWeight；
// Bases 讓字表外的讀音在本機就照統計引擎；Chars 讓含字表外字元的候選在本機拿掉。
// 有 text task 的序列模型套件另給 Texts（混打切法比較）；worker 就緒時沒宣告 rerank.ModeTexts 的話，ScoreTexts 在本機就回錯誤、照規則。
func (m Manifest) options(scorer rerank.Scorer) rerank.Options {
	o := rerank.Options{Scorer: scorer, Weight: m.Weight, Margin: m.Margin, AllowPartial: m.AllowPartial,
		ContextRunes: contextRunes(m), HintGap: m.HintGap, TextWeight: m.TextWeight, TextMargin: m.TextMargin,
		NeutralExpand: m.NeutralExpand, NeutralBases: neutralBases(m),
		UnknownTones: m.desktopUnknownTones(), Bases: m.bases, Chars: m.chars}
	if t, ok := scorer.(rerank.TextsScorer); ok && m.textsMode() {
		o.Texts = t
	}
	return o
}

// textsMode：texts 評分用 text task 的版面（不放讀音），套件要有前文與 text task。
func (m Manifest) textsMode() bool { return contextRunes(m) > 0 && m.TextWeight > 0 }

// supports 是送出前的檢查（rerank.Options.Supports）：桌面不送 tone_known，字表外的基底與字不送。不建 NeutralBases，每次請求都走。
func (m Manifest) supports(r rerank.Request) bool {
	return rerank.Options{UnknownTones: m.desktopUnknownTones(), Bases: m.bases, Chars: m.chars}.Supports(r)
}

// neutralBases 只在套件宣告 neutral_expand_penalty 時啟用候選擴充；清單為一聲（無聲調符號）的基底。
func neutralBases(m Manifest) map[zhuyin.Syllable]bool {
	if m.NeutralExpand <= 0 {
		return nil
	}
	out := map[zhuyin.Syllable]bool{}
	for _, y := range m.NeutralTolerant {
		out[zhuyin.Syllable(y).Base()] = true
	}
	return out
}

type Stats struct {
	Calls       int   `json:"calls"`
	Scored      int   `json:"scored"`
	Timeouts    int   `json:"timeouts"`
	Late        int   `json:"late"` // texts 評分超過呼叫端期限、照規則；worker 繼續算完，不算逾時
	Errors      int   `json:"errors"`
	Unavailable int   `json:"unavailable"`
	TotalUS     int64 `json:"total_us"`
}

func (s *Stats) add(o Stats) {
	s.Calls += o.Calls
	s.Scored += o.Scored
	s.Timeouts += o.Timeouts
	s.Late += o.Late
	s.Errors += o.Errors
	s.Unavailable += o.Unavailable
	s.TotalUS += o.TotalUS
}

type Worker struct {
	mu       sync.Mutex
	cmd      *exec.Cmd
	input    io.WriteCloser
	output   *bufio.Scanner
	dead     bool
	done     chan struct{}
	manifest Manifest
	timeout  time.Duration
	texts    bool         // 就緒時宣告支援 rerank.ModeTexts
	late     <-chan frame // 超過期限還沒讀的 texts 回覆，下一次請求前讀掉（settle）
	lateBy   time.Time    // late 的硬時限：送出後 timeout
	next     uint64
	stats    Stats
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// 套件契約：encoder 不看前文；序列模型只收外殼確認過的自身提交文字（最多 64 字）。
// v2 是同一個序列模型協定再加逐音節未知聲調（tone_policy），worker 同時收 v1 與 v2。
var contracts = map[string]string{
	"zuin-encoder-runtime-v1": "empty",
	rerank.SequenceSchemaV1:   "confirmed_left_64",
	rerank.SequenceSchemaV2:   "confirmed_left_64",
}

func validContract(m Manifest) bool {
	return m.Schema != "" && contracts[m.Schema] == m.ContextPolicy && rerank.ValidTonePolicy(m.Schema, m.TonePolicy)
}

func contextRunes(m Manifest) int {
	if m.ContextPolicy == "confirmed_left_64" {
		return rerank.MaxContextRunes
	}
	return 0
}

func Open(c Config) (*Worker, error) {
	m, err := loadManifest(c)
	if err != nil {
		return nil, err
	}
	cmd := pythonCommand(c.Python, c.Script, "--package", c.Package, "--dictionary", c.DictionarySHA256, "--threads", fmt.Sprint(c.Threads))
	return start(cmd, m, c.Timeout)
}

// loadManifest 核對設定、套件契約、字典配對與模型雜湊；不啟動 worker。
func loadManifest(c Config) (Manifest, error) {
	var m Manifest
	if c.Timeout <= 0 || c.Timeout > 100*time.Millisecond {
		return m, errors.New("neural timeout must be in (0,100ms]")
	}
	if c.Threads < 1 || c.Threads > MaxThreads {
		return m, errors.New("invalid neural threads")
	}
	b, err := os.ReadFile(filepath.Join(c.Package, "manifest.json"))
	if err != nil {
		return m, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if !validContract(m) || len(m.DictionarySHA256) != 64 || m.DictionarySHA256 != c.DictionarySHA256 || !(m.Weight > 0) || math.IsInf(m.Weight, 0) || !(m.Margin >= 0) || math.IsInf(m.Margin, 0) || !(m.HintGap >= 0) || math.IsInf(m.HintGap, 0) ||
		!(m.TextWeight >= 0) || math.IsInf(m.TextWeight, 0) || !(m.TextMargin >= 0) || math.IsInf(m.TextMargin, 0) ||
		!(m.NeutralExpand >= 0) || math.IsInf(m.NeutralExpand, 0) || m.NeutralExpand > 0 && len(m.NeutralTolerant) == 0 {
		return m, errors.New("neural manifest contract mismatch")
	}
	digest, err := fileSHA(filepath.Join(c.Package, "model.onnx"))
	if err != nil {
		return m, err
	}
	vocab, err := os.ReadFile(filepath.Join(c.Package, "vocab.json"))
	if err != nil {
		return m, err
	}
	if sum := sha256.Sum256(vocab); digest != m.Files["model.onnx"] || hex.EncodeToString(sum[:]) != m.Files["vocab.json"] {
		return m, errors.New("neural package hash mismatch")
	}
	// 讀不出音節基底或字表時不在本機檢查（Bases／Chars 為 nil），字表本身對不對照舊由 worker 啟動時判斷。
	if strings.HasPrefix(m.Schema, "zuin-sequence-runtime-") {
		if bases, e := rerank.ParseBases(vocab); e == nil {
			m.bases = bases
		}
		if chars, e := rerank.ParseChars(vocab); e == nil {
			m.chars = chars
		}
	}
	return m, nil
}

// NoTelemetryEnv 關掉 onnxruntime 的遙測，給所有會 import onnxruntime 的 Python 子行程（docs/46）。
// 內附的 onnxruntime 1.30.0 在 import 時（建立全域 Env）就啟動遙測，變數必須在那之前就在環境裡：Windows 版不連網，
// 改寫 ETW（Microsoft.ML.ONNXRuntime，交給系統的診斷資料服務），不讀 ORT_DISABLE_TELEMETRY（那是其他平台 1DS 遙測的
// 公開關閉方式，一併設定無妨）。ORT_RUNNING_UNIT_TESTS=1 讓它不註冊、不寫任何事件（1.30.0 只有遙測讀它；升級
// onnxruntime 要重新確認）。
//
// worker 腳本另外再設一次並呼叫 disable_telemetry_events()。
var NoTelemetryEnv = []string{"ORT_DISABLE_TELEMETRY=1", "ORT_RUNNING_UNIT_TESTS=1"}

// pythonCommand 以隔離模式啟動 worker：不讀使用者的 PYTHON* 環境變數與 user site；
// 不寫 bytecode（內附 Python 已預先編譯，安裝後的檔案必須維持與收據相同）；
// stdio 一律 UTF-8（Windows 管道預設是系統碼頁，中文會變亂碼）；Windows 不開主控台視窗；
// onnxruntime 不連網（NoTelemetryEnv，覆蓋使用者環境裡的同名變數）。
func pythonCommand(python, script string, args ...string) *exec.Cmd {
	cmd := exec.Command(python, append([]string{"-I", "-B", "-X", "utf8", script}, args...)...)
	cmd.Env = append(os.Environ(), NoTelemetryEnv...)
	hideWindow(cmd)
	return cmd
}
func start(cmd *exec.Cmd, m Manifest, timeout time.Duration) (*Worker, error) {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	// stdout 是協定；stderr 只在就緒前收集來判斷啟動失敗的原因（見 startupStderr）。
	stderr := &startupStderr{}
	cmd.Stderr = stderr
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, launchError("neural worker", err)
	}
	w := &Worker{cmd: cmd, input: input, output: bufio.NewScanner(output), manifest: m, timeout: timeout, done: make(chan struct{})}
	w.output.Buffer(make([]byte, 4096), frameLimit)
	go func() { _ = cmd.Wait(); close(w.done) }()
	b, err := w.await(w.send(nil), 10*time.Second)
	var ready struct {
		AllowPartial bool     `json:"allow_partial"`
		Ready        bool     `json:"ready"`
		Schema       string   `json:"schema"`
		Model        string   `json:"model_sha256"`
		Dictionary   string   `json:"dictionary_sha256"`
		Modes        []string `json:"modes"`
	}
	if err == nil {
		err = json.Unmarshal(b, &ready)
	}
	if err != nil || !ready.Ready || ready.Schema != m.Schema || ready.Model != m.Files["model.onnx"] || ready.Dictionary != m.DictionarySHA256 || ready.AllowPartial != m.AllowPartial {
		w.stop()
		awaitExit(w.done)
		return nil, startupError("neural worker", err, stderr.finish())
	}
	stderr.finish()
	w.texts = m.textsMode() && slices.Contains(ready.Modes, rerank.ModeTexts)
	return w, nil
}

type frame struct {
	b   []byte
	err error
}

// send 送出請求並在背景讀一行回覆。同一時間至多一個 I/O goroutine：上一個沒讀完的回覆要先 settle。
func (w *Worker) send(request []byte) <-chan frame {
	done := make(chan frame, 1)
	go func() {
		if request != nil {
			if _, err := w.input.Write(request); err != nil {
				done <- frame{err: err}
				return
			}
		}
		if !w.output.Scan() {
			err := w.output.Err()
			if err == nil {
				err = io.EOF
			}
			done <- frame{err: err}
			return
		}
		done <- frame{b: append([]byte(nil), w.output.Bytes()...)}
	}()
	return done
}

// await 等回覆到 timeout。逾時關閉管線並殺掉 worker，不排隊、不可晚套用。
func (w *Worker) await(done <-chan frame, timeout time.Duration) ([]byte, error) {
	if r, ok := wait(done, timeout); ok {
		return r.b, r.err
	}
	w.stop()
	return nil, ErrTimeout
}

// awaitBy 等回覆：deadline 比硬時限 hard 早時只等到 deadline，沒回就留給下一次請求（w.late）、回 rerank.ErrDeadline，
// worker 繼續算；否則等到 hard，逾時同 await。
func (w *Worker) awaitBy(done <-chan frame, deadline, hard time.Time) ([]byte, error) {
	if deadline.IsZero() || !deadline.Before(hard) {
		return w.await(done, time.Until(hard))
	}
	if r, ok := wait(done, time.Until(deadline)); ok {
		return r.b, r.err
	}
	w.late, w.lateBy = done, hard
	return nil, rerank.ErrDeadline
}

// settle 讀掉上一個超過期限的回覆（不用）。等法同 awaitBy：texts 評分的 deadline 前還沒回就繼續留著，
// worker 還在算，送新的請求只會再晚一次。
func (w *Worker) settle(deadline time.Time) error {
	late := w.late
	if late == nil {
		return nil
	}
	w.late = nil
	_, err := w.awaitBy(late, deadline, w.lateBy)
	return err
}

// wait 等 done 最多 d；時間到時再看一次剛好回來的。
func wait(done <-chan frame, d time.Duration) (frame, bool) {
	timer := time.NewTimer(max(d, 0))
	defer timer.Stop()
	select {
	case r := <-done:
		return r, true
	case <-timer.C:
		return poll(done)
	}
}

func poll(done <-chan frame) (frame, bool) {
	select {
	case r := <-done:
		return r, true
	default:
		return frame{}, false
	}
}
func (w *Worker) stop() {
	if !w.dead {
		w.dead = true
		_ = w.input.Close()
		_ = w.cmd.Process.Kill()
	}
}
func (w *Worker) Close() { w.mu.Lock(); w.stop(); w.mu.Unlock(); <-w.done }

func (w *Worker) Options() rerank.Options { return w.manifest.options(w) }

func (w *Worker) Stats() Stats { w.mu.Lock(); defer w.mu.Unlock(); return w.stats }
func (w *Worker) Dead() bool   { w.mu.Lock(); defer w.mu.Unlock(); return w.dead }
func (w *Worker) Score(r rerank.Request) ([]float64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	supported := w.manifest.supports(r) && r.AllowPartial == w.manifest.AllowPartial && (r.Mode != rerank.ModeText || w.manifest.textsMode())
	if contextRunes(w.manifest) == 0 {
		r.Context = "" // 未宣告前文的套件永遠收不到文件文字
	}
	return w.score(supported, func(e envelope) any {
		return struct {
			rerank.Request
			envelope
		}{r, e}
	}, r.Revision, len(r.Candidates), time.Time{})
}

// ScoreTexts 是 rerank.TextsScorer。deadline 內沒回就回 rerank.ErrDeadline、照規則，worker 繼續算完，
// 回覆在下一次請求前讀掉丟棄；從送出算起超過 timeout 一樣殺掉 worker。
func (w *Worker) ScoreTexts(r rerank.TextsRequest, deadline time.Duration) ([]float64, error) {
	by := time.Now().Add(deadline)
	w.mu.Lock()
	defer w.mu.Unlock()
	supported := w.texts && rerank.Options{Chars: w.manifest.chars}.SupportsTexts(r)
	return w.score(supported, func(e envelope) any {
		return struct {
			Mode         string `json:"mode"`
			AllowPartial bool   `json:"allow_partial,omitempty"`
			rerank.TextsRequest
			envelope
		}{rerank.ModeTexts, w.manifest.AllowPartial, r, e}
	}, r.Revision, len(r.Texts), by)
}

// score 需持有 w.mu：Score 與 ScoreTexts 共用的統計與檢查。supported 為 false 時在本機拒絕、不殺 worker；
// request 以這次的身分組請求。
func (w *Worker) score(supported bool, request func(envelope) any, revision, n int, deadline time.Time) ([]float64, error) {
	w.stats.Calls++
	if w.dead {
		w.stats.Unavailable++
		return nil, ErrUnavailable
	}
	if !supported {
		return nil, errors.New("unsupported neural request")
	}
	started := time.Now()
	defer func() { w.stats.TotalUS += time.Since(started).Microseconds() }()
	w.next++
	return w.roundTrip(request(w.envelope()), revision, n, deadline)
}

// envelope 是請求的身分（w.next 是這次的 ID），回覆要原樣帶回。
type envelope struct {
	ID         uint64 `json:"id"`
	Model      string `json:"model_sha256"`
	Dictionary string `json:"dictionary_sha256"`
}

func (w *Worker) envelope() envelope {
	return envelope{w.next, w.manifest.Files["model.onnx"], w.manifest.DictionarySHA256}
}

// roundTrip 需持有 w.mu：送出請求、收 n 個分數（deadline 見 awaitBy）。逾時、身分或分數不對殺掉 worker；
// worker 回報不支援的輸入時不殺。
func (w *Worker) roundTrip(request any, revision, n int, deadline time.Time) ([]float64, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	if len(b)+1 > frameLimit {
		return nil, errors.New("neural request frame too large")
	}
	if err = w.settle(deadline); err == nil {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			err = rerank.ErrDeadline // 讀掉晚到的回覆就到期限了：不送
		} else {
			b, err = w.awaitBy(w.send(append(b, '\n')), deadline, time.Now().Add(w.timeout))
		}
	}
	switch {
	case errors.Is(err, rerank.ErrDeadline):
		w.stats.Late++
		return nil, err
	case errors.Is(err, ErrTimeout):
		w.stats.Timeouts++
	case err != nil:
		w.stats.Errors++
	}
	if err != nil {
		w.stop()
		return nil, err
	}
	var reply struct {
		envelope
		Revision int       `json:"revision"`
		Scores   []float64 `json:"scores"`
		Fallback string    `json:"fallback"`
	}
	err = json.Unmarshal(b, &reply)
	if err != nil || reply.envelope != w.envelope() || reply.Revision != revision {
		w.stats.Errors++
		w.stop()
		return nil, errors.New("neural reply identity mismatch")
	}
	if reply.Fallback != "" {
		return nil, errors.New("neural unsupported input")
	}
	if len(reply.Scores) != n {
		w.stats.Errors++
		w.stop()
		return nil, errors.New("neural score count mismatch")
	}
	for _, score := range reply.Scores {
		if math.IsNaN(score) || math.IsInf(score, 0) {
			w.stats.Errors++
			w.stop()
			return nil, errors.New("invalid neural score")
		}
	}
	w.stats.Scored++
	return reply.Scores, nil
}
