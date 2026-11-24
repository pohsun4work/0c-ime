//go:debug default=go1.26

// 0c-server 是行程外的輸入法引擎（docs/19）：在 named pipe 上為 TSF DLL 提供服務。
// 每位使用者一個實例；它是個人化資料唯一的寫入者。
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/0c-biggo/0c-ime/bridge/ipc"
	"github.com/0c-biggo/0c-ime/core/engine"
	"github.com/0c-biggo/0c-ime/core/layout"
	"github.com/0c-biggo/0c-ime/core/personal"
	"github.com/0c-biggo/0c-ime/core/proto"
	"github.com/0c-biggo/0c-ime/core/session"
	"github.com/0c-biggo/0c-ime/core/vocab"
	"github.com/0c-biggo/0c-ime/core/zhconv"
	"github.com/0c-biggo/0c-ime/internal/datadir"
	"github.com/0c-biggo/0c-ime/internal/e2esync"
	"github.com/0c-biggo/0c-ime/internal/neural"
	"github.com/0c-biggo/0c-ime/internal/settings"
	"github.com/0c-biggo/0c-ime/internal/simplified"
)

// buildVersion 由打包工具以 -ldflags "-X main.buildVersion=<版本>" 寫入；開發建置為 dev。
var buildVersion = "dev"

