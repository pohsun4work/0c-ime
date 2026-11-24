//! 呼叫 0c-update 的 JSON 契約（docs/settings-contract.md）。
//!
//! 設定邏輯與更新驗證全在 Go；這裡只負責用絕對路徑啟動它、處理逾時，並把 stdout 的 JSON 原樣交給前端。
//! 前端可以送來的只有固定的幾個動作與經過檢查的鍵值，Go 端另外再驗證一次。

use serde_json::Value;
use std::io::Read;
use std::os::windows::process::CommandExt;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};

/// 可以指定開啟的頁面；與 internal/settingsapp 的白名單一致（vocab 是 0.2.0 的「我的詞彙」，
/// features.vocab 不為 true（舊版 0c-update）時畫面改開總覽；models 是 #180 的「模型」）。
/// mode、candidates、keyboard 是 0.2.5 拆開的「中英切換」「選字」「鍵盤與符號」（#207）；舊的 input 照樣收下，畫面導到「中英切換」。
pub const PAGES: [&str; 10] = [
    "overview", "input", "update", "troubleshoot", "about", "vocab", "models", "mode", "candidates", "keyboard",
];

/// 取出命令列的 `--page X`；不認得的頁面開總覽。
pub fn page_from_args(args: &[String]) -> &'static str {
    let wanted = args.windows(2).find(|w| w[0] == "--page").map(|w| w[1].as_str()).unwrap_or("");
    PAGES.iter().copied().find(|p| *p == wanted).unwrap_or(PAGES[0])
}

const UPDATER: &str = "0c-update.exe";

/// 不閃出主控台視窗（CreateProcess 的 CREATE_NO_WINDOW）。
pub const CREATE_NO_WINDOW: u32 = 0x0800_0000;

fn usable(path: &Path) -> bool {
    path.is_absolute() && path.is_file()
}

/// 找出要呼叫的 0c-update。優先用啟動這個程式的那一個（環境變數 ZC_UPDATER）：設定程式與它讀同一份契約，
/// 版本必定一致；其次是同一個資料夾裡的（核心版本目錄與完整安裝的版本目錄都把 0c-update.exe 放在設定程式旁邊）；
/// 再來是 %LOCALAPPDATA%\Zuin\bin 的穩定副本。
pub fn find_updater(env_updater: Option<&str>, current_exe: Option<&Path>, local_app_data: Option<&Path>) -> Result<PathBuf, String> {
    let mut candidates: Vec<PathBuf> = Vec::new();
    if let Some(p) = env_updater.filter(|p| !p.is_empty()) {
        candidates.push(PathBuf::from(p));
    }
    if let Some(dir) = current_exe.and_then(|exe| exe.parent()) {
        candidates.push(dir.join(UPDATER));
    }
    if let Some(base) = local_app_data {
        candidates.push(base.join("Zuin").join("bin").join(UPDATER));
    }
    candidates.into_iter().find(|p| usable(p)).ok_or_else(|| "找不到 0c-update；請重新安裝靈犀注音".to_string())
}

