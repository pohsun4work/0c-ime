//! 一個輸入欄位在 0c-server 上的 session：請求、重連後的重播、快照的解讀。
//! 與 Fcitx 5 外殼的 session.cpp 對應；不依賴 TSF，可單獨測試。
use std::time::{Duration, Instant};

use serde_json::{json, Value};

use crate::ipc::Client;
use crate::log::zlog;

pub const PROTOCOL_VERSION: i64 = 0;

#[derive(Clone, Debug, Default)]
pub struct Segment {
    pub text: String,
    pub style: String,
}

#[derive(Clone, Debug, Default)]
pub struct Snapshot {
    pub ok: bool,      // false：server 不可用，呼叫端應放行按鍵
    pub handled: bool, // false：把原始按鍵交還應用程式
    pub commit: String,
    pub preedit: Vec<Segment>,
    pub cursor_rune: usize,
    pub cursor_utf16: usize, // 核心直接提供 UTF-16 位置；TSF 的範圍以 UTF-16 計
    pub suggestion: String,
    pub candidates: Vec<(i64, String)>, // 目前這一頁的（ID, 文字）
    pub candidate_notes: Vec<String>, // 與 candidates 等長的低調附註（模糊音近音為「近音 ㄓ」），其餘為空字串
    /// 與 candidates 等長的鍵標（#112）：核心給的選字鍵（1、q、;）；舊核心沒有時依位置 1–0，
    /// 英文模式（只有自動補字候選，數字是文字）為空字串、不畫。
    pub candidate_keys: Vec<String>,
    pub columns: usize,
    pub candidate_kind: String,
    pub candidate_title: String,
    pub page: i64,
    pub pages: i64,
    pub highlight: i64,
    pub english: bool,
    pub full_width: bool,
    pub english_autocomplete: bool,
    pub english_pending: bool,
    pub english_can_start: bool,
    pub reconnected: bool,
    pub context_id: u64, // 本次提交可供外殼確認的代號；不是文件文字
    /// 核心這次請求的提示（例如「已加入詞庫：…」，#123）；顯示在候選窗上方，下一鍵或幾秒後消失。
    pub notices: Vec<String>,
    /// 讀音提示（#162）：中英混打時照字面顯示的數字按空白會組成的注音（20 → ㄉㄢ）；沒有時為空字串。
    pub reading_hint: String,
}

impl Snapshot {
    /// 要顯示的提示：多則時以「；」串成一行。
    pub fn notice(&self) -> String {
        self.notices.join("；")
    }

    pub fn text(&self) -> String {
        self.preedit.iter().map(|s| s.text.as_str()).collect()
    }

    /// 字寬是每個 session 的狀態；這些字元在半形可直接交給宿主，不能只看靜態 idle_keys。
    pub fn full_width_extra_key(&self, key: &str) -> bool {
        self.full_width && key.len() == 1 && (key.as_bytes()[0].is_ascii_uppercase() || " \\'\"<>?".contains(key))
    }

    /// 全形時（#278）沒在組字的數字鍵盤（KP1、KP*）：核心一律打成全形字元；半形時照舊交還宿主（#75）。
    pub fn full_width_keypad(&self, key: &str) -> bool {
        self.full_width && key.strip_prefix("KP").is_some_and(|c| c.len() == 1 && c.as_bytes()[0].is_ascii_graphic())
    }

    /// Predict only keys the English word buffer will actually consume.
    pub fn english_wants_key(&self, key: &str) -> bool {
        // 全形英文：可見字元、空白與數字鍵盤（KP1，#278）都打全形。數字鍵盤以前落到 english_key_policy 最後
        // 「先送給核心」的路：TestKeyDown、OnKeyDown 的預檢與 OnKeyDown 本身各送一次，按一下打出「１１１」。
        let literal = key.strip_prefix("KP").unwrap_or(key);
        if self.english && self.full_width && (literal.len() == 1 && literal.as_bytes()[0].is_ascii_graphic() || key == " ") {
            return true;
        }
        if !self.english_autocomplete || !self.english {
            return false;
        }
        let letter = key.len() == 1 && key.as_bytes()[0].is_ascii_alphabetic();
        if self.english_pending {
            // Enter 只送出單字、不交給宿主（#235），TestKeyDown 就要說吃，OnKeyDown 才由核心送出；
            // 在 TestKeyDown 先送出再放行的話，Windows Terminal 會連同命令一起執行。
            letter && self.text().len() < 48
                || matches!(key, "Backspace" | "Escape" | "Enter")
                || (self.english_suggesting() && matches!(key, "Tab" | "Up" | "Down" | "ShiftDelete" | "ShiftBackspace"))
        } else {
            self.english_can_start && letter
        }
    }

    pub fn composing(&self) -> bool {
        !self.preedit.is_empty() || !self.candidates.is_empty()
    }

    /// 英文單字有補字候選（#328）：引擎的反悔鍵在英文模式只作用在反白的補字候選上，這時外殼才送
    /// ShiftDelete／ShiftBackspace（service 的 vocab_keys），english_wants_key 在 TestKeyDown 就吃下。
    pub fn english_suggesting(&self) -> bool {
        self.english_pending && !self.candidates.is_empty()
    }

    /// 組字區還在標示要加入詞庫的範圍（樣式 marked，#123）：這時的提示是標示說明，留到標示結束。
    pub fn marking(&self) -> bool {
        self.preedit.iter().any(|s| s.style == "marked")
    }
}

pub struct Session {
    client: Client,
    /// 測試用：`handle` 送出的每個請求的 type，依序記下（#66）。
    #[cfg(test)]
    pub(crate) sent: Vec<String>,
    id: u64,
    generation: u32,
    english: bool,
    replay_english: bool,
    pub english_state: Snapshot,
    down: bool,      // 上次請求時 server 不可用；available() 會試著重連
    log: Vec<Value>, // 這次組字以來的請求（提交或放棄後清空）
    // 切到英文清空組字前的日誌與當時的模式；只給緊接著的放回切換接續（#90）。
    cleared: Option<(Vec<Value>, bool)>,
    toggled: bool, // 上一個操作是成功的切換；見 toggle()
    // 已確認前文：預設開啟，ZUIN_DOCUMENT_CONTEXT=0 時外殼完全不追蹤（使用者設定在引擎端）。
    context_enabled: bool,
    pending_context: Option<(u64, String)>, // 最近一次提交的代號與文字，等待外殼確認游標仍緊接其後
    // 欄位標 IS_PRIVATE（無痕／私密視窗）：每個請求帶 private，引擎不學習；不提供前文給外殼讀取。
    private: bool,
    // 引擎是否認得 #123 的新鍵名（open 回覆 info 的 vocab_keys，0.2.0 起的引擎回報 true）。
    // 跟著 session 快取，不每鍵查 health；設定套用時引擎重啟，重開 session 就會更新。
    vocab_keys: bool,
    // english 是否已是引擎那邊的模式（#160）：拿到過快照，或向引擎查過新 session 開始時的模式。新開的程式在這之前
    // 不知道開始時是不是英文，第一個按鍵前先查（learn_mode），攔不攔按鍵與中／英指示才會對。
    mode_known: bool,
    // 引擎回報「所有輸入框共用中英模式」（mode_memory=shared，#115）：換欄位後別的程式可能已經切換，重新查。
    shared: bool,
    // 沒在組字時哪些鍵交給引擎（health 的 info，見 idle_key_from，#183）與它是在哪一代連線上問到的。設定一改引擎就重啟、
    // 舊連線跟著斷，所以同一代、連線還在時沿用，不每按一個鍵就問一次 health。
    idle_info: Option<Value>,
    idle_generation: u32,
}

/// 重開 session（引擎重啟、斷線）時要不要送「設定模式」還原外殼記得的模式（#160）。新 session 從開始時的模式起：
/// 還不知道模式（新開的程式還沒問過）就照新 session 的；記得的模式跟開始時的不同才送。舊引擎沒有回報開始時的模式，
/// 當成注音，跟以前只在英文時還原相同。
fn restore_mode(known: bool, want_english: bool, start_english: bool) -> Option<bool> {
    (known && want_english != start_english).then_some(want_english)
}

/// health 回報的新 session 開始時的模式（#160 的 initial_mode；全部共用時是目前共用的模式）。舊引擎沒有這一項時為 None。
fn initial_english(info: &Value) -> Option<bool> {
    info["initial_mode"].as_str().map(|m| m == "english")
}

impl Session {
    pub fn new() -> Session {
        Session::with_client(Client::new())
    }

    /// 測試：換掉用戶端（模擬引擎斷線）。
    #[cfg(test)]
    pub(crate) fn set_client_for_test(&mut self, client: Client) {
        self.client = client;
    }

    pub fn with_client(client: Client) -> Session {
        Session {
            client,
            #[cfg(test)]
            sent: Vec::new(),
            id: 0,
            generation: 0,
            english: false,
            replay_english: false,
            english_state: Snapshot::default(),
            down: false,
            log: Vec::new(),
            cleared: None,
            toggled: false,
            context_enabled: std::env::var("ZUIN_DOCUMENT_CONTEXT").as_deref() != Ok("0"),
            pending_context: None,
            private: false,
            vocab_keys: false,
            mode_known: false,
            shared: false,
            idle_info: None,
            idle_generation: 0,
        }
    }