func main() {
	defaultData, defaultUser, defaultSock := defaultPaths()
	data := flag.String("data", defaultData, "資料包目錄")
	user := flag.String("user", defaultUser, "個人資料檔")
	sock := flag.String("socket", defaultSock, "named pipe 名稱")
	legacy := flag.String("legacy-socket", legacyName(), "過渡期同時服務的舊 zuin named pipe 名稱；空值停用，指定 -socket 時預設停用")
	idle := flag.Duration("idle-exit", 0, "沒有任何連線這麼久之後自行結束（0 為不結束）")
	lockWait := flag.Duration("lock-wait", 5*time.Second, "個人資料目錄被另一個引擎鎖住時最多等這麼久（交接時舊引擎存完檔才放鎖）；逾時照常輸入但不存檔")
	checkData := flag.Bool("check-data", false, "僅驗證資料包能載入，不開啟管道、不讀寫個人詞庫")
	neuralPackage := flag.String("neural-package", "", "神經選字 ONNX 包；未指定時依 settings.json 使用資料包旁內附的 neural/select")
	neuralPython := flag.String("neural-python", "python3", "ONNX worker Python 路徑；指定時也取代內附或系統的 Python")
	neuralWorker := flag.String("neural-worker", "", "worker 腳本路徑（encoder_runtime.py 或 sequence_runtime.py）")
	neuralTimeout := flag.Duration("neural-timeout", 100*time.Millisecond, "模型決策期限，上限 100ms；逾時保留原排序")
	neuralThreads := flag.Int("neural-threads", neural.DefaultThreads(), "模型 CPU threads（1–8，預設為 CPU 邏輯核心數，最多 8）")
	punctPackage := flag.String("punct-package", "", "提交時補標點 ONNX 包；未指定時依 settings.json 使用資料包旁內附的 neural/punct")
	punctWorker := flag.String("punct-worker", "", "補標點 worker 腳本路徑（punct_runtime.py）；Python 同 -neural-python")
	punctTimeout := flag.Duration("punct-timeout", 150*time.Millisecond, "補標點期限，上限 500ms；逾時照原文提交")
	punctThreads := flag.Int("punct-threads", neural.DefaultThreads(), "補標點 CPU threads（1–8，預設為 CPU 邏輯核心數，最多 8）")
	flag.Parse()
	lg := log.New(os.Stderr, "0c-server: ", log.LstdFlags)
	if *checkData {
		if _, err := datadir.Open(*data); err != nil {
			lg.Fatal(err)
		}
		fmt.Println("資料包可載入")
		return
	}

	// 過渡期：升級當下已載入的舊外殼仍連舊名稱，因此同時服務新舊名稱。隔離測試指定 -socket 時不碰舊名稱。
	explicit := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if explicit["socket"] && !explicit["legacy-socket"] {
		*legacy = ""
	}
	names := []string{*sock}
	if *legacy != "" && *legacy != *sock {
		names = append(names, *legacy)
	}

	// 單一實例：任一名稱能連上，表示已有 server（可能是舊版 zuin-server）在跑，它才是個人資料的寫入者。
	for _, name := range names {
		if serving(name) {
			fmt.Fprintln(os.Stderr, "0c-server 已經在執行：", name)
			os.Exit(0)
		}
	}
	// 個人資料寫入者鎖（#120）：在載入資料包之前取得，同時啟動的第二個引擎因此不必白載資料包。
	if err := os.MkdirAll(filepath.Dir(*user), 0o700); err != nil {
		lg.Fatal(err)
	}
	writerLock, lockState, yield := claimWriter(filepath.Dir(*user), names, *lockWait, lg)
	if yield {
		fmt.Fprintln(os.Stderr, "0c-server 已經在執行（同時啟動的另一個引擎先完成載入）")
		os.Exit(0)
	}
	writable := lockState != lockBusy

	start := time.Now()
	eng, err := datadir.Open(*data)
	if err != nil {
		lg.Fatalf("載入資料包 %s：%v", *data, err)
	}
	// 回復到舊版後，個人詞庫可能已由新版改成本版不認得的格式：照常輸入，但不讀入也不寫回，
	// 以免誤讀欄位或丟掉新版資料。回到新版，或以 0c-update rollback -restore-user 還原後恢復學習。
	personalState := "loaded"
	var newer *personal.NewerFormatError
	if err := datadir.LoadUser(eng, *user); errors.As(err, &newer) {
		lg.Printf("個人詞庫 %s 由較新版本寫成（格式 %d），本次不讀入也不存檔：%v", *user, newer.Format, err)
		personalState = "newer_format_not_loaded"
	} else if err != nil {
		lg.Fatalf("載入個人資料：%v", err)
	}
	eng.Model.Now = func() int64 { return time.Now().Unix() }

	// 使用者設定：預設神經選字與前文開啟，補標點（Beta）關閉。明確的 -neural-*／-punct-* 參數優先（開發與隔離測試）。
	// #206：中英切換鍵預設改成左右 Shift；0.2.4 以前存檔時順手寫進去的 right 在第一次以新版啟動時換一次。
	// 放在引擎：不論是新版安裝程式、還是舊版更新器在背景裝上這一版，第一個以新設定服務的一定是新版引擎。
	if writable {
		if adopted, err := settings.AdoptBothShift(*user); err != nil {
			lg.Printf("中英切換鍵未換成左右 Shift，下次啟動再試：%v", err)
		} else if adopted {
			lg.Printf("中英切換鍵改成左右 Shift（Windows 的新預設，#206；可在設定程式改回右 Shift）")
		}
		if adopted, err := settings.AdoptHoshiSymbols(*user); err != nil {
			lg.Printf("符號預設未換成照微軟，下次啟動再試：%v", err)
		} else if adopted {
			lg.Printf("符號預設改成照微軟：標點寬度所有符號鍵打半形、內建靈犀注音的標點操作改微軟參考")
		}
	}
	settingsReport, configErr := settings.Read(settings.Path(*user))
	config := settingsReport.Settings
	if configErr != nil {
		lg.Printf("設定檔無效，使用預設值：%v", configErr)
	}
	for _, note := range settingsReport.Notes { // 較新版本寫入本版不認得的欄位或值：只有那一項退回預設
		lg.Printf("設定檔：%s", note)
	}
	eng.DocumentContextOff = !config.DocumentContext
	eng.Fuzzy = config.FuzzyConfig()
	eng.InputStyle = config.ProfileRules()
	eng.MixedInput = config.MixedInput
	eng.Toneless = config.EffectiveToneless() // #150：開啟時 session 停用混打的自動判斷；health 的 mixed_input 照設定檔回報。所有符號鍵打半形時暫停（#282）
	eng.EnglishAutocomplete = config.EnglishAutocomplete
	eng.CandidateBeforeCursor = config.CandidateCursor == "before"
	eng.EmojiShortcut = config.EmojiShortcut
	eng.EmojiEqualsFirst = config.EmojiEqualsFirst
	eng.EscKeepComposition = !config.EscClear
	eng.SpaceInsertsSpace = !config.SpaceSelects
	eng.AdvanceAfterSelect = config.AdvanceAfterSelect
	eng.CtrlPunct = config.EffectiveCtrlPunctuation()      // #168；所有符號鍵打半形時固定開（#282）
	eng.ShiftSymbolHalfwidth = config.ShiftSymbolHalfwidth // #258
	eng.AllSymbolsHalfwidth = config.AllSymbolsHalfwidth   // #282
	eng.CJKLatinSpace = config.CJKLatinSpace               // #273
	eng.ZhuyinErrorHint = config.ZhuyinErrorHint           // #300
	eng.NoLearning = !config.PersonalLearning              // #297
	eng.CapsLockEnglish = config.CapsLockEnglish           // #173

	if config.OutputSimplified { // #118：字典編在執行檔裡，只有開啟時才解析
		if conv, err := simplified.TW2S(); err != nil {
			lg.Printf("簡體字典無法載入，照樣送出繁體：%v", err)
		} else {
			eng.Simplified = conv
		}
	}
	// #169：輸入法選單（與 0c-update set -apply）即時切換「輸出簡體字」：字典第一次開啟時才解析，之後共用；
	// 新的值照 0c-update set 的寫法寫進設定檔（同一把寫入鎖、保留不認得的欄位），設定程式的開關跟著變。
	eng.SimplifiedSwitch = engine.SimplifiedSwitch{
		Load: func() (*zhconv.Converter, error) {
			conv, err := simplified.TW2S()
			if err != nil {
				lg.Printf("簡體字典無法載入，照樣送出繁體：%v", err)
			}
			return conv, err
		},
		Save: func(on bool) error {
			value := "off"
			if on {
				value = "on"
			}
			if _, err := settings.Change(*user, [2]string{"output-simplified", value}); err != nil {
				lg.Printf("輸出簡體字已切換為 %s，但設定檔沒有寫入：%v", value, err)
				return err
			}
			return nil
		},
	}
	eng.SelectionKeys = config.SelectionKeys // #112
	// 0.2.0 正式開放（#128）：引擎的欄位留著，零值給測試確認關閉時行為不變。
	eng.Preview02 = true
	eng.SharedMode = config.ModeMemory == settings.ModeShared // #115
	eng.StartEnglish = config.StartsEnglish()                 // #160
	// 自訂詞彙（#122）。檔案由較新版本寫成或讀不懂時照常輸入，只是不使用也不改寫它。
	vocabPath := datadir.VocabPath(*user)
	vocabState, vocabDisabled := "disabled", "自訂詞彙尚未開放"
	if eng.Preview02 {
		var newer *vocab.NewerFormatError
		switch err := datadir.LoadVocab(eng, vocabPath); {
		case errors.As(err, &newer):
			lg.Printf("自訂詞彙 %s 由較新版本寫成（格式 %d），本次不讀入也不修改", vocabPath, newer.Format)
			vocabState, vocabDisabled = "newer_format_not_loaded", "自訂詞彙檔由較新版本寫成，請更新到最新版再修改"
		case err != nil:
			lg.Printf("自訂詞彙無法讀取，本次不使用也不修改：%v", err)
			vocabState, vocabDisabled = "unreadable", "自訂詞彙檔無法讀取，請到問題排解匯出狀態"
		case !writable: // 另一個引擎持有個人資料鎖：照樣讀進來打字用，但不能改，改了也存不下來
			vocabState, vocabDisabled = "read_only_locked", "另一個引擎正在使用個人資料，暫時不能修改自訂詞彙"
		default:
			vocabState, vocabDisabled = "loaded", ""
			// 說明裡有詞面（姓名是私人資料），日誌只記筆數；內容由管理頁顯示（vocab status 的 notes）。
			if v := eng.VocabSnapshot(); v != nil && len(v.Problems()) > 0 {
				lg.Printf("自訂詞彙有 %d 項內容沒有使用（手動改壞或超過上限），檔案原樣保留", len(v.Problems()))
			}
		}
	}
	// 鍵盤排列（#83）：倚天 41 鍵時 session 停用混打與 = 開 Emoji；設定檔的值不動，health 照實回報。
	eng.Layout, _ = layout.ByName(config.KeyboardLayout)
	if config.CandidateLayout == "horizontal" {
		eng.CandidateColumns = config.ProfileRules().CandidatePageSize
	}
	if config.CandidateLayout == "grid" {
		eng.CandidateColumns = 5
	}
	// 模型與字典配對出貨；缺 manifest 時無法核對配對，保留統計引擎，不讓輸入法停擺。
	dictionaryManifest, manifestErr := os.ReadFile(filepath.Join(*data, "manifest.json"))
	dictionarySHA := ""
	if manifestErr == nil {
		dictionarySHA = fmt.Sprintf("%x", sha256.Sum256(dictionaryManifest))
	}
	// Python、worker 與模型分開找：Python 用安裝包內附的；模型依安裝包內建 → 個人目錄（#180）的順序；
	// 都沒有時統計選字。
	bundled := neural.Find(neural.Options{DataDir: *data, UserDir: filepath.Dir(*user), DictionarySHA256: dictionarySHA})
	python, pythonSource := bundled.Python, bundled.PythonSource
	if explicit["neural-python"] {
		python, pythonSource = *neuralPython, "custom"
		bundled.Python, bundled.PythonSource = python, pythonSource
		bundled.HasSelect, bundled.HasPunct = bundled.SelectSource != "", bundled.PunctSource != ""
	}
	if pythonSource == "" {
		pythonSource = "missing"
	}
	neuralState := "disabled_by_setting"
	var selectionSource, punctuationSource string
	var selectionWorker *neural.Supervisor
	var punctuationWorker *neural.PunctWorker
	if config.Selection == settings.Neural && !explicit["neural-package"] {
		neuralState = "not_installed"
		if bundled.HasSelect {
			*neuralPackage, *neuralWorker, *neuralPython = bundled.SelectPackage, bundled.SelectWorker, python
			selectionSource = bundled.SelectSource
			if !explicit["neural-threads"] {
				*neuralThreads = neural.DefaultThreads()
			}
		}
	}
	if config.Punctuation && !explicit["punct-package"] && bundled.HasPunct {
		*punctPackage, *punctWorker, *neuralPython = bundled.PunctPackage, bundled.PunctWorker, python
		punctuationSource = bundled.PunctSource
		if !explicit["punct-threads"] {
			*punctThreads = neural.DefaultThreads()
		}
	}
	if *neuralPackage != "" {
		workerErr := manifestErr
		var worker *neural.Supervisor
		if workerErr == nil {
			// 模型在背景載入；引擎先以統計引擎服務，外殼的第一個按鍵不必等 Python。
			worker, workerErr = neural.OpenSupervisedAsync(neural.Config{Python: *neuralPython, Script: *neuralWorker, Package: *neuralPackage,
				DictionarySHA256: dictionarySHA, Threads: *neuralThreads, Timeout: *neuralTimeout})
		}
		if workerErr != nil {
			lg.Printf("神經模型未啟用，保留統計引擎：%v", workerErr)
			neuralState = "startup_fallback"
		} else {
			worker.SetLog(lg.Printf)
			defer func() {
				worker.Close()
				lg.Printf("神經統計（無文字）：%+v 重啟 %d 次", worker.Stats(), worker.Restarts())
			}()
			eng.Rerank = worker.Options()
			selectionWorker = worker
			neuralState = "enabled"
			lg.Printf("神經選字已啟用（背景載入模型）：%s（%d threads）", filepath.Base(*neuralPackage), *neuralThreads)
		}
	}
	if *punctPackage != "" {
		pw, punctErr := neural.OpenPunct(neural.PunctConfig{Python: *neuralPython, Script: *punctWorker, Package: *punctPackage,
			Threads: *punctThreads, Timeout: *punctTimeout})
		if punctErr != nil {
			lg.Printf("補標點未啟用：%v", punctErr)
		} else {
			pw.SetLog(lg.Printf)
			defer func() {
				pw.Close()
				lg.Printf("補標點統計（無文字）：%+v", pw.Stats())
			}()
			eng.Punct = pw
			punctuationWorker = pw
			lg.Printf("補標點已啟用：%s", filepath.Base(*punctPackage))
		}
	}
	lg.Printf("已載入 %s（%v）", *data, time.Since(start).Round(time.Millisecond))

	// 存檔共用一把鎖；自訂詞彙要在本機 API 開始服務前就能存（#196），所以先定義。
	var saveMu sync.Mutex
	// 自訂詞彙在指令改了它時立即寫（IPC 與本機 API 都是）；失敗時記下。交接前與結束時，
	// 寫入失敗過、或引擎裡的版本跟最後寫入的不同（漏存的路徑）都再寫一次。
	vocabVersion := eng.VocabVersion
	var vocabDirty bool
	savedVocab := vocabVersion()
	saveVocab := func() error {
		saveMu.Lock()
		defer saveMu.Unlock()
		if vocabDisabled != "" {
			return nil
		}
		v := vocabVersion() // 寫入前讀：寫進去的只會更新，之後最多多存一次，不會漏存
		if err := datadir.SaveVocab(eng, vocabPath); err != nil {
			vocabDirty = true
			lg.Printf("自訂詞彙存檔失敗：%v", err)
			return err
		}
		vocabDirty, savedVocab = false, v
		return nil
	}
	vocabUnsaved := func() bool {
		saveMu.Lock()
		defer saveMu.Unlock()
		return vocabDisabled == "" && (vocabDirty || vocabVersion() != savedVocab)
	}

	var listeners []net.Listener
	var stopping atomic.Bool // 已決定結束（closeAll）：不再提醒或交接
	stopAPI, apiInfo := func() {}, func() map[string]string { return map[string]string{"api_state": "off"} }
	closeAll := func() {
		stopping.Store(true)
		stopAPI() // 交接時先放開 API 的埠，新引擎接手
		for _, l := range listeners {
			l.Close()
		}
	}
	for _, name := range names {
		l, err := listen(name)
		if err != nil {
			// 舊名稱被同時啟動的舊版搶先時也結束，避免兩個寫入者。
			closeAll()
			lg.Fatal(err)
		}
		listeners = append(listeners, l)
	}
	// 存檔：學習改變後標記，使用者停下來或最長間隔到了才整份重寫（save_schedule.go）；
	// 交接與結束時一定寫。存檔前每日清理一次衰減到不影響排序的項目。
	schedule := newSaveSchedule()
	// vocabEdited 在自訂詞彙指令改了詞彙後呼叫（IPC 與本機 API 都是）：立即寫 vocab.json，並標記 user.json 要存。
	// 加詞、刪詞、改讀音與復原也會改學習資料（刪詞清掉指向它的手選、更正與證據，加詞解除 tombstone、
	// 取代衝突的舊更正，#122）；只存 vocab.json 的話，之後沒打字就結束或交接，user.json 不會重寫，
	// 重啟後留著指向已刪除自訂詞的更正（#196）。只標記、不加急：組字中加詞與候選窗反悔鍵也走這裡，
	// 加急會在下一秒整份重寫 user.json，打字中途就存；交接（BeforeDrain）與結束（含結束訊號、
	// 閒置結束）一定會 save()，所以照一般學習等使用者停下來或最長間隔即可。
	vocabEdited := func() error {
		schedule.changed(time.Now(), false)
		return saveVocab()
	}
	if eng.Preview02 && vocabDisabled == "" { // 本機 API 只能動自訂詞彙：詞彙不能改時不開（#126）
		stopAPI, apiInfo = startVocabAPI(filepath.Dir(*user), eng, buildVersion, vocabEdited, lg)
	}
	if err := e2esync.Pause(filepath.Dir(*user), "writer_ready"); err != nil {
		closeAll()
		lg.Fatal(err)
	}

	save := func() error {
		saveMu.Lock()
		defer saveMu.Unlock()
		if personalState != "loaded" || !writable {
			return nil
		}
		dirty, restore := schedule.take()
		if !dirty {
			return nil
		}
		if err := datadir.SaveUser(eng, *user); err != nil {
			restore()
			lg.Printf("存檔失敗：%v", err)
			return err
		}
		return nil
	}
	srv := &ipc.Server{Engine: eng, Log: lg, VocabDisabled: vocabDisabled, OnVocab: func() { vocabEdited() },
		// 關閉「記住我選的字詞」（#297）時送出不改個人資料，不為它排存檔：user.json 不會因打字而重寫。
		// 加詞、撤銷等改了個人資料的操作照舊由 OnVocab／OnForget 排存檔。
		OnCommit: func() {
			if !eng.NoLearning {
				schedule.changed(time.Now(), false)
			}
		},
		OnForget: func() { schedule.changed(time.Now(), true) }}
	// 設定程式的「清除學習紀錄」（#301）：清空記憶體裡的個人資料並立刻存檔，不重啟引擎。
	srv.ResetPersonal = personalReset{mu: &saveMu, eng: eng, path: *user, writable: writable, state: &personalState, schedule: schedule}.run
	srv.BeforeDrain = func() error {
		if err := save(); err != nil {
			return err
		}
		if vocabUnsaved() {
			if err := saveVocab(); err != nil {
				return err
			}
		}
		return e2esync.Pause(filepath.Dir(*user), "writer_saved")
	} // 不釋放端點，直到學習存檔成功；失敗時保留原引擎供重試。
	srv.OnDrained = closeAll
	srv.Info = map[string]string{"version": buildVersion, "protocol": fmt.Sprint(proto.Version),
		"neural_startup_state": neuralState, "personal_store": personalState, "personal_lock": lockState,
		"input_profile": config.ActiveProfile().ID, "input_profile_fingerprint": config.ProfileFingerprint(),
		"input_style_shift_space": config.ProfileRules().ShiftSpace, "input_style_shift_letter": config.ProfileRules().ShiftLetter, "input_style_caps_letter": config.ProfileRules().CapsLetter,
		"selection_setting": config.Selection, "punctuation": fmt.Sprint(eng.Punct != nil), "document_context": fmt.Sprint(config.DocumentContext),
		"fuzzy": eng.Fuzzy.Key(), "mixed_input": fmt.Sprint(config.MixedInput), "english_autocomplete": fmt.Sprint(config.EnglishAutocomplete), "left_shift": fmt.Sprint(config.TogglesLeft()), "right_shift": fmt.Sprint(config.TogglesRight()), "toggle_key": config.ToggleKey, "shift_tap_ms": fmt.Sprint(config.ShiftTapMillis()), "mode_memory": config.ModeMemory, "start_mode": config.EffectiveStartMode(), "esc_clear": fmt.Sprint(config.EscClear), "space_selects": fmt.Sprint(config.SpaceSelects), "advance_after_select": fmt.Sprint(config.AdvanceAfterSelect),
		"selection_keys": config.SelectionKeys, "candidate_cursor": config.CandidateCursor, "candidate_layout": config.CandidateLayout, "emoji_shortcut": config.EmojiShortcut, "emoji_equals_first": fmt.Sprint(config.EmojiEqualsFirst),
		"keyboard_layout": config.KeyboardLayout, "preview_0_2": fmt.Sprint(eng.Preview02), "vocab_state": vocabState,
		// #123：外殼據此決定要不要把組字中的 Shift＋←／→、候選窗的 Shift＋Delete／⌫ 送給引擎。
		"vocab_keys": fmt.Sprint(eng.Preview02 && vocabDisabled == ""),
		// #135：沒在組字時的聲調鍵要交給核心（單按＋空白是聲調符號）；Windows 外殼在 TestKeyDown 依此決定。
		"tone_mark_keys": fmt.Sprint(eng.ToneMarkKeys()),
		// #183：沒在組字時引擎會處理的可見 ASCII 按鍵，依鍵盤排列與設定（許氏的數字、- / 交還應用程式）。
		// Windows 外殼的 TestKeyDown 依此決定；舊引擎沒有這一項，外殼照舊依 keyboard_layout、emoji_shortcut 與 tone_mark_keys 推算。
		"idle_keys": eng.IdleKeys(),
		// #278：全形（Shift＋空白）時沒在組字引擎會處理的可見 ASCII 按鍵（@ # * + 等打全形）。外殼在快照的 full_width
		// 為 true 時照這一項；舊引擎沒有這一項，外殼照舊只補攔大寫、空白與 \ ' " < > ?。
		"idle_keys_full_width": eng.IdleKeysFullWidth(),
		// #168：引擎認得 Ctrl＋標點的鍵名而且設定開著；外殼只在這一項是 true、又在中文模式時送那六個鍵，
		// 舊引擎沒有這一項，外殼照舊交給應用程式。
		"ctrl_punctuation": fmt.Sprint(eng.CtrlPunct),
		// #231：引擎認得的 Ctrl＋標點鍵（不跟著設定）。外殼送 Ctrl+: ( ) 前要看到這一項列了那個字元，
		// 舊引擎沒有這一項，外殼只送最早的六個鍵。
		"ctrl_punctuation_keys": proto.CtrlPunctKeys,
		// #258：Shift＋符號打半形；外殼不用看（沒在組字時交還的鍵已從 idle_keys 拿掉），0c-update 拿它跟設定檔比較。
		"shift_symbol_halfwidth": fmt.Sprint(eng.ShiftSymbolHalfwidth),
		// #282：所有符號鍵打半形；外殼不用看（交還的鍵已從 idle_keys 拿掉），0c-update 拿它跟設定檔的標點寬度比較。
		// 舊引擎沒有這一項，標點寬度選 all 時 0c-update 照舊顯示待套用。
		"all_symbols_halfwidth": fmt.Sprint(eng.AllSymbolsHalfwidth),
		// #273：中英文之間自動加空格；外殼不用看（送出的 Commit 已補好），0c-update 拿它跟設定檔比較。
		"cjk_latin_space": fmt.Sprint(eng.CJKLatinSpace),
		// #300：打錯注音時提示；外殼不用看（快照的 bell 說這一鍵要提示），0c-update 拿它跟設定檔比較。
		"zhuyin_error_hint": fmt.Sprint(eng.ZhuyinErrorHint),
		// #297：記住我選的字詞；外殼不用看（不學的是引擎），0c-update 拿它跟設定檔比較。
		"personal_learning": fmt.Sprint(!eng.NoLearning),
		// #173：引擎認得 caps_lock 請求而且設定「Caps Lock 開著時直接打英文」開著；外殼只在這一項是 true 時回報
		// Caps Lock 狀態，舊引擎沒有這一項、或設定關閉時，注音模式照舊不理 Caps Lock（#168）。
		"caps_lock_english": fmt.Sprint(eng.CapsLockEnglish),
		// #219：引擎處理 caps_lock 請求（開啟暫時切英文、關閉回到開啟前的模式，期間手動切換為準）；同 caps_lock_english。
		"caps_lock_request": fmt.Sprint(eng.CapsLockEnglish),
		// #150：不打聲調也能選字；外殼不用看，0c-update 拿它跟設定檔比較。
		"toneless": fmt.Sprint(eng.Toneless),
		// #192：設定「用快捷鍵切換簡體字」開著；外殼只在這一項是 true 時攔切換鍵，按下時送 output_simplified（同選單）。
		// 設定改了會重啟引擎，舊引擎沒有這一項，外殼照舊把這個鍵交給應用程式。
		"simplified_hotkey": fmt.Sprint(config.SimplifiedHotkey),
		// #169：引擎認得 output_simplified 請求，「輸出簡體字」可以即時切換；目前的值在 RuntimeInfo 的 output_simplified。
		// 舊引擎沒有這一項，外殼退回 0c-update set output-simplified … -apply（重啟引擎套用）。
		"output_simplified_live": "true"}
	// #181：模型從哪裡來（bundled：安裝包內建；user：個人目錄），推論用哪個 Python（bundled、system、custom、missing）。
	srv.Info["neural_python"] = pythonSource
	if selectionSource != "" {
		srv.Info["selection_source"] = selectionSource
	}
	if punctuationSource != "" {
		srv.Info["punctuation_source"] = punctuationSource
	}
	srv.Info["english_words"] = fmt.Sprint(eng.English.Len())
	srv.Info["english_pack"] = eng.EnglishPack
	if eng.EnglishPack == "" {
		srv.Info["english_pack"] = "prototype"
	}
	// 舊欄位保留啟動配置語意；另提供當下可用性，避免背景啟動失敗仍被視為模型已就緒。
	srv.RuntimeInfo = func() map[string]string {
		selectionState := neuralState
		if selectionWorker != nil {
			selectionState = selectionWorker.State()
		} else if selectionState == "startup_fallback" {
			selectionState = "fallback"
		}
		punctuationState := "not_installed"
		if !config.Punctuation {
			punctuationState = "disabled_by_setting"
		}
		if punctuationWorker != nil {
			punctuationState = punctuationWorker.State()
		} else if *punctPackage != "" {
			punctuationState = "fallback"
		}
		info := map[string]string{"selection_state": selectionState, "punctuation_state": punctuationState}
		// 「輸出簡體字」可以從選單即時切換（#169）：回報引擎目前實際的值，設定程式據此判斷有沒有套用。
		info["output_simplified"] = fmt.Sprint(eng.OutputSimplified())
		// #160：新 session 開始時的模式（全部共用時是目前共用的模式）。外殼在第一個按鍵前據此顯示中／英。
		info["initial_mode"] = proto.ModeZhuyin
		if eng.InitialEnglish() {
			info["initial_mode"] = proto.ModeEnglish
		}
		for k, v := range apiInfo() {
			info[k] = v
		}
		// 退回時附上原因代碼（例如缺少系統元件），狀態摘要與設定程式據此說明；不含輸出內容。
		if selectionState == "fallback" && selectionWorker != nil && selectionWorker.Reason() != "" {
			info["selection_reason"] = selectionWorker.Reason()
		}
		if punctuationState == "fallback" && punctuationWorker != nil && punctuationWorker.Reason() != "" {
			info["punctuation_reason"] = punctuationWorker.Reason()
		}
		return info
	}
	contextWeight := session.DefaultOptions().DocumentContextWeight
	if eng.DocumentContextWeight != nil {
		contextWeight = *eng.DocumentContextWeight
	}
	srv.Info["document_context_weight"] = fmt.Sprintf("%g", contextWeight)
	if executable, err := os.Executable(); err == nil {
		if binary, err := os.ReadFile(executable); err == nil {
			srv.Info["executable_sha256"] = fmt.Sprintf("%x", sha256.Sum256(binary))
		}
	}
	if dictionarySHA != "" {
		srv.Info["data_sha256"] = dictionarySHA
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		srv.Info["go"] = info.GoVersion
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.modified" {
				srv.Info[setting.Key] = setting.Value
			}
		}
	}
	go func() {
		lastBusy := time.Now()
		for now := range time.Tick(saveTick) {
			if schedule.due(now, srv.LastKey()) {
				save()
			}
			// 「靈犀學到的」頁的復原複本含使用者要求刪除的學習：操作完不再回來時，也在 10 分鐘後放掉（#182）。
			eng.ExpireLearnedUndo()
			if srv.Conns() > 0 {
				lastBusy = time.Now()
			} else if *idle > 0 && time.Since(lastBusy) > *idle {
				lg.Printf("閒置 %v，結束", *idle)
				closeAll()
				return
			}
		}
	}()
	sig := make(chan os.Signal, 1)
	// 結束訊號都照常存檔再結束（學習最晚一分鐘才寫入，不能直接被終止）：Ctrl+C／Ctrl+Break 由 Go 轉成 SIGINT，
	// 登出／關機的 CTRL_LOGOFF_EVENT／CTRL_SHUTDOWN_EVENT 轉成 SIGTERM；前提是引擎不載入 user32／gdi32
	// （見 logoff_windows_test.go）。
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		closeAll()
	}()

	var wg sync.WaitGroup
	for _, l := range listeners {
		wg.Add(1)
		go func(l net.Listener) {
			defer wg.Done()
			if err := srv.Serve(l); err != nil && !errors.Is(err, net.ErrClosed) {
				lg.Print(err)
			}
			// 任一名稱停止服務就整體結束，舊外殼不會留在只剩一半的 server 上。
			closeAll()
		}(l)
	}
	wg.Wait()
	save()
	if vocabUnsaved() { // 例如存檔失敗過、又不是交接（訊號或閒置結束）：放鎖前最後再試一次
		saveVocab()
	}
	// 最後一次存檔之後才放鎖（端點此時已關閉）：交接中等鎖的新引擎在這之後才讀個人檔。
	// 神經 worker 等收尾在 main 返回後才執行，不佔著鎖。
	writerLock.Release()
}