fn valid_key(key: &str) -> bool {
    let mut chars = key.chars();
    key.len() <= 40
        && chars.next().is_some_and(|c| c.is_ascii_lowercase())
        && chars.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

// 值不能以 - 開頭：0c-update 會把它當成旗標（例如 -apply），而不是設定的值。
// 含標點的值只放行明列的幾個：選字鍵 asdfghjkl;（#112，值就是那一排鍵）。
const PUNCTUATED_VALUES: &[&str] = &["asdfghjkl;"];

fn valid_value(value: &str) -> bool {
    PUNCTUATED_VALUES.contains(&value)
        || value.len() <= 40
            && value.chars().next().is_some_and(|c| c.is_ascii_lowercase() || c.is_ascii_digit())
            && value.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

/// 前端送來的設定修改：數量與字元都限制住，不讓任意字串變成 0c-update 的參數。
/// 這只是第一道檢查；鍵是否存在、值是否合法由 0c-update 判斷。
pub fn validate_pairs(pairs: &[(String, String)]) -> Result<(), String> {
    if pairs.is_empty() || pairs.len() > 16 {
        return Err("設定修改的數量不正確".to_string());
    }
    for (key, value) in pairs {
        if !valid_key(key) || !valid_value(value) {
            return Err(format!("設定 {key:?} 的格式不正確"));
        }
    }
    Ok(())
}

/// Only the displayed release and checksum can be passed to fixed update actions.
pub fn update_action_args(action: &str, version: &str, checksum: &str) -> Result<Vec<String>, String> {
    let command = match action {
        "download" => "download-now",
        "install" => "finish",
        _ => return Err("未知的更新動作".to_string()),
    };
    let version_ok = version.len() <= 80
        && version.chars().next().is_some_and(|c| c.is_ascii_digit())
        && version.chars().all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.');
    if !version_ok || checksum.len() != 64 || !checksum.chars().all(|c| c.is_ascii_digit() || ('a'..='f').contains(&c)) {
        return Err("更新目標格式不正確，請重新檢查更新".to_string());
    }
    Ok([command, "-json", "-target", version, "-target-sha256", checksum].iter().map(|s| s.to_string()).collect())
}

/// 前端可以送的詞彙指令（core/engine 的 VocabRequest.op）。
pub const VOCAB_OPS: &[&str] = &[
    "status", "list", "add", "remove", "readings", "undo", "batches", "rank", "annotate", "learned", "keep", "forget",
    "unlearn", "forgotten", "relearn", "learned-undo", "phrases", "phrase-add", "phrase-remove", "phrase-move",
];

/// 一個詞彙指令最多這麼大（500 個詞或片語的上限遠低於此）。
const VOCAB_REQUEST_MAX: usize = 1 << 20;

/// 檢查前端送來的詞彙指令並序列化成 0c-update vocab request 的標準輸入。
/// 只接受已知的指令；來源（source）一律拿掉，由 Go 端當成「手動」：畫面不能冒充 AI 助手或組字中加入。
pub fn vocab_request_body(request: &Value) -> Result<Vec<u8>, String> {
    let mut object = request.as_object().cloned().ok_or("詞彙指令格式不正確")?;
    let op = object.get("op").and_then(Value::as_str).unwrap_or("");
    if !VOCAB_OPS.contains(&op) {
        return Err(format!("未知的詞彙指令 {op:?}"));
    }
    object.remove("source");
    let body = serde_json::to_vec(&Value::Object(object)).map_err(|e| e.to_string())?;
    if body.len() > VOCAB_REQUEST_MAX {
        return Err("一次送出的內容太多，請分批加入".to_string());
    }
    Ok(body)
}

/// 「讓 AI 幫你加詞」的固定動作對應到 0c-update api 的參數。埠號只在 enable 時有意義。
pub fn api_args(action: &str, port: Option<u16>) -> Result<Vec<String>, String> {
    let mut out: Vec<String> = match action {
        "status" => vec!["api", "status"],
        "enable" => vec!["api", "enable"],
        "disable" => vec!["api", "disable"],
        "regen" => vec!["api", "token", "-regen"],
        "test" => vec!["api", "test"],
        _ => return Err("未知的 AI 助手動作".to_string()),
    }
    .into_iter()
    .map(String::from)
    .collect();
    out.push("-json".to_string());
    if let Some(port) = port {
        if action != "enable" || port < 1024 {
            return Err("埠號要在 1024～65535 之間".to_string());
        }
        out.push("-port".to_string());
        out.push(port.to_string());
    }
    Ok(out)
}

/// 「模型」頁（#180）只讀狀態：0c-update models status -json 的參數與逾時（秒），不連網。
pub fn models_args() -> (Vec<String>, u64) {
    (vec!["models".to_string(), "status".to_string(), "-json".to_string()], 20)
}

/// 「清除學習紀錄」（#301）：只有 reset。0c-update personal reset -json 在引擎執行中時請它清空並存檔，
/// 遇到不支援的舊引擎會先等它交接（最多 40 秒），所以逾時放寬到 120 秒。
pub fn personal_args(action: &str) -> Result<(Vec<String>, u64), String> {
    match action {
        "reset" => Ok((vec!["personal".to_string(), "reset".to_string(), "-json".to_string()], 120)),
        _ => Err("未知的學習紀錄動作".to_string()),
    }
}

/// 設定視窗本身不離開本機頁面（local_origin），外部網址交給系統瀏覽器；只放行 Hugging Face 的模型頁
/// （0c-update models status 的 url），字元限制在網址路徑用得到的範圍，不會被當成參數或指令。
pub fn browser_url(url: &str) -> Result<&str, String> {
    const PREFIX: &str = "https://huggingface.co/";
    let rest = url.strip_prefix(PREFIX).ok_or("只能開啟 Hugging Face 的網址")?;
    let ok = !rest.is_empty()
        && url.len() <= 300
        && !rest.contains("..")
        && !rest.starts_with('/')
        && rest.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '/' | '.' | '_' | '-'));
    if ok {
        Ok(url)
    } else {
        Err("網址格式不正確".to_string())
    }
}