    /// 還不知道這個程式（TSF 執行緒）的中英模式時，向引擎查新 session 開始時的模式（#160）。
    /// spawn=false 只用已經在執行的引擎、查完不留連線（Activate 時決定中／英指示，不為此啟動引擎，
    /// 也不讓沒有打字的執行緒各自佔著一條連線）。回傳模式是否因此改變。
    pub fn learn_mode(&mut self, spawn: bool) -> bool {
        if self.mode_known {
            return false;
        }
        let probe = !spawn && !self.client.connected();
        if probe && !self.client.connect_existing() {
            return false;
        }
        let reply = self.client.call(&json!({"op": "health"}));
        if probe && self.id == 0 {
            self.client.release();
        }
        let Some(reply) = reply.filter(|r| r["ok"] == true) else {
            return false;
        };
        self.mode_known = true;
        self.shared = reply["info"]["mode_memory"] == "shared";
        let before = self.english;
        if let Some(english) = initial_english(&reply["info"]) {
            self.english = english;
        }
        before != self.english
    }

    /// 目前的 session 是否可以送 #123 的新鍵名（Shift＋←／→、Shift＋Delete／Backspace）。
    /// 還沒開 session 或引擎不支援時為 false；沒在組字時這些鍵本來就交還應用程式，不需要查。
    pub fn vocab_keys(&self) -> bool {
        self.vocab_keys && self.id != 0 && self.generation == self.client.generation()
    }

    /// 每次按鍵前由外殼依欄位政策設定；組字中途變成私人時，引擎讓整段組字保持私人。
    pub fn set_private(&mut self, private: bool) {
        self.private = private;
        if private {
            self.pending_context = None;
        }
    }

    /// 開始新組字前由外殼呼叫：回傳剛提交、等待確認的文字（每次提交只提供一次）。
    pub fn take_pending_context(&mut self) -> Option<(u64, String)> {
        if self.private {
            self.pending_context = None;
        }
        self.pending_context.take()
    }

    /// 外殼核對游標前的文字後回報；相符才請引擎以該段提交作為下一段的前文。
    pub fn confirm_context(&mut self, id: u64, confirmed: bool) {
        if !confirmed || id == 0 || self.id == 0 || self.generation != self.client.generation() {
            return;
        }
        let _ = self.client.call(
            &json!({"op": "handle", "session": self.id, "req": {"type": "context", "on": true, "context_id": id}}),
        );
    }

    pub fn english(&self) -> bool {
        self.english
    }

    /// 設定的 Shift＋空白有動作（切全半形、插入空白、開單字候選）時送 ShiftSpace；跟 idle_key 用同一份快取。
    pub fn style_shift_space(&mut self) -> bool {
        let _ = self.idle_key("");
        matches!(self.idle_info.as_ref().and_then(|v| v["shift_space"].as_str()), Some("fullwidth" | "space" | "single"))
    }

    /// 沒在組字時這個鍵要不要交給引擎（見 idle_key_from）。TestKeyDown 只查設定，不送按鍵：答錯時 WPF 會掉鍵，
    /// 或吃掉應用程式的鍵。health 的答案跟著連線快取（設定一改引擎就重啟、連線跟著斷），連線還在時不再往返。
    /// 問不到（引擎正在交接、剛重啟還沒起來、連線錯誤）時不交還宿主：OnKeyDown 的 handle() 會等交接結束再送，
    /// 這裡說不吃的字母卻直接以英文打進文件（改設定後的第一個字）。沿用上一次問到的答案；從沒問到過就照
    /// keys::starts_composition 的固定集合。不記代數，下一個鍵重新問。英文模式、Ctrl／Alt 組合不會走到這裡（wants_key）。
    pub fn idle_key(&mut self, key: &str) -> bool {
        let fresh = self.idle_info.is_some()
            && self.client.connected()
            && self.idle_generation == self.client.generation()
            && self.client.alive();
        // 字寬是 session 的狀態（Shift＋空白，#278），跟著最近一次快照；換欄位清掉 english_state 時引擎也開新的半形 session。
        let full_width = self.english_state.full_width;
        if !fresh {
            match self.client.call(&json!({"op": "health"})).filter(|r| r["ok"] == true) {
                Some(reply) => {
                    self.idle_info = Some(reply["info"].clone());
                    self.idle_generation = self.client.generation();
                }
                None => return idle_key_from(self.idle_info.as_ref().unwrap_or(&Value::Null), key, full_width),
            }
        }
        self.idle_info.as_ref().is_some_and(|info| idle_key_from(info, key, full_width))
    }

    /// 引擎是否認得 Ctrl＋標點的鍵名 `key`（health 的 ctrl_punctuation_keys，#168、#231）。
    /// 只在按下 Ctrl＋標點鍵時查詢；引擎不可用時為 false，照舊交給應用程式。
    pub fn ctrl_punctuation(&mut self, key: &str) -> bool {
        self.client.call(&json!({"op": "health"})).is_some_and(|reply| {
            reply["ok"] == true
                && crate::keys::ctrl_punct_known(key, reply["info"]["ctrl_punctuation_keys"].as_str().unwrap_or(""))
        })
    }

    /// 引擎目前載入的中英切換鍵設定（左右 Shift 與單按時限，#117）。在 Shift 單按放開時查詢，
    /// 設定套用（引擎重啟）後立即生效；引擎不可用或舊版引擎見 ShiftConfig::from_info。
    pub fn shift_config(&mut self) -> crate::keys::ShiftConfig {
        // A retired pipe must reconnect before the first Shift tap asks for
        // capabilities; otherwise call() fails once and uses legacy defaults.
        if self.client.connected() { self.client.alive(); }
        let reply = self.client.call(&json!({"op": "health"}));
        let info = reply
            .as_ref()
            .filter(|r| r["ok"] == true)
            .map(|r| &r["info"]);
        crate::keys::ShiftConfig::from_info(info)
    }

    /// server 是否可用。上次失敗後只做一次快速的連線嘗試（有速率限制，不會等啟動），
    /// 連得上才恢復吃鍵；連不上就讓宿主自己處理按鍵。
    pub fn available(&mut self) -> bool {
        if self.down && self.client.connect() {
            self.down = false;
        }
        !self.down
    }

    fn ensure_open(&mut self) -> bool {
        if self.id != 0 && self.generation == self.client.generation() {
            return true;
        }
        let reply = self
            .client
            .call(&json!({"op": "open", "version": PROTOCOL_VERSION}));
        let Some(reply) = reply else {
            self.id = 0;
            return false;
        };
        if !reply["ok"].as_bool().unwrap_or(false) {
            zlog!("open: {}", reply["error"].as_str().unwrap_or("?"));
            self.id = 0;
            return false;
        }
        self.id = reply["session"].as_u64().unwrap_or(0);
        self.generation = self.client.generation();
        self.vocab_keys = reply["info"]["vocab_keys"] == "true";
        self.shared = reply["info"]["mode_memory"] == "shared";
        let start_english = reply["info"]["start_mode"] == "english";
        self.pending_context = None; // 新 session 不認得舊代號
        if self.context_enabled {
            // 宣告外殼能確認文件位置；引擎依使用者設定決定是否採用。失敗只是不用前文。
            let _ = self.client.call(
                &json!({"op": "handle", "session": self.id, "req": {"type": "context", "context_enabled": true}}),
            );
        }
        if let Some(info) = reply["info"].as_object() {
            zlog!(
                "open: session={} server={} data={}",
                self.id,
                info.get("executable_sha256")
                    .and_then(|v| v.as_str())
                    .map(|s| &s[..s.len().min(12)])
                    .unwrap_or("?"),
                info.get("data_sha256")
                    .and_then(|v| v.as_str())
                    .map(|s| &s[..s.len().min(12)])
                    .unwrap_or("?")
            );
        }
        let want = if self.log.is_empty() {
            self.english
        } else {
            self.replay_english
        };
        if let Some(english) = restore_mode(self.mode_known, want, start_english) {
            let req = json!({"type": "mode", "on": english});
            let restored = self
                .client
                .call(&json!({"op": "handle", "session": self.id, "req": req}));
            if !restored
                .as_ref()
                .and_then(|r| r["ok"].as_bool())
                .unwrap_or(false)
            {
                self.id = 0;
                return false;
            }
        }
        // 重播：新 session 回到斷線前的組字狀態
        let log = self.log.clone();
        for req in &log {
            let r = self
                .client
                .call(&json!({"op": "handle", "session": self.id, "req": req}));
            if r.is_none() {
                self.id = 0;
                return false;
            }
        }
        true
    }