/// 只支援 stdio 的 AI 工具要填的 0c-mcp 路徑：不隨版本改變的位置（範例 5），%LOCALAPPDATA%\Zuin\bin 的穩定副本。
pub fn planned_mcp_path(local_app_data: Option<&Path>) -> Option<PathBuf> {
    local_app_data.map(|base| base.join("Zuin").join("bin").join("0c-mcp.exe"))
}

fn tail(bytes: &[u8], max: usize) -> String {
    let text = String::from_utf8_lossy(bytes);
    let text = text.trim();
    let start = text.char_indices().rev().nth(max).map(|(i, _)| i).unwrap_or(0);
    text[start..].to_string()
}

/// 0c-update 的契約輸出是一份 JSON 物件（成功或 ok:false 的錯誤文件）；不論結束碼都原樣交給前端。
/// 不是 JSON 時才算失敗，並附上 stderr 的尾端說明原因。
pub fn parse_output(stdout: &[u8], stderr: &[u8], code: Option<i32>) -> Result<Value, String> {
    match serde_json::from_slice::<Value>(stdout) {
        Ok(value) if value.is_object() => Ok(value),
        _ => Err(format!(
            "0c-update 沒有回傳可解析的結果（結束碼 {}）：{}",
            code.map(|c| c.to_string()).unwrap_or_else(|| "無".to_string()),
            tail(stderr, 300)
        )),
    }
}

/// 執行 0c-update 並等它結束；超過 timeout 就強制結束它。stdout 與 stderr 各用一條執行緒讀，避免管線塞滿卡死。
pub fn run_blocking(updater: &Path, args: &[String], timeout: Duration) -> Result<Value, String> {
    run_with_input(updater, args, None, timeout)
}

/// 同 run_blocking，另外把 input 寫進標準輸入（vocab request 的 JSON 指令），寫完就關閉。
pub fn run_with_input(updater: &Path, args: &[String], input: Option<Vec<u8>>, timeout: Duration) -> Result<Value, String> {
    let mut command = Command::new(updater);
    let stdin = if input.is_some() { Stdio::piped() } else { Stdio::null() };
    command.args(args).stdin(stdin).stdout(Stdio::piped()).stderr(Stdio::piped()).creation_flags(CREATE_NO_WINDOW);
    let mut child = command.spawn().map_err(|e| format!("無法啟動 0c-update：{e}"))?;
    let writer = match (input, child.stdin.take()) {
        (Some(bytes), Some(mut pipe)) => Some(thread::spawn(move || {
            use std::io::Write;
            let _ = pipe.write_all(&bytes); // 對方提早結束時寫入失敗：結果看它的輸出
        })),
        _ => None,
    };
    let mut out = child.stdout.take().ok_or("無法讀取 0c-update 的輸出")?;
    let mut err = child.stderr.take().ok_or("無法讀取 0c-update 的錯誤輸出")?;
    let read_out = thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = out.read_to_end(&mut buf);
        buf
    });
    let read_err = thread::spawn(move || {
        let mut buf = Vec::new();
        let _ = err.read_to_end(&mut buf);
        buf
    });
    let deadline = Instant::now() + timeout;
    let status = loop {
        match child.try_wait() {
            Ok(Some(status)) => break Some(status),
            Ok(None) if Instant::now() >= deadline => {
                let _ = child.kill();
                let _ = child.wait();
                break None;
            }
            Ok(None) => thread::sleep(Duration::from_millis(20)),
            Err(e) => return Err(format!("等待 0c-update 失敗：{e}")),
        }
    };
    let stdout = read_out.join().unwrap_or_default();
    let stderr = read_err.join().unwrap_or_default();
    if let Some(writer) = writer {
        let _ = writer.join();
    }
    match status {
        None => Err(format!("0c-update 執行逾時（超過 {} 秒）", timeout.as_secs())),
        Some(status) => parse_output(&stdout, &stderr, status.code()),
    }
}

/// 把輸出裡的家目錄路徑換成 ~，匯出的摘要才不帶使用者名稱。
///
/// Windows 的家目錄含反斜線，輸出是 JSON 時每個反斜線會變成兩個，所以連同跳脫後與正斜線的寫法一起換；
/// 只換原樣的寫法會把使用者名稱留在摘要裡。
pub fn scrub_home(text: &str, home: &str) -> String {
    if home.len() < 2 {
        return text.to_string();
    }
    let mut out = text.replace(&home.replace('\\', "\\\\"), "~");
    if home.contains('\\') {
        out = out.replace(&home.replace('\\', "/"), "~");
    }
    out.replace(home, "~")
}

/// 西曆日期與時間（UTC）。Howard Hinnant 的 civil_from_days，免得為了一個時間戳多一個相依套件。
pub fn civil(secs: u64) -> (i64, u32, u32, u32, u32, u32) {
    let days = (secs / 86_400) as i64;
    let rem = (secs % 86_400) as u32;
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = (doy - (153 * mp + 2) / 5 + 1) as u32;
    let month = if mp < 10 { mp + 3 } else { mp - 9 } as u32;
    let year = yoe + era * 400 + i64::from(month <= 2);
    (year, month, day, rem / 3600, rem % 3600 / 60, rem % 60)
}

pub fn file_stamp(secs: u64) -> String {
    let (y, mo, d, h, mi, s) = civil(secs);
    format!("{y:04}{mo:02}{d:02}-{h:02}{mi:02}{s:02}")
}

pub fn iso_time(secs: u64) -> String {
    let (y, mo, d, h, mi, s) = civil(secs);
    format!("{y:04}-{mo:02}-{d:02}T{h:02}:{mi:02}:{s:02}Z")
}

/// 匯出的狀態摘要：版本、平台、引擎與設定的狀態、更新狀態。不含輸入內容、個人詞庫或使用者名稱。
pub fn build_summary(status: &Value, settings: &Value, app_version: &str, secs: u64, home: &str) -> String {
    let doc = serde_json::json!({
        "generated_utc": iso_time(secs),
        "settings_program": app_version,
        "platform": format!("{}/{}", std::env::consts::OS, std::env::consts::ARCH),
        "status": status,
        "settings": settings,
    });
    scrub_home(&serde_json::to_string_pretty(&doc).unwrap_or_default(), home)
}

/// 「我的詞彙」匯出（#255）的文字檔上限：一萬個詞、各 4 個讀音加 200 則千字片語也遠小於這個大小。
pub const VOCAB_EXPORT_MAX: usize = 16 << 20;