    /// 送出一個 proto.Request。連線中斷時自動重連、重開 session 並重播。
    pub fn handle(&mut self, mut request: Value) -> Snapshot {
        if self.private {
            request["private"] = Value::Bool(true); // 也記進重播紀錄，重連後仍是私人組字
        }
        let generation_before = self.client.generation();
        let english_before = self.english;
        let kind = request["type"].as_str().unwrap_or("").to_string();
        #[cfg(test)]
        self.sent.push(kind.clone());
        // 欄數不是使用者操作；其他請求都讓下一次切換不再還原注音（#90）。
        let mut cleared = None;
        if kind != "context" && kind != "display" {
            self.toggled = false;
            cleared = self.cleared.take();
        }
        let mut attempts = 0;
        // 引擎正在重啟時等到第一次看到的期限（#216）：交接、改設定後退出、當掉都一樣。宿主的 UI 執行緒等著，
        // 之後的按鍵在訊息佇列裡依序排隊，新引擎起來後照順序送到，不會亂序或重複。持續斷線不延長。
        let mut wait_until: Option<Instant> = None;
        loop {
            attempts += 1;
            if attempts > 3 {
                wait_until = wait_until.or(self.client.restart_deadline());
                if !wait_until.is_some_and(|until| Instant::now() < until) {
                    break;
                }
                std::thread::sleep(Duration::from_millis(20));
            }
            if !self.client.connect() || !self.ensure_open() {
                continue;
            }
            if !self.vocab_keys {
                // 按鍵當下的判斷用的是舊 session；重連到不支援新鍵名的引擎時換回原本的鍵。
                downgrade_vocab_key(&mut request);
            }
            let reply = self
                .client
                .call(&json!({"op": "handle", "session": self.id, "req": request}));
            let Some(reply) = reply else { continue }; // 連線斷了：下一輪重連並重播
            if !reply["ok"].as_bool().unwrap_or(false) {
                zlog!("handle: {}", reply["error"].as_str().unwrap_or("?"));
                self.id = 0; // server 不認得這個 session（例如重啟過）：重開並重播
                continue;
            }
            let mut s = parse(&reply);
            s.reconnected = generation_before != self.client.generation();
            if !s.commit.is_empty() {
                self.pending_context =
                    (self.context_enabled && !self.private && s.context_id != 0 && !s.reconnected)
                        .then(|| (s.context_id, s.commit.clone()));
            } else if s.reconnected {
                self.pending_context = None;
            }
            self.english = s.english;
            self.mode_known = true;
            self.english_state = s.clone();
            self.down = false;
            if !s.commit.is_empty() || kind == "cancel" || !s.composing() {
                // #90：切到英文清掉未打完的注音而結束組字時留下這段日誌；立即切回放回注音時接上，重播才得到同樣的組字。
                if kind == "toggle" && s.commit.is_empty() && !self.log.is_empty() {
                    let mut log = std::mem::take(&mut self.log);
                    log.push(request);
                    self.cleared = Some((log, self.replay_english));
                }
                self.log.clear();
            } else {
                if self.log.is_empty() {
                    match cleared.take().filter(|_| kind == "toggle") {
                        Some((log, english)) => {
                            self.log = log;
                            self.replay_english = english;
                        }
                        None => self.replay_english = english_before,
                    }
                }
                self.log.push(request);
            }
            return s;
        }
        zlog!("handle: server 不可用，放行按鍵");
        self.client.abandon_restart(); // 本次期限結束；下一次按鍵仍可正常恢復連線。
        self.down = true;
        Snapshot::default()
    }

    /// 清除重播紀錄；不重連、不重播，不讓舊組字流入私人欄位或下個欄位。
    pub fn reset(&mut self) {
        self.log.clear();
        self.cleared = None;
        self.toggled = false;
        self.english_state = Snapshot::default();
        self.pending_context = None;
        // 全部共用時別的程式可能已經切換：下一次重新查（learn_mode）。各程式各自記住時沿用這個程式的模式。
        if self.shared {
            self.mode_known = false;
        }
        if self.id != 0 && self.generation == self.client.generation() {
            let _ = self
                .client
                .call(&json!({"op": "close", "session": self.id}));
        }
        self.id = 0;
    }

    pub fn key(&mut self, key: &str) -> Snapshot {
        self.handle(json!({"type": "key", "key": key}))
    }

    /// 單按 Shift 或輸入指示器按鈕切換中英。上一次切換之後沒有任何按鍵或其他請求時帶 restore，
    /// 核心才放回切到英文時清掉的未打完注音（#90 防誤觸）。
    pub fn toggle(&mut self) -> Snapshot {
        let restore = self.toggled;
        let snapshot = self.handle(json!({"type": "toggle", "restore": restore}));
        self.toggled = snapshot.ok;
        snapshot
    }

    /// 沒有送給核心就交還宿主的按鍵（快捷鍵、閒置時的英文字母）同樣表示切換之後有操作。
    pub fn bypassed(&mut self) {
        self.toggled = false;
    }
}

impl Drop for Session {
    fn drop(&mut self) {
        if self.id != 0 && self.generation == self.client.generation() {
            let _ = self
                .client
                .call(&json!({"op": "close", "session": self.id}));
        }
    }
}

/// 把 #123 的新鍵名換回原本的鍵（引擎不支援時）；其他請求不變。
fn downgrade_vocab_key(request: &mut Value) {
    if request["type"] != "key" {
        return;
    }
    if let Some(base) = request["key"].as_str().and_then(crate::keys::vocab_base) {
        request["key"] = Value::from(base);
    }
}

fn parse(reply: &Value) -> Snapshot {
    let mut s = Snapshot::default();
    let Some(j) = reply.get("snapshot") else {
        return s;
    };
    s.ok = true;
    s.handled = j["handled"].as_bool().unwrap_or(false);
    s.commit = j["commit"].as_str().unwrap_or("").to_string();
    s.context_id = j["context_id"].as_u64().unwrap_or(0);
    s.suggestion = j["suggestion"].as_str().unwrap_or("").to_string();
    s.english = j["mode"].as_str() == Some("english");
    s.full_width = j["full_width"].as_bool().unwrap_or(false);
    s.english_autocomplete = j["english_autocomplete"].as_bool().unwrap_or(false);
    s.english_pending = j["english_pending"].as_bool().unwrap_or(false);
    s.english_can_start = j["english_can_start"].as_bool().unwrap_or(false);
    s.reading_hint = j["reading_hint"]["text"].as_str().unwrap_or("").to_string();
    if let Some(notices) = j["notices"].as_array() {
        s.notices = notices
            .iter()
            .filter_map(|n| n.as_str())
            .filter(|n| !n.is_empty())
            .map(str::to_string)
            .collect();
    }
    if let Some(p) = j["preedit"].as_array() {
        for g in p {
            s.preedit.push(Segment {
                text: g["text"].as_str().unwrap_or("").to_string(),
                style: g["style"].as_str().unwrap_or("normal").to_string(),
            });
        }
    }
    s.cursor_rune = j["cursor"]["rune"].as_u64().unwrap_or(0) as usize;
    s.cursor_utf16 = j["cursor"]["utf16"].as_u64().unwrap_or(0) as usize;
    if let Some(c) = j.get("candidates") {
        s.columns = c["columns"].as_u64().unwrap_or(0).min(10) as usize;
        s.candidate_kind = c["kind"].as_str().unwrap_or("").to_string();
        s.candidate_title = c["title"].as_str().unwrap_or("").to_string();
        s.page = c["page"].as_i64().unwrap_or(0);
        s.pages = c["pages"].as_i64().unwrap_or(0);
        s.highlight = c["highlight"].as_i64().unwrap_or(0);
        if let Some(items) = c["items"].as_array() {
            for it in items {
                s.candidates.push((
                    it["id"].as_i64().unwrap_or(0),
                    it["text"].as_str().unwrap_or("").to_string(),
                ));
                s.candidate_notes
                    .push(it["note"].as_str().unwrap_or("").to_string());
                let index = s.candidate_keys.len();
                s.candidate_keys.push(match it["key"].as_str() {
                    Some(key) => key.to_string(),
                    None if s.english => String::new(),
                    None => ((index + 1) % 10).to_string(),
                });
            }
        }
    }
    s
}

#[cfg(test)]
mod parse_tests {
    use super::*;

    #[test]
    fn chinese_full_width_preflight_covers_literal_keys_without_claiming_tones() {
        let snap = parse(&json!({"snapshot":{"mode":"zhuyin","full_width":true}}));
        for key in ["A", "Z", " ", "'", "\"", "\\", "<", ">", "?"] { assert!(snap.full_width_extra_key(key), "{key}"); }
        for key in ["3", "a", "Tab", "ShiftSpace"] { assert!(!snap.full_width_extra_key(key), "{key}"); }
        assert!(!Snapshot::default().full_width_extra_key("\\"));
        // #278：數字鍵盤全形時由核心打全形，半形時交還宿主。
        for key in ["KP1", "KP0", "KP*", "KP+", "KP-", "KP.", "KP/"] {
            assert!(snap.full_width_keypad(key), "{key}");
            assert!(!Snapshot::default().full_width_keypad(key), "半形 {key}");
        }
        for key in ["1", "KP", "Left", "@"] { assert!(!snap.full_width_keypad(key), "{key}"); }
    }

    #[test]
    fn full_width_english_preflight_consumes_printable_keys_without_completion() {
        let snap = parse(&json!({"snapshot":{"mode":"english","full_width":true}}));
        for key in ["a", "A", "0", "9", " ", "'", "\\"] { assert!(snap.english_wants_key(key), "{key}"); }
        // 數字鍵盤（#278）：全形時打全形，預檢就說吃，OnKeyDown 才送一次。
        for key in ["KP1", "KP0", "KP*", "KP+", "KP."] { assert!(snap.english_wants_key(key), "{key}"); }
        for key in ["Tab", "Escape", "Left", "KP"] { assert!(!snap.english_wants_key(key), "{key}"); }
        assert!(!parse(&json!({"snapshot":{"mode":"english"}})).english_wants_key("a"));
        assert!(!parse(&json!({"snapshot":{"mode":"english"}})).english_wants_key("KP1"), "半形數字鍵盤照舊交給宿主");
    }

    #[test]
    fn english_key_preflight_matches_buffer_boundaries() {
        let mut s = parse(
            &json!({"snapshot": {"mode": "english", "english_autocomplete": true,
            "english_pending": true, "preedit": [{"text":"hel"}],
            "candidates": {"items":[{"id":0,"text":"hello"}]}}}),
        );
        assert!(s.english_pending && s.english_suggesting());
        // #328：補字候選上的反悔鍵在 TestKeyDown 就吃下，不先送出單字。
        for k in ["h", "H", "Backspace", "Escape", "Enter", "Tab", "Up", "Down", "ShiftDelete", "ShiftBackspace"] {
            assert!(s.english_wants_key(k), "{}", k);
        }
        for k in [
            " ", ".", "1", "Left", "Right", "Delete", "Home", "End", "PageUp", "PageDown", "ShiftLeft", "ShiftRight",
        ] {
            assert!(!s.english_wants_key(k), "must pass host action: {}", k);
        }
        s.candidates.clear();
        assert!(!s.english_suggesting());
        for k in ["Tab", "Up", "Down", "ShiftDelete", "ShiftBackspace"] {
            assert!(!s.english_wants_key(k));
        }
        s.preedit[0].text = "a".repeat(48);
        assert!(
            !s.english_wants_key("a"),
            "word limit must pass the next letter"
        );
        let legacy = parse(&json!({"snapshot":{"mode":"english"}}));
        assert!(!legacy.english_pending && !legacy.english_wants_key("a"));
    }

    // #160：重開 session 時只在記得的模式跟新 session 開始時的模式不同才還原；還沒問過模式就照新 session 的。
    #[test]
    fn restore_mode_only_when_it_differs_from_the_start() {
        assert_eq!(restore_mode(true, true, false), Some(true), "舊引擎（開始時注音）：記得英文才還原，跟以前一樣");
        assert_eq!(restore_mode(true, false, false), None);
        assert_eq!(restore_mode(true, false, true), Some(false), "開始時英文：記得注音要還原，不然引擎重啟就變英文");
        assert_eq!(restore_mode(true, true, true), None);
        assert_eq!(restore_mode(false, false, true), None, "新開的程式還沒問過：照新 session 開始時的英文");
        assert_eq!(initial_english(&json!({"initial_mode": "english"})), Some(true));
        assert_eq!(initial_english(&json!({"initial_mode": "zhuyin"})), Some(false));
        assert_eq!(initial_english(&json!({"version": "old"})), None, "舊引擎沒有回報");
    }

    #[test]
    fn candidate_notes_line_up_with_candidates() {
        let reply: Value = serde_json::from_str(
            r#"{"snapshot":{"handled":true,"candidates":{"page":0,"pages":1,"highlight":0,
                "items":[{"id":0,"text":"吃"},{"id":1,"text":"痴","note":"近音 ㄔ"}]}}}"#,
        )
        .unwrap();
        let s = parse(&reply);
        assert_eq!(s.candidates.len(), 2);
        assert_eq!(
            s.candidate_notes,
            vec!["".to_string(), "近音 ㄔ".to_string()]
        );
    }

    // #112：鍵標照核心給的選字鍵；舊核心依位置 1–0（第 10 項是 0），英文模式的補字候選不畫。
    #[test]
    fn candidate_keys_follow_the_engine() {
        let s = parse(&json!({"snapshot": {"mode": "zhuyin", "candidates": {"items": [
            {"id": 0, "text": "是", "key": "1"}, {"id": 1, "text": "式", "key": "q"}, {"id": 2, "text": "示", "key": ";"}]}}}));
        assert_eq!(s.candidate_keys, vec!["1", "q", ";"]);
        let items: Vec<Value> = (0..10).map(|i| json!({"id": i, "text": "字"})).collect();
        let old = parse(&json!({"snapshot": {"mode": "zhuyin", "candidates": {"items": items}}}));
        assert_eq!(old.candidate_keys, vec!["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"]);
        let english = parse(&json!({"snapshot": {"mode": "english", "candidates": {"items": [{"id": 0, "text": "hello"}]}}}));
        assert_eq!(english.candidate_keys, vec![""]);
    }

    #[test]
    fn notices_and_marked_style_are_parsed() {
        // #123：加詞的提示與標示樣式。
        let s = parse(&json!({"snapshot": {"handled": true,
            "preedit": [{"text": "我和", "style": "normal"}, {"text": "世傑", "style": "marked"}],
            "notices": ["已加入詞庫：世傑", "", 3]}}));
        assert_eq!(s.notices, vec!["已加入詞庫：世傑".to_string()]);
        assert_eq!(s.notice(), "已加入詞庫：世傑");
        assert_eq!(s.preedit[1].style, "marked");
        assert!(s.marking(), "組字區有 marked 分段就是標示中");
        let two = parse(&json!({"snapshot": {"notices": ["甲", "乙"]}}));
        assert_eq!(two.notice(), "甲；乙");
        let old = parse(&json!({"snapshot": {"handled": true}}));
        assert!(old.notices.is_empty() && old.notice().is_empty(), "舊引擎沒有 notices");
        assert!(!two.marking() && !old.marking(), "沒有 marked 分段不是標示中");
    }

    #[test]
    fn reading_hint_is_parsed() {
        // #162：中英混打打 20 時的讀音提示；舊引擎沒有這個欄位。
        let s = parse(&json!({"snapshot": {"handled": true,
            "preedit": [{"text": "20", "style": "reading"}],
            "reading_hint": {"start": 0, "length": 2, "text": "ㄉㄢ"}}}));
        assert_eq!(s.reading_hint, "ㄉㄢ");
        assert_eq!(s.preedit[0].style, "reading");
        let old = parse(&json!({"snapshot": {"handled": true}}));
        assert!(old.reading_hint.is_empty());
    }

    #[test]
    fn vocab_key_names_fall_back_for_unsupported_engines() {
        for (name, base) in [
            ("ShiftLeft", "Left"),
            ("ShiftRight", "Right"),
            ("ShiftDelete", "Delete"),
            ("ShiftBackspace", "Backspace"),
        ] {
            let mut r = json!({"type": "key", "key": name, "private": true});
            downgrade_vocab_key(&mut r);
            assert_eq!(r, json!({"type": "key", "key": base, "private": true}));
        }
        for mut r in [
            json!({"type": "key", "key": "Left"}),
            json!({"type": "key", "key": "a"}),
            json!({"type": "select", "key": "ShiftLeft", "index": 1}),
        ] {
            let before = r.clone();
            downgrade_vocab_key(&mut r);
            assert_eq!(r, before);
        }
    }
}

/// 沒在組字時這個鍵要不要交給引擎，照 health 回報的 idle_keys（core/session.IdleKeys，測試逐鍵比對實際處理結果）；
/// 全形（`full_width`，Shift＋空白，#278）時照 idle_keys_full_width（core/session.FullWidthIdleKeys），@ # * + 等也打全形。
/// 從沒問到過引擎（`info` 是 Null）時照 keys::starts_composition 的固定集合。
fn idle_key_from(info: &Value, key: &str, full_width: bool) -> bool {
    let reported = if full_width { &info["idle_keys_full_width"] } else { &info["idle_keys"] };
    match reported.as_str() {
        Some(keys) => key.chars().count() == 1 && keys.contains(key),
        None => crate::keys::starts_composition(key),
    }
}

#[cfg(test)]
mod idle_key_tests {
    use super::*;

    #[test]
    fn full_width_idle_keys_follow_the_engine_report() {
        // #278：客戶的設定（標準排列、Shift＋空白切全形）。半形時 @ # * + 交還宿主，全形時由引擎打成全形。
        let info = json!({
            "idle_keys": "!\"&'(),-./0123456789:;<=>?[\\]^_`abcdefghijklmnopqrstuvwxyz{}~",
            "idle_keys_full_width": "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~",
        });
        for key in ["@", "#", "$", "%", "*", "+", "|", "A"] {
            assert!(!idle_key_from(&info, key, false), "半形 {key}");
            assert!(idle_key_from(&info, key, true), "全形 {key}");
        }
        for key in ["j", "3", ",", "["] {
            assert!(idle_key_from(&info, key, false) && idle_key_from(&info, key, true), "{key}");
        }
        assert!(!idle_key_from(&info, "KP1", true), "數字鍵盤不在清單，由 wants_key 看字寬");
        // 引擎的全形清單沒列的鍵（例如以後的排列把某個可見字元當一聲）全形時照樣交還宿主，不能吃了又不處理。
        let partial = json!({"idle_keys": "abc", "idle_keys_full_width": "abc@"});
        assert!(idle_key_from(&partial, "@", true));
        assert!(!idle_key_from(&partial, "#", true));
        // 從沒問到過引擎：可見 ASCII 都交給引擎，數字鍵盤照舊由 wants_key 看字寬。
        for key in ["a", ",", "@", "3"] {
            assert!(idle_key_from(&Value::Null, key, false) && idle_key_from(&Value::Null, key, true), "{key}");
        }
        assert!(!idle_key_from(&Value::Null, "KP1", false));
    }
}