/// 匯出檔放的位置：下載資料夾，沒有就家目錄（與狀態摘要相同）。
pub fn download_dir(home: &Path) -> PathBuf {
    let downloads = home.join("Downloads");
    if downloads.is_dir() { downloads } else { home.to_path_buf() }
}

/// 寫一個新檔案：不覆寫既有的檔案（使用者資料夾本來就只有自己能讀）。
pub fn write_private(path: &Path, bytes: &[u8]) -> std::io::Result<()> {
    use std::io::Write;
    std::fs::OpenOptions::new().write(true).create_new(true).open(path)?.write_all(bytes)
}

/// 在 dir 寫 stem.ext；已經有同名的檔案（同一秒匯出兩次）就改名 stem-2.ext、stem-3.ext…，回傳寫好的路徑。
pub fn write_new_file(dir: &Path, stem: &str, ext: &str, bytes: &[u8]) -> std::io::Result<PathBuf> {
    for n in 1..=100 {
        let name = if n == 1 { format!("{stem}.{ext}") } else { format!("{stem}-{n}.{ext}") };
        let path = dir.join(name);
        match write_private(&path, bytes) {
            Ok(()) => return Ok(path),
            Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => continue,
            Err(e) => return Err(e),
        }
    }
    Err(std::io::Error::new(std::io::ErrorKind::AlreadyExists, "同名的檔案太多"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;
    use std::fs;

    fn temp(name: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("zc-settings-test-{}-{}", std::process::id(), name));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    /// 測試用的替身：批次檔（Rust 經 cmd.exe 執行 .cmd），找路徑的測試只看檔案在不在。
    fn script(dir: &Path, name: &str, body: &str) -> PathBuf {
        let path = dir.join(name);
        fs::write(&path, format!("@echo off\r\n{body}\r\n")).unwrap();
        path
    }

    fn strings(items: &[&str]) -> Vec<String> {
        items.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn page_comes_from_the_whitelist_only() {
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "update"])), "update");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "vocab"])), "vocab");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "models"])), "models");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "input"])), "input");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "mode"])), "mode");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "candidates"])), "candidates");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "keyboard"])), "keyboard");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "account"])), "overview");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page", "../etc"])), "overview");
        assert_eq!(page_from_args(&strings(&["0c-settings", "--page"])), "overview");
        assert_eq!(page_from_args(&strings(&["0c-settings"])), "overview");
    }

    #[test]
    fn updater_prefers_the_one_that_launched_us() {
        let dir = temp("find");
        let launcher = script(&dir, "launcher-update.cmd", "");
        let sibling_dir = dir.join("app");
        fs::create_dir_all(&sibling_dir).unwrap();
        let sibling = script(&sibling_dir, UPDATER, "");
        let exe = sibling_dir.join("0c-settings.exe");
        let local = dir.join("Local");
        let stable_dir = local.join("Zuin").join("bin");
        fs::create_dir_all(&stable_dir).unwrap();
        let stable = script(&stable_dir, UPDATER, "");

        let found = |env: Option<&str>, exe: Option<&Path>| find_updater(env, exe, Some(&local));
        assert_eq!(found(launcher.to_str(), Some(&exe)).unwrap(), launcher);
        assert_eq!(found(None, Some(&exe)).unwrap(), sibling);
        assert_eq!(found(Some(""), Some(&exe)).unwrap(), sibling);
        assert_eq!(found(Some(r"relative\path"), None).unwrap(), stable);
        assert_eq!(found(Some(r"C:\does\not\exist"), None).unwrap(), stable);
        // 版本目錄裡旁邊沒有 0c-update 時用穩定副本；兩處都沒有時回報錯誤。
        let lonely = dir.join("versions").join("x").join("0c-settings.exe");
        assert_eq!(found(None, Some(&lonely)).unwrap(), stable);
        assert!(find_updater(None, Some(&lonely), Some(&dir.join("elsewhere"))).is_err());
    }

    #[test]
    fn only_plain_keys_and_values_reach_the_updater() {
        let ok = |k: &str, v: &str| (k.to_string(), v.to_string());
        assert!(validate_pairs(&[ok("selection", "statistical"), ok("fuzzy-z-zh", "select"), ok("document-context", "off")]).is_ok());
        // 選字鍵（#112）的三個值；asdfghjkl; 是明列放行的含標點值。
        for keys in ["1234567890", "1234qweras", "asdfghjkl;"] {
            assert!(validate_pairs(&[ok("selection-keys", keys)]).is_ok(), "{keys}");
        }
        assert!(validate_pairs(&[ok("selection-keys", "asdfghjk;l")]).is_err());
        // 旗標（以 - 開頭的鍵或值）、空值、大寫、空白、標點與非英數字元都擋掉。
        for bad in [ok("-apply", "on"), ok("Selection", "neural"), ok("selection", ""), ok("selection", "a b"), ok("selection", "x;y"),
                    ok("", "on"), ok("1abc", "on"), ok("selection", "--json"), ok("selection", "-apply"), ok("a/b", "on"),
                    ok("selection", "神經"), ok("selection", &"x".repeat(41))] {
            assert!(validate_pairs(&[bad.clone()]).is_err(), "應拒絕 {bad:?}");
        }
        assert!(validate_pairs(&[]).is_err());
        assert!(validate_pairs(&vec![ok("selection", "neural"); 17]).is_err());
    }

    #[test]
    fn contract_documents_pass_through_whatever_the_exit_code() {
        let error_doc = br#"{"schema":1,"ok":false,"error":{"code":"unknown_key","message":"x"}}"#;
        let value = parse_output(error_doc, b"", Some(1)).unwrap();
        assert_eq!(value["error"]["code"], "unknown_key");
        assert_eq!(parse_output(br#"{"ok":true}"#, b"", Some(0)).unwrap()["ok"], true);
        let failed = parse_output(b"not json", "error while loading shared libraries".as_bytes(), Some(127)).unwrap_err();
        assert!(failed.contains("127") && failed.contains("shared libraries"), "{failed}");
        assert!(parse_output(b"[1,2]", b"", Some(0)).is_err(), "契約輸出一定是物件");
        assert!(parse_output(b"", b"", None).unwrap_err().contains("無"));
    }

    #[test]
    fn runs_the_updater_with_exact_arguments_and_parses_its_json() {
        let dir = temp("run");
        let updater = script(&dir, "0c-update.cmd", r#"echo {"ok":true,"args":"%1 %2 %3"}"#);
        let value = run_blocking(&updater, &strings(&["set", "-json", "-apply"]), Duration::from_secs(5)).unwrap();
        assert_eq!(value["args"], "set -json -apply");
        // 結束碼 1 但輸出契約錯誤文件：照樣交給前端。
        let failing = script(&dir, "fails.cmd", "echo {\"schema\":1,\"ok\":false,\"error\":{\"code\":\"busy\"}}\r\nexit /b 1");
        assert_eq!(run_blocking(&failing, &[], Duration::from_secs(5)).unwrap()["error"]["code"], "busy");
    }

    #[test]
    fn a_hung_updater_is_killed_at_the_timeout() {
        // 直接執行 PING.EXE：批次檔被結束時，它啟動的子行程不會跟著結束。
        let ping = Path::new(&std::env::var_os("SystemRoot").unwrap()).join("System32").join("PING.EXE");
        let start = Instant::now();
        let error = run_blocking(&ping, &strings(&["-n", "30", "127.0.0.1"]), Duration::from_millis(300)).unwrap_err();
        assert!(error.contains("逾時"), "{error}");
        assert!(start.elapsed() < Duration::from_secs(5), "逾時後要立刻結束它，不能等到它自己結束");
    }

    #[test]
    fn large_output_does_not_deadlock_the_pipe() {
        let dir = temp("large");
        fs::write(dir.join("big.json"), format!("{{\"pad\":\"{}\"}}", "x".repeat(2000 * 64))).unwrap();
        let big = script(&dir, "0c-update.cmd", r#"type "%~dp0big.json""#);
        let value = run_blocking(&big, &[], Duration::from_secs(10)).unwrap();
        assert_eq!(value["pad"].as_str().unwrap().len(), 2000 * 64);
    }

    #[test]
    fn missing_executable_is_reported_not_panicked() {
        let error = run_blocking(Path::new("/nonexistent/0c-update"), &[], Duration::from_secs(1)).unwrap_err();
        assert!(error.contains("無法啟動"), "{error}");
    }

    #[test]
    fn update_actions_bind_the_displayed_target_and_reject_flags() {
        let checksum = "a".repeat(64);
        let args = update_action_args("download", "0.1.0-dev.49", &checksum).unwrap();
        assert_eq!(args, vec!["download-now", "-json", "-target", "0.1.0-dev.49", "-target-sha256", &checksum]);
        assert_eq!(update_action_args("install", "0.1.0-dev.49", &checksum).unwrap()[0], "finish");
        for (action, version, hash) in [("shell", "0.1.0-dev.49", checksum.as_str()), ("download", "-feed", checksum.as_str()), ("install", "0.1.0-dev.49", "--json")] {
            assert!(update_action_args(action, version, hash).is_err());
        }
    }

    #[test]
    fn timestamps_are_correct_utc() {
        assert_eq!(file_stamp(0), "19700101-000000");
        assert_eq!(file_stamp(951_782_400), "20000229-000000"); // 閏日
        assert_eq!(file_stamp(1_700_000_000), "20231114-221320");
        assert_eq!(iso_time(1_791_248_523), "2026-10-06T01:02:03Z");
        assert_eq!(file_stamp(4_102_444_799), "20991231-235959");
    }

    #[test]
    fn vocab_export_goes_to_the_download_folder_without_overwriting() {
        let home = temp("vocab-export");
        assert_eq!(download_dir(&home), home, "沒有下載資料夾時放家目錄");
        fs::create_dir(home.join("Downloads")).unwrap();
        assert_eq!(download_dir(&home), home.join("Downloads"));

        let dir = home.join("Downloads");
        let first = write_new_file(&dir, "0c-vocab-20261104-010203", "txt", "陳怡君\tㄔㄣˊ ㄧˊ ㄐㄩㄣ\n".as_bytes()).unwrap();
        let second = write_new_file(&dir, "0c-vocab-20261104-010203", "txt", b"second").unwrap();
        assert_eq!(first, dir.join("0c-vocab-20261104-010203.txt"));
        assert_eq!(second, dir.join("0c-vocab-20261104-010203-2.txt"), "同一秒再匯出不覆寫，改名");
        assert_eq!(fs::read_to_string(&first).unwrap(), "陳怡君\tㄔㄣˊ ㄧˊ ㄐㄩㄣ\n");
        let _ = fs::remove_dir_all(&home);
    }

    #[test]
    fn summary_keeps_state_and_drops_the_home_directory() {
        // JSON 文字裡的反斜線是跳脫過的，正斜線寫法也要換。
        let status = json!({"installed": "0.1.0-dev.43", "runtime": {"state": "running"}});
        let settings = json!({"file": {"path": r"C:\Users\alice\AppData\Local\Zuin\settings.json", "alt": "C:/Users/alice/x"}});
        let text = build_summary(&status, &settings, "0.1.0-dev.43", 1_791_248_523, r"C:\Users\alice");
        assert!(!text.contains("alice"), "{text}");
        assert!(text.contains(r"~\\AppData\\Local\\Zuin\\settings.json") && text.contains("~/x"), "{text}");
        assert!(text.contains("0.1.0-dev.43") && text.contains("2026-10-06T01:02:03Z"));
        assert_eq!(scrub_home("/x", "/"), "/x", "家目錄太短時不替換");
    }

    #[test]
    fn vocab_requests_are_whitelisted_and_cannot_claim_another_source() {
        let body = vocab_request_body(&json!({"op": "add", "source": "assistant", "words": [{"word": "陳怡君", "readings": ["ㄔㄣˊ ㄧˊ ㄐㄩㄣ"]}]})).unwrap();
        let sent: Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(sent["op"], "add");
        assert!(sent.get("source").is_none(), "來源由 Go 端決定（手動）");
        assert_eq!(sent["words"][0]["word"], "陳怡君");
        for bad in [json!({"op": "shell"}), json!({"op": ""}), json!({}), json!(["list"]), json!("list"), json!({"op": 3})] {
            assert!(vocab_request_body(&bad).is_err(), "應拒絕 {bad}");
        }
        let huge = json!({"op": "phrase-add", "texts": ["字".repeat(400_000)]});
        assert!(vocab_request_body(&huge).is_err());
        for op in VOCAB_OPS {
            assert!(vocab_request_body(&json!({"op": op})).is_ok());
        }
    }

    #[test]
    fn api_actions_map_to_fixed_arguments() {
        assert_eq!(api_args("status", None).unwrap(), strings(&["api", "status", "-json"]));
        assert_eq!(api_args("regen", None).unwrap(), strings(&["api", "token", "-regen", "-json"]));
        assert_eq!(api_args("enable", Some(47200)).unwrap(), strings(&["api", "enable", "-json", "-port", "47200"]));
        assert_eq!(api_args("test", None).unwrap(), strings(&["api", "test", "-json"]));
        assert!(api_args("token", None).is_err());
        assert!(api_args("enable", Some(80)).is_err());
        assert!(api_args("status", Some(47200)).is_err());
    }

    #[test]
    fn mcp_path_is_the_version_independent_location() {
        let local = Path::new(r"C:\Users\alice\AppData\Local");
        assert_eq!(planned_mcp_path(Some(local)).unwrap(), PathBuf::from(r"C:\Users\alice\AppData\Local\Zuin\bin\0c-mcp.exe"));
        assert_eq!(planned_mcp_path(None), None);
    }

    #[test]
    fn requests_reach_the_updater_on_standard_input() {
        let dir = temp("stdin");
        let updater = script(&dir, "0c-update.cmd", "set /p line=\r\necho {\"ok\":true,\"got\":%line%}");
        let value = run_with_input(&updater, &strings(&["vocab", "request"]), Some(br#"{"op":"list"}"#.to_vec()), Duration::from_secs(5));
        assert_eq!(value.unwrap()["got"]["op"], "list");
    }

    #[test]
    fn personal_args_only_reset() {
        assert_eq!(personal_args("reset").unwrap(), (strings(&["personal", "reset", "-json"]), 120));
        for bad in ["", "status", "reset -json", "-json", "remove"] {
            assert!(personal_args(bad).is_err(), "{bad:?}");
        }
    }

    #[test]
    fn models_args_only_status() {
        assert_eq!(models_args(), (strings(&["models", "status", "-json"]), 20));
    }

    #[test]
    fn browser_url_only_hugging_face_paths() {
        let ok = "https://huggingface.co/Funmula/0c-sequence-d512/tree/df4b9f04aa11";
        assert_eq!(browser_url(ok), Ok(ok));
        for bad in [
            "http://huggingface.co/Funmula/x",
            "https://huggingface.co.evil.example/x",
            "https://huggingface.co/",
            "https://huggingface.co//evil.example",
            "https://huggingface.co/Funmula/x?a=1&b=2",
            "https://huggingface.co/Funmula/x\"&calc",
            "https://huggingface.co/Funmula/../x",
            "https://huggingface.co/Funmula/x y",
            "file:///etc/passwd",
            "javascript:alert(1)",
        ] {
            assert!(browser_url(bad).is_err(), "{bad:?}");
        }
    }
}