#[cfg(test)]
mod tests {
    //! 對真的 0c-server 做整合測試：先在另一個管道名稱啟動 server，
    //! 再以 ZUIN_TEST_PIPE 指定；沒有設定就略過（不讀日常個人詞庫）。
    use super::*;

    fn session() -> Option<Session> {
        let pipe = std::env::var("ZUIN_TEST_PIPE").ok()?;
        Some(Session::with_client(Client::with_pipe(&pipe)))
    }

    #[test]
    fn english_autocomplete_preserves_host_boundaries() {
        let Ok(pipe) = std::env::var("ZUIN_TEST_ENGLISH_PIPE") else {
            return;
        };
        let mut s = Session::with_client(Client::with_pipe(&pipe));
        let start = s.handle(json!({"type":"mode", "on":true}));
        assert!(start.ok && start.english_can_start);
        for boundary in [" ", "Left", "Delete", "."] {
            for k in ["h", "e", "l"] {
                assert!(s.key(k).handled);
            }
            assert!(s.english_state.english_pending);
            assert!(
                !s.english_state.english_wants_key(boundary),
                "TestKeyDown must decline {}",
                boundary
            );
            let next = s.key(boundary);
            assert!(next.ok && !next.handled && !next.english_pending);
            assert_eq!(next.commit, "hel");
            s.key(" ");
        }
        for k in ["h", "e", "l"] {
            s.key(k);
        }
        let accepted = s.key("Tab");
        assert!(accepted.handled && accepted.text() != "hel");
        let enter = s.key("Enter");
        assert_eq!(enter.commit, accepted.text());
        // #235：Enter 只送出單字，不交給宿主；沒有組字時的 Enter 才交給宿主。
        assert!(enter.handled && !enter.english_pending);
        assert!(!s.english_state.english_wants_key("Enter"));
        assert!(!s.key("Enter").handled);
    }

    #[test]
    fn types_and_commits() {
        let Some(mut s) = session() else {
            eprintln!("略過：沒有 ZUIN_TEST_PIPE");
            return;
        };
        let mut last = Snapshot::default();
        for k in ["s", "u", "3", "c", "l", "3"] {
            last = s.key(k);
            assert!(last.ok, "server 不可用");
            assert!(last.handled);
        }
        assert_eq!(last.text(), "你好");
        assert_eq!(last.cursor_utf16, 2);
        assert!(last.composing());
        let committed = s.key("Enter");
        assert_eq!(committed.commit, "你好");
        assert!(!committed.composing());
        assert!(s.log.is_empty(), "提交後重播紀錄要清空");
    }

    // P4.5：= 直接打 =，跟其他明確的符號一樣先留在組字區（Emoji 到 P11 才移進符號表）。
    #[test]
    fn equals_types_equals() {
        let Some(mut s) = session() else { return };
        assert!(s.idle_key("="));
        assert_eq!(s.id, 0, "查設定不能建立或改動組字");
        let typed = s.key("=");
        assert!(typed.ok && typed.handled && typed.candidates.is_empty());
        assert_eq!(typed.text(), "=");
        assert_eq!(s.key("Enter").commit, "=");
    }

    #[test]
    fn commit_offers_document_context_once() {
        let Some(mut s) = session() else { return };
        for k in ["s", "u", "3", "c", "l", "3"] {
            s.key(k);
        }
        let committed = s.key("Enter");
        assert_eq!(committed.commit, "你好");
        let (id, text) = s.take_pending_context().expect("提交後應提供待確認前文");
        assert!(id != 0);
        assert_eq!(text, "你好");
        assert!(s.take_pending_context().is_none(), "每次提交只提供一次");
        s.confirm_context(id, true);
        let next = s.key("s");
        assert!(next.ok && next.handled, "確認前文後仍可繼續輸入");
        s.reset();
        assert!(s.take_pending_context().is_none());
    }

    #[test]
    fn private_field_composes_without_offering_context() {
        // 無痕／私密視窗（IS_PRIVATE）照常組字，請求帶 private，提交後不提供前文。
        let Some(mut s) = session() else { return };
        s.set_private(true);
        for k in ["s", "u", "3", "c", "l", "3"] {
            assert!(s.key(k).handled);
        }
        assert!(s.log.iter().all(|r| r["private"] == true), "重播紀錄也要是私人組字");
        assert_eq!(s.key("Enter").commit, "你好");
        assert!(s.take_pending_context().is_none(), "私人欄位不讀前文");
        s.set_private(false);
        for k in ["s", "u", "3"] {
            s.key(k);
        }
        assert!(s.log.iter().all(|r| r.get("private").is_none()));
        s.reset();
    }

    // #160：引擎設成開始時英文（ZUIN_TEST_START_ENGLISH_PIPE 的 settings.json 有 start_mode=english、mode_memory=native）。
    // 新開的程式第一個按鍵前就知道是英文；切到注音後引擎重啟（斷線重開）仍是注音；換欄位（reset）維持這個程式的模式。
    #[test]
    fn start_english_is_known_before_the_first_key() {
        let Ok(pipe) = std::env::var("ZUIN_TEST_START_ENGLISH_PIPE") else {
            return;
        };
        let mut s = Session::with_client(Client::with_pipe(&pipe));
        assert!(!s.english());
        assert!(s.learn_mode(false) && s.english(), "Activate 時就知道開始時是英文");
        assert!(!s.client.connected(), "Activate 的查詢不留連線");
        assert!(!s.learn_mode(true), "問過就不再問");
        let snap = s.key("t");
        assert!(snap.ok && snap.english && !snap.handled, "第一個按鍵是英文，交還應用程式");
        let snap = s.toggle();
        assert!(snap.ok && !snap.english);
        s.client.disconnect_for_test();
        let snap = s.key("j");
        assert!(!snap.english && snap.handled, "重開 session 後仍是這個程式切過的注音");
        s.key("Escape");
        s.reset();
        assert!(!s.learn_mode(false) && !s.english(), "各程式各自記住：換欄位不重新問");
        let snap = s.key("j");
        assert!(!snap.english, "換欄位後仍是注音");
        s.key("Escape");
        let mut other = Session::with_client(Client::with_pipe(&pipe));
        assert!(other.learn_mode(true) && other.english(), "另一個新開的程式從英文開始");
    }

    // #278：Shift＋空白切全形後，TestKeyDown 的預檢（idle_key、full_width_keypad）要跟引擎實際的處理一致：
    // 說要吃卻回未處理，WPF 會掉鍵；說不吃，@ # * 就交給應用程式打成半形。ZUIN_TEST_FULL_WIDTH_PIPE 是
    // settings.json 有 style-shift-space=fullwidth 的獨立引擎（例如「我的原有設定」改 Shift＋空白切換全半形）。
    #[test]
    fn full_width_preflight_matches_the_engine() {
        let Ok(pipe) = std::env::var("ZUIN_TEST_FULL_WIDTH_PIPE") else {
            return;
        };
        let mut s = Session::with_client(Client::with_pipe(&pipe));
        for full_width in [false, true] {
            for c in 0x21u8..=0x7e {
                let key = (c as char).to_string();
                if full_width {
                    let snap = s.key("ShiftSpace");
                    assert!(snap.ok && snap.handled && snap.full_width, "{snap:?}");
                }
                let want = s.idle_key(&key);
                let snap = s.key(&key);
                assert!(snap.ok);
                assert_eq!(want, snap.handled, "全形={full_width} {key:?}：預檢 {want}，引擎 {snap:?}");
                if full_width && "@#$%*+=|".contains(&key) {
                    assert_eq!(snap.commit, char::from_u32(c as u32 + 0xfee0).unwrap().to_string(), "{key}");
                }
                s.reset();
            }
            for key in ["KP1", "KP0", "KP*", "KP+", "KP-", "KP.", "KP/"] {
                if full_width {
                    s.key("ShiftSpace");
                }
                let want = s.english_state.full_width_keypad(key);
                let snap = s.key(key);
                assert_eq!(want, snap.handled, "全形={full_width} {key}：{snap:?}");
                s.reset();
            }
        }
    }

    #[test]
    fn mode_and_mixed_composition_survive_reconnect() {
        let Some(mut s) = session() else { return };
        s.key("s");
        s.key("u");
        s.key("3");
        s.handle(json!({"type":"toggle"}));
        s.key("a");
        s.client.disconnect_for_test();
        let snap = s.key("b");
        assert!(snap.english);
        assert_eq!(snap.text(), "你ab");
        s.key("Enter");
        s.client.disconnect_for_test();
        let snap = s.key("x");
        assert!(snap.english);
        assert!(!snap.handled);
        s.reset();
        assert!(s.log.is_empty());
        let snap = s.handle(json!({"type":"mode", "on":false}));
        assert!(snap.text().is_empty());
        assert!(!snap.english);
    }

    #[test]
    fn immediate_toggle_back_restores_pending_zhuyin() {
        let Some(mut s) = session() else { return };
        s.key("j");
        let english = s.toggle();
        assert!(english.english && !english.composing(), "切到英文清掉未打完的注音（#90）");
        let back = s.toggle();
        assert!(!back.english);
        assert_eq!(back.text(), "ㄨ", "立即切回放回注音");
        s.client.disconnect_for_test();
        let replayed = s.key("i");
        assert!(!replayed.english);
        assert_eq!(replayed.text(), "ㄨㄛ", "斷線重播後仍是放回的注音");
        s.toggle();
        s.bypassed();
        let snap = s.toggle();
        assert!(!snap.english && !snap.composing(), "切換之間有交還宿主的按鍵就不放回");
        s.reset();
    }

    #[test]
    fn reset_discards_pending_composition_even_after_disconnect() {
        let Some(mut s) = session() else { return };
        for k in ["s", "u", "3"] {
            s.key(k);
        }
        s.client.disconnect_for_test();
        s.reset();
        let snap = s.key("c");
        assert_eq!(snap.text(), "ㄏ");
        assert!(snap.commit.is_empty());
    }

    #[test]
    fn passes_through_when_not_handled() {
        let Some(mut s) = session() else { return };
        let snap = s.key("Enter"); // 空的組字區：Enter 交還應用程式
        assert!(snap.ok);
        assert!(!snap.handled);
        assert!(!snap.composing());
    }

    #[test]
    fn replays_after_reconnect() {
        let Some(mut s) = session() else { return };
        for k in ["s", "u", "3"] {
            s.key(k);
        }
        assert_eq!(s.log.len(), 3);
        // 模擬斷線：丟掉連線後下一鍵應重開 session 並重播，組字不遺失
        s.client.disconnect_for_test();
        let snap = s.key("c");
        assert!(snap.ok);
        assert!(snap.reconnected);
        assert_eq!(snap.text(), "你ㄏ");
        let _ = s.handle(json!({"type": "cancel"}));
    }

    // Native named-pipe fixture: the closing writer explicitly refuses all
    // requests, then its replacement accepts one key. No desktop registration,
    // daily dictionary, or external server is needed for these regressions.
    struct HandoffServer {
        name: String,
        stop: std::sync::Arc<std::sync::atomic::AtomicBool>,
        worker: Option<std::thread::JoinHandle<usize>>,
    }

    impl HandoffServer {
        fn new(error: &'static str, reject_for: Option<std::time::Duration>) -> Self {
            use std::io::{Read, Write};
            use std::os::windows::io::FromRawHandle;
            use std::sync::atomic::Ordering;
            use windows::Win32::Foundation::{GetLastError, ERROR_PIPE_CONNECTED};
            use windows::Win32::Storage::FileSystem::PIPE_ACCESS_DUPLEX;
            use windows::Win32::System::Pipes::{
                ConnectNamedPipe, CreateNamedPipeW, PIPE_READMODE_BYTE, PIPE_TYPE_BYTE, PIPE_WAIT,
            };
            use windows_core::PCWSTR;
            let name = format!(
                r"\\.\pipe\0c-tsf-handoff-{}-{}",
                std::process::id(),
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_nanos()
            );
            let stop = std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false));
            let stopping = stop.clone();
            let pipe_name = zuin_win_common::wide(&name);
            let (ready, wait) = std::sync::mpsc::channel();
            let worker = std::thread::spawn(move || {
                let until = reject_for.map(|d| std::time::Instant::now() + d);
                let mut ready = Some(ready);
                let mut keys = 0;
                while !stopping.load(Ordering::SeqCst) {
                    let handle = unsafe {
                        CreateNamedPipeW(
                            PCWSTR::from_raw(pipe_name.as_ptr()),
                            PIPE_ACCESS_DUPLEX,
                            PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                            1,
                            65536,
                            65536,
                            0,
                            None,
                        )
                    };
                    assert!(!handle.is_invalid());
                    let mut file = unsafe { std::fs::File::from_raw_handle(handle.0) };
                    if let Some(r) = ready.take() {
                        r.send(()).unwrap();
                    }
                    let connected = unsafe { ConnectNamedPipe(handle, None) };
                    assert!(connected.is_ok() || unsafe { GetLastError() } == ERROR_PIPE_CONNECTED);
                    if stopping.load(Ordering::SeqCst) {
                        break;
                    }
                    loop {
                        let mut header = [0; 4];
                        if file.read_exact(&mut header).is_err() {
                            break;
                        }
                        let mut body = vec![0; u32::from_le_bytes(header) as usize];
                        file.read_exact(&mut body).unwrap();
                        let request: Value = serde_json::from_slice(&body).unwrap();
                        let refusing = until.is_none_or(|t| std::time::Instant::now() < t);
                        let reply = if refusing {
                            json!({"ok": false, "error": error})
                        } else if request["op"] == "open" {
                            json!({"ok": true, "session": 1})
                        } else if request["op"] == "handle" && request["req"]["type"] == "key" {
                            keys += 1;
                            json!({"ok": true, "snapshot": {"handled": true, "commit": "你好"}})
                        } else {
                            json!({"ok": true})
                        };
                        if file
                            .write_all(&zuin_win_common::frame(&reply).unwrap())
                            .is_err()
                        {
                            break;
                        }
                    }
                }
                keys
            });
            wait.recv_timeout(std::time::Duration::from_secs(2))
                .unwrap();
            Self {
                name,
                stop,
                worker: Some(worker),
            }
        }

        fn finish(mut self) -> usize {
            use std::sync::atomic::Ordering;
            self.stop.store(true, Ordering::SeqCst);
            let worker = self.worker.take().unwrap();
            // Wake a blocking accept after the test session has closed its pipe.
            while !worker.is_finished() {
                if let Ok(wake) = std::fs::OpenOptions::new()
                    .read(true)
                    .write(true)
                    .open(&self.name)
                {
                    drop(wake);
                    break;
                }
                std::thread::sleep(std::time::Duration::from_millis(2));
            }
            worker.join().unwrap()
        }
    }

    #[test]
    fn closing_writer_waits_for_replacement_and_commits_once() {
        let server =
            HandoffServer::new("引擎正在切換", Some(std::time::Duration::from_millis(200)));
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        let snap = s.key("Enter");
        assert!(snap.ok && snap.handled, "handoff must retain the key");
        assert_eq!(snap.commit, "你好");
        drop(s);
        assert_eq!(server.finish(), 1, "replacement must receive the key once");
    }

    // #183 code review：改設定讓引擎重啟時，交接中的第一個字母 TestKeyDown 問不到 idle_keys。不能交還宿主（中文模式直接打出
    // 英文字母）：沿用上一次的答案，從沒問到過就照舊推算；OnKeyDown 的 handle() 等交接結束，新引擎收到這個鍵一次。
    #[test]
    fn idle_letters_during_handoff_go_to_the_engine() {
        let server =
            HandoffServer::new("引擎正在切換", Some(std::time::Duration::from_millis(500)));
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        assert!(s.idle_key("a"), "從沒問到過：字母交給引擎（同 0.2.4）");
        assert!(s.client.restart_pending(), "引擎回了「引擎正在切換」");
        // 上一次在舊連線上問到的答案（許氏：數字交還宿主、字母交給引擎）照樣適用，不因交接把數字也吃掉。
        s.idle_info = Some(json!({"idle_keys": "!\"&'(),.:;<=>?[\\]^_`abcdefghijklmnoprstuvwxyz{}~"}));
        assert!(s.idle_key("a") && s.idle_key("j"));
        assert!(!s.idle_key("1") && !s.idle_key("q"));
        let snap = s.key("a");
        assert!(snap.ok && snap.handled, "OnKeyDown 等交接結束才送");
        drop(s);
        assert_eq!(server.finish(), 1, "新引擎收到這個鍵一次");
    }

    #[test]
    fn closing_writer_has_a_fixed_deadline() {
        let server = HandoffServer::new("引擎正在切換", None);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        let start = std::time::Instant::now();
        assert!(!s.key("Enter").ok);
        assert!(
            start.elapsed() < crate::ipc::RESTART_WAIT + std::time::Duration::from_millis(1500),
            "refusals must not extend the deadline"
        );
        drop(s);
        assert_eq!(server.finish(), 0);
    }

    /// #216 的假引擎：模擬改設定套用、更新交接或當掉後的重啟。第一個引擎回覆 `old_keys` 個按鍵後關掉連線與管道（退出），
    /// 管道消失 `gap` 之後新引擎才建立管道（None：永遠不回來）。`hung`：第一個引擎回覆 `old_keys` 個按鍵後不退出，
    /// 之後收下請求卻永遠不回覆（卡住，不是重啟）。
    /// 每個引擎收到的按鍵依序記下（引擎代號, 按鍵）。
    struct RestartServer {
        name: String,
        keys: std::sync::Arc<std::sync::Mutex<Vec<(u32, String)>>>,
        stop: std::sync::Arc<std::sync::atomic::AtomicBool>,
        gone: std::sync::mpsc::Receiver<()>,
    }

    impl RestartServer {
        fn new(old_keys: usize, gap: Option<Duration>, hung: bool) -> Self {
            use std::io::{Read, Write};
            use std::os::windows::io::FromRawHandle;
            use std::sync::atomic::{AtomicBool, Ordering};
            use std::sync::{Arc, Mutex};
            use windows::Win32::Storage::FileSystem::PIPE_ACCESS_DUPLEX;
            use windows::Win32::System::Pipes::{
                ConnectNamedPipe, CreateNamedPipeW, PIPE_READMODE_BYTE, PIPE_TYPE_BYTE, PIPE_WAIT,
            };
            use windows_core::PCWSTR;
            let name = format!(
                r"\\.\pipe\0c-tsf-restart-{}-{}",
                std::process::id(),
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_nanos()
            );
            let keys = Arc::new(Mutex::new(Vec::new()));
            let stop = Arc::new(AtomicBool::new(false));
            let (seen, stopping) = (keys.clone(), stop.clone());
            let pipe_name = zuin_win_common::wide(&name);
            let (ready, wait) = std::sync::mpsc::channel();
            let (gone_tx, gone) = std::sync::mpsc::channel();
            std::thread::spawn(move || {
                // 一個引擎：一次一條連線，連線斷了再開下一個實例；回覆滿 limit 個按鍵就退出（關掉連線與管道）。
                let serve = |engine: u32, limit: Option<usize>, ready: Option<std::sync::mpsc::Sender<()>>| {
                    let mut ready = ready;
                    let mut served = 0;
                    while !stopping.load(Ordering::SeqCst) {
                        let handle = unsafe {
                            CreateNamedPipeW(
                                PCWSTR::from_raw(pipe_name.as_ptr()),
                                PIPE_ACCESS_DUPLEX,
                                PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                                1,
                                65536,
                                65536,
                                0,
                                None,
                            )
                        };
                        assert!(!handle.is_invalid());
                        let mut file = unsafe { std::fs::File::from_raw_handle(handle.0) };
                        if let Some(r) = ready.take() {
                            r.send(()).unwrap();
                        }
                        let _ = unsafe { ConnectNamedPipe(handle, None) };
                        if stopping.load(Ordering::SeqCst) {
                            return;
                        }
                        loop {
                            let mut header = [0; 4];
                            if file.read_exact(&mut header).is_err() {
                                break;
                            }
                            let mut body = vec![0; u32::from_le_bytes(header) as usize];
                            if file.read_exact(&mut body).is_err() {
                                break;
                            }
                            let request: Value = serde_json::from_slice(&body).unwrap();
                            if hung && limit.is_some_and(|n| served >= n) {
                                continue; // 卡住：收下請求、永遠不回覆
                            }
                            let reply = if request["op"] == "open" {
                                json!({"ok": true, "session": engine, "info": {}})
                            } else if request["op"] == "handle" && request["req"]["type"] == "key" {
                                let key = request["req"]["key"].as_str().unwrap_or("").to_string();
                                seen.lock().unwrap().push((engine, key.clone()));
                                served += 1;
                                json!({"ok": true, "snapshot": {"handled": true, "commit": key}})
                            } else {
                                json!({"ok": true, "info": {}})
                            };
                            if file.write_all(&zuin_win_common::frame(&reply).unwrap()).is_err() {
                                break;
                            }
                            if !hung && limit.is_some_and(|n| served >= n) {
                                return; // 退出：drop(file) 關掉連線，管道跟著消失
                            }
                        }
                    }
                };
                serve(1, Some(old_keys), Some(ready));
                if hung {
                    return;
                }
                let _ = gone_tx.send(());
                let Some(gap) = gap else { return };
                std::thread::sleep(gap);
                serve(2, None, None);
            });
            wait.recv_timeout(Duration::from_secs(2)).unwrap();
            Self { name, keys, stop, gone }
        }

        /// 等第一個引擎退出（回覆完最後一個按鍵、關掉管道）。
        fn wait_gone(&self) {
            self.gone.recv_timeout(Duration::from_secs(2)).unwrap();
        }

        fn keys(&self) -> Vec<(u32, String)> {
            self.keys.lock().unwrap().clone()
        }
    }

    impl Drop for RestartServer {
        fn drop(&mut self) {
            self.stop.store(true, std::sync::atomic::Ordering::SeqCst);
            // 叫醒等著連線的實例。
            let _ = std::fs::OpenOptions::new().read(true).write(true).open(&self.name);
        }
    }

    fn keyed(list: &[(u32, &str)]) -> Vec<(u32, String)> {
        list.iter().map(|(e, k)| (*e, k.to_string())).collect()
    }

    // #216：改設定套用後引擎重啟（舊引擎關掉連線退出、新引擎約 1 秒後才開始監聽），這時打的字不能不見。
    // 修正前 handle() 試 3 次就放行（距上次啟動不到 10 秒時連等都不等），TestKeyDown 已吃下的字母就消失了。
    #[test]
    fn keys_typed_while_the_engine_restarts_reach_the_new_engine_in_order() {
        let gap = Duration::from_millis(800);
        let server = RestartServer::new(1, Some(gap), false);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        assert!(s.key("a").handled);
        server.wait_gone();
        // TestKeyDown 的查詢不等新引擎（不卡宿主的這一次呼叫），字母照舊交給引擎。
        let start = Instant::now();
        assert!(s.idle_key("g"), "交接中的字母交給引擎");
        assert!(start.elapsed() < Duration::from_millis(300), "TestKeyDown 不等：{:?}", start.elapsed());
        assert!(s.client.restart_pending(), "舊引擎關掉連線：等新引擎");
        // OnKeyDown：等到新引擎起來才送；之後的按鍵（宿主的訊息佇列裡依序排著）照順序送到新引擎。
        let first = s.key("g");
        assert!(first.ok && first.handled, "第一個字母要等到新引擎");
        assert!(start.elapsed() >= gap - Duration::from_millis(100), "{:?}", start.elapsed());
        assert!(first.reconnected);
        for k in ["4", "s", "u", "3"] {
            assert!(s.key(k).handled, "{k}");
        }
        assert!(!s.client.restart_pending(), "接上新引擎就不再等");
        assert_eq!(
            server.keys(),
            keyed(&[(1, "a"), (2, "g"), (2, "4"), (2, "s"), (2, "u"), (2, "3")]),
            "依序、沒有遺失、沒有重複"
        );
    }

    // #216：更新交接剛好在兩個詞之間、使用者還在連續打字：舊引擎退出前的鍵在舊引擎，之後的都到新引擎。
    #[test]
    fn typing_through_a_handover_between_words_loses_nothing() {
        let server = RestartServer::new(3, Some(Duration::from_millis(400)), false);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        for k in ["s", "u", "3", "c", "l", "3"] {
            assert!(s.key(k).ok, "{k}");
        }
        assert_eq!(
            server.keys(),
            keyed(&[(1, "s"), (1, "u"), (1, "3"), (2, "c"), (2, "l"), (2, "3")])
        );
    }

    // #216：組字中（還沒送出）引擎重啟：新引擎起來後重播組字一次，接著的鍵照順序到。
    #[test]
    fn composition_survives_the_restart_and_is_replayed_once() {
        let server = RestartServer::new(1, Some(Duration::from_millis(500)), false);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        // 假引擎回 commit；這裡把 "a" 記成組字中的請求，模擬打到一半。
        assert!(s.key("a").handled);
        s.log.push(json!({"type": "key", "key": "a"}));
        server.wait_gone();
        assert!(s.key("b").handled);
        assert_eq!(server.keys(), keyed(&[(1, "a"), (2, "a"), (2, "b")]), "新引擎先重播一次組字，再收到這一鍵");
    }

    // #216：等到期限新引擎仍沒起來（例如啟動失敗）：這一鍵放行（外殼改以原字元送進文件），之後的鍵立刻放行，不每鍵都等。
    #[test]
    fn engine_that_never_returns_releases_within_the_deadline() {
        let server = RestartServer::new(1, None, false);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        assert!(s.key("a").handled);
        server.wait_gone();
        let start = Instant::now();
        assert!(!s.key("g").ok);
        let waited = start.elapsed();
        assert!(
            waited >= crate::ipc::RESTART_WAIT - Duration::from_millis(200)
                && waited < crate::ipc::RESTART_WAIT + Duration::from_millis(1500),
            "等到期限為止：{waited:?}"
        );
        let start = Instant::now();
        assert!(!s.key("4").ok);
        assert!(!s.available());
        assert!(start.elapsed() < Duration::from_millis(500), "之後不再等：{:?}", start.elapsed());
        assert_eq!(server.keys(), keyed(&[(1, "a")]));
    }

    // #216：引擎卡住（連線還在、只是不回覆）不是重啟：照舊每次 I/O 逾時、試 3 次就放行，不另外等到重啟期限。
    #[test]
    fn a_hung_engine_is_not_a_restart() {
        let server = RestartServer::new(1, None, true);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        assert!(s.key("a").handled);
        let start = Instant::now();
        assert!(!s.key("g").ok);
        // 記成重啟的話會等到 RESTART_WAIT（5 秒）才放行。
        assert!(start.elapsed() < Duration::from_millis(4500), "逾時不記成重啟：{:?}", start.elapsed());
    }

    /// 依 open 回覆的 info 決定 vocab_keys 的管道伺服器；記下收到的按鍵。
    fn vocab_server(vocab_keys: Option<&'static str>) -> (String, std::sync::Arc<std::sync::Mutex<Vec<String>>>) {
        use std::io::{Read, Write};
        use std::os::windows::io::FromRawHandle;
        use windows::Win32::Storage::FileSystem::PIPE_ACCESS_DUPLEX;
        use windows::Win32::System::Pipes::{
            ConnectNamedPipe, CreateNamedPipeW, PIPE_READMODE_BYTE, PIPE_TYPE_BYTE, PIPE_WAIT,
        };
        use windows_core::PCWSTR;
        let name = format!(
            r"\\.\pipe\0c-tsf-vocab-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        );
        let keys = std::sync::Arc::new(std::sync::Mutex::new(Vec::new()));
        let seen = keys.clone();
        let pipe_name = zuin_win_common::wide(&name);
        let (ready, wait) = std::sync::mpsc::channel();
        std::thread::spawn(move || {
            let handle = unsafe {
                CreateNamedPipeW(
                    PCWSTR::from_raw(pipe_name.as_ptr()),
                    PIPE_ACCESS_DUPLEX,
                    PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                    1,
                    65536,
                    65536,
                    0,
                    None,
                )
            };
            let mut file = unsafe { std::fs::File::from_raw_handle(handle.0) };
            ready.send(()).unwrap();
            let _ = unsafe { ConnectNamedPipe(handle, None) };
            loop {
                let mut header = [0; 4];
                if file.read_exact(&mut header).is_err() {
                    break;
                }
                let mut body = vec![0; u32::from_le_bytes(header) as usize];
                file.read_exact(&mut body).unwrap();
                let request: Value = serde_json::from_slice(&body).unwrap();
                let reply = if request["op"] == "open" {
                    let mut info = json!({"data_sha256": "x"});
                    if let Some(v) = vocab_keys {
                        info["vocab_keys"] = Value::from(v);
                    }
                    json!({"ok": true, "session": 7, "info": info})
                } else if request["op"] == "handle" && request["req"]["type"] == "key" {
                    seen.lock().unwrap().push(request["req"]["key"].as_str().unwrap_or("").to_string());
                    json!({"ok": true, "snapshot": {"handled": true, "preedit": [{"text": "我", "style": "normal"}],
                        "notices": ["已加入詞庫：世傑"]}})
                } else {
                    json!({"ok": true})
                };
                if file.write_all(&zuin_win_common::frame(&reply).unwrap()).is_err() {
                    break;
                }
            }
        });
        wait.recv_timeout(std::time::Duration::from_secs(2)).unwrap();
        (name, keys)
    }

    #[test]
    fn vocab_keys_follow_the_open_reply() {
        // 支援的引擎：open 之後才知道，新鍵名原樣送出；提示一併解析。
        let (name, keys) = vocab_server(Some("true"));
        let mut s = Session::with_client(Client::with_pipe(&name));
        assert!(!s.vocab_keys(), "還沒開 session 不能送新鍵名");
        let first = s.key("j");
        assert!(first.ok && s.vocab_keys());
        assert_eq!(first.notices, vec!["已加入詞庫：世傑".to_string()]);
        s.key("ShiftLeft");
        assert_eq!(*keys.lock().unwrap(), vec!["j", "ShiftLeft"]);
        // 開關關閉（false）與舊引擎（沒有這一項）：不送新鍵名，萬一送到也換回原本的鍵。
        for v in [Some("false"), None] {
            let (name, keys) = vocab_server(v);
            let mut s = Session::with_client(Client::with_pipe(&name));
            s.key("j");
            assert!(!s.vocab_keys(), "{v:?}");
            s.key("ShiftLeft");
            s.key("ShiftBackspace");
            assert_eq!(*keys.lock().unwrap(), vec!["j", "Left", "Backspace"], "{v:?}");
            assert!(s.log.iter().all(|r| r["key"] != "ShiftLeft"), "重播紀錄也是原本的鍵");
        }
    }

    #[test]
    fn unrelated_rejections_do_not_wait_for_handoff() {
        let server = HandoffServer::new("協定版本不符", None);
        let mut s = Session::with_client(Client::with_pipe(&server.name));
        let start = std::time::Instant::now();
        assert!(!s.key("Enter").ok);
        assert!(start.elapsed() < std::time::Duration::from_millis(500));
        drop(s);
        assert_eq!(server.finish(), 0);
    }

    /// 套用設定時會重啟的假引擎：health／open 回報 Shift 與 Shift＋空白的設定（`on` 時開著）。`test_restart` 模擬
    /// 引擎存檔退出、以新設定再起來：回覆讀完後關掉這條連線、設定換成相反的值，下一條連線是「新的引擎」。
    /// 永遠有一個管道在等連線。
    struct RestartingServer {
        name: String,
        restarted: std::sync::mpsc::Receiver<()>,
    }

    impl RestartingServer {
        fn new(on: bool) -> Self {
            use std::io::{Read, Write};
            use std::os::windows::io::FromRawHandle;
            use std::sync::atomic::{AtomicBool, Ordering};
            use std::sync::Arc;
            use windows::Win32::Storage::FileSystem::PIPE_ACCESS_DUPLEX;
            use windows::Win32::System::Pipes::{
                ConnectNamedPipe, CreateNamedPipeW, PIPE_READMODE_BYTE, PIPE_TYPE_BYTE, PIPE_UNLIMITED_INSTANCES,
                PIPE_WAIT,
            };
            use windows_core::PCWSTR;
            let name = format!(
                r"\\.\pipe\0c-tsf-restart-{}-{}",
                std::process::id(),
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_nanos()
            );
            let on = Arc::new(AtomicBool::new(on));
            let (restart_tx, restarted) = std::sync::mpsc::channel();
            let (ready, wait) = std::sync::mpsc::channel();
            let pipe_name = zuin_win_common::wide(&name);
            std::thread::spawn(move || {
                let create = || unsafe {
                    CreateNamedPipeW(
                        PCWSTR::from_raw(pipe_name.as_ptr()),
                        PIPE_ACCESS_DUPLEX,
                        PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT,
                        PIPE_UNLIMITED_INSTANCES,
                        65536,
                        65536,
                        0,
                        None,
                    )
                };
                let mut next = create();
                ready.send(()).unwrap();
                loop {
                    let handle = next;
                    let _ = unsafe { ConnectNamedPipe(handle, None) };
                    next = create(); // 先開好下一個，客戶端重連時一定有管道
                    let (on, restart_tx) = (on.clone(), restart_tx.clone());
                    let mut file = unsafe { std::fs::File::from_raw_handle(handle.0) };
                    std::thread::spawn(move || {
                        loop {
                            let mut header = [0; 4];
                            if file.read_exact(&mut header).is_err() {
                                return;
                            }
                            let mut body = vec![0; u32::from_le_bytes(header) as usize];
                            file.read_exact(&mut body).unwrap();
                            let request: Value = serde_json::from_slice(&body).unwrap();
                            let enabled = on.load(Ordering::SeqCst);
                            let info = json!({
                                "left_shift": enabled.to_string(),
                                "right_shift": enabled.to_string(),
                                "shift_space": if enabled { "fullwidth" } else { "disabled" },
                                "start_mode": "zhuyin",
                                "initial_mode": "zhuyin",
                                "mode_memory": "native",
                            });
                            let reply = match request["op"].as_str() {
                                Some("health") => json!({"ok": true, "info": info}),
                                Some("open") => json!({"ok": true, "session": 1, "info": info}),
                                _ => json!({"ok": true}),
                            };
                            if file.write_all(&zuin_win_common::frame(&reply).unwrap()).is_err() {
                                return;
                            }
                            if request["op"] == "test_restart" {
                                let _ = file.sync_all(); // 等客戶端讀完回覆
                                drop(file);
                                on.store(!on.load(Ordering::SeqCst), Ordering::SeqCst);
                                restart_tx.send(()).unwrap();
                                return;
                            }
                        }
                    });
                }
            });
            wait.recv_timeout(std::time::Duration::from_secs(2)).unwrap();
            RestartingServer { name, restarted }
        }

        fn restart(&self, s: &mut Session) {
            let restart = json!({"op": "test_restart"});
            // 上一次重啟後這條連線已經斷了：第一次呼叫才發現，重連再送。
            assert!(s.client.call(&restart).or_else(|| s.client.call(&restart)).is_some());
            self.restarted.recv_timeout(std::time::Duration::from_secs(2)).unwrap();
        }
    }

    #[test]
    fn first_style_modifier_after_handoff_uses_replacement_capabilities() {
        for first in ["shift", "space"] {
            let server = RestartingServer::new(false);
            let mut s = Session::with_client(Client::with_pipe(&server.name));
            assert!(!s.style_shift_space());
            assert!(!s.shift_config().toggles(crate::keys::Side::Left));
            for enabled in [true, false] {
                server.restart(&mut s);
                let actual = match first {
                    "shift" => s.shift_config().toggles(crate::keys::Side::Left),
                    _ => s.style_shift_space(),
                };
                assert_eq!(actual, enabled, "first {first} after handoff");
            }
        }
    }
}
