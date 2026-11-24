# 靈犀注音設定（Tauri）

獨立的設定程式（#47）：Rust 外殼＋系統 WebView，畫面是純 HTML／CSS／JS，沒有 npm、沒有打包器。設定邏輯與更新驗證都在 Go；這個程式只呼叫 `0c-update` 的 JSON 契約（[JSON 契約實作](../../cmd/0c-update/contract.go)），沒有網路、登入或機密。

## 結構

| 路徑 | 內容 |
|---|---|
| `src/updater.rs` | 找出並啟動 `0c-update`、逾時、解析 JSON、匯出摘要；附單元測試 |
| `src/main.rs` | 指令白名單、視窗、單一實例 |
| `src/watch.rs` | 「我的詞彙」的即時同步：常駐 `0c-update vocab watch`、轉成 `vocab-changed` 事件、意外結束時重新連上 |
| `ui/` | `index.html`、`app.css`、`app.js`；設定項目依契約動態產生。`vocab.js` 是「我的詞彙」頁，`models.js` 是「模型」頁，`personal-reset.js` 是「我的詞彙」頁「靈犀學到的」底下的「清除學習紀錄」，`update-mode.js` 是更新方式的顯示名稱與說明 |
| `test/` | `*_test.js`（`node --test`，不碰畫面的判斷）、`scroll_position_test.py`（Playwright 在 Chromium 跑真的頁面，背景更新保留捲動位置，#268；沒有 Chromium 就跳過）、`settings-update-test.ps1`（更新視窗驗收） |
| `build.rs`、`capabilities/main.json` | 只有 `build.rs` 列出的指令能被前端呼叫，capability 只授權這些指令與事件監聽 |
| `tauri.conf.json` | CSP：`default-src 'none'; script-src 'self'; style-src 'self'`；不載入任何遠端內容 |

## 中英切換、選字、鍵盤與符號（輸入設定）

輸入設定分成側欄的三頁，不摺疊（摺起來的設定會找不到）。各頁依 `app.js` 的 `INPUT_PAGES` 排列，順序也是 Tab 順序：

| 頁面（`--page`） | 項目 |
|---|---|
| 中英切換（`mode`） | 中英切換鍵、單按 Shift 的時限、開始時的中英模式、切換視窗後的中英模式、英文模式自動補字 |
| 選字（`candidates`） | 選字方式（神經選字）、依前文選字、修改字詞的位置、選字後游標移到下一個字、選字排列、每頁候選數、選字鍵、按 Esc 清除整段組字、記住我選的字詞、組字時的 Tab、候選窗的 Tab／空白鍵／Enter、候選方向鍵，另一張卡是模糊音總開關 |
| 鍵盤與符號（`keyboard`） | 提交時補標點（Beta）、Shift＋空白、Shift＋Delete |

中英混打、標點照微軟、所有符號鍵打半形、Ctrl＋標點打全形、標準注音排列都寫死在引擎，沒有設定；舊設定檔裡這些欄位照常讀得進來、存檔時保留，但不起作用（`internal/settings` 的 `retiredKeys`）。

側欄順序：總覽、中英切換、選字、鍵盤與符號、我的詞彙、模型、更新、問題排解、關於。契約裡不在任何一頁的新項目（較新的 0c-update 才有）畫在「選字」最後，所以新增設定不必先改畫面；要放別頁再加進 `INPUT_PAGES`。舊的頁名 `input` 照樣接受，畫面導到側欄第一頁「中英切換」（`PAGE_ALIAS`）。三份頁名清單由 `internal/settingsapp` 的 `TestPageNamesMatchTheSettingsApp` 核對。總覽的「有設定已儲存、尚未套用」帶到第一個尚未套用的項目所在的那一頁。

契約的項目帶 `disabled` 時（不用 Shift 時的開始模式與單按 Shift 的時限）開關反灰、畫成關閉，名稱旁顯示 `disabled_reason`，不在 Tab 順序裡；設定檔的值不動。造成停用的那一項（`disabled_by`）在別頁時，說明下面多一個「到「…」修改「…」」，按了切到那一頁並把焦點放在那一項；在同一頁不另外放。

模糊音顯示一列總開關，預設開啟（四組 select），開啟時以同一次設定交易將四組近音都設為 select，關閉時全部設 off；沒有逐組控制或自動修正單選。舊設定只啟用部分配對或使用 auto 時，畫面視為開啟，載入不改寫；關閉再開啟才統一成四組 select。CLI 的逐組設定與原有檔案格式仍保留，未知欄位與唯讀保護沿用既有契約。

## 我的詞彙（0.2.0，#125）

`features.vocab` 為真時（0.2.0 起的 `0c-update` 一律為真；舊版沒有這個欄位時不顯示），側欄「鍵盤與符號」下面出現「我的詞彙」，總覽多一張小卡（詞數與 AI 助手最近加的數量；沒有詞時是「把常打錯的人名、術語加進來」）。後端命令、Tauri 指令與事件見 [詞彙命令實作](../../cmd/0c-update/vocab.go)。

- **我加的詞**：頂端是搜尋與新增列。打字時（組字結束後 250 毫秒）查 `annotate`，每個字一格讀音：破音字是黃色下拉選單，查不到的字自己填注音；Enter 加入。英文詞（`ls`、`kubectl`，#146）不查讀音，直接 Enter 加入，清單上讀音的位置標示「英文」、沒有排名。一次貼上多行變成預覽清單（不是漢字也不是英文詞的行不能勾），勾選後一起加入（超過 500 個分批送，整批復原）；「詞＋Tab＋讀音」的行照那個讀音加，不重查。每列：詞、讀音（點了改，可加到 4 個讀音，送出時附 `expect`）、排名徽章（`rank`：首選／第 N 個／第 N 頁，滑過顯示前面是哪個詞）、來源與時間、刪除。超過 1,000 筆分頁，搜尋交給引擎（`list` 的 `query`）。
- **匯出、匯入**（#255）：標題列右邊兩個按鈕。「匯出」把我加的詞與我的片語存成文字檔（`ui/vocab-text.js` 組字串，Rust 的 `export_vocab` 寫到下載資料夾），頁首顯示路徑。「匯入」開 WebView 的檔案對話框（`<input type="file">`），讀進來和貼上多行同一個預覽：上方是摘要（已經有的略過幾個、幾行不能加）與「加入勾選的…」，清單超過 12 列時在框內捲動，片語列在詞下面。格式與合併規則見 [`ui/vocab-text.js`](ui/vocab-text.js)。
- **靈犀學到的**（#182）：`learned` 清單，`kind` 顯示成「常一起用」（`phrase`）或「你選過的寫法」（`reading`），不顯示次數；頂端說明三個動作，每列有勾選框與「保留」（`keep`）、「移除」（`unlearn`）、「不再學」（`forget`），已是自訂詞的顯示「已在我加的詞」、不能勾。勾選列有全選（部分勾選時半勾）與三個批次按鈕，一次送一個指令。移除、不再學、恢復學習之後先拿掉那幾列，底部提示附「復原」（`learned-undo`），焦點交給它；保留是自訂詞的批次，照舊從「最近變更」也能復原。`forgotten_count` 大於 0 時右上角出現「不再學的詞（N）」，打開是另一個清單（`forgotten`），每列與批次都能「恢復學習」（`relearn`），「‹ 靈犀學到的」返回；全部恢復後自動返回、入口消失。清單最後是「清除學習紀錄」（#301，見下面一節；「不再學的詞」裡不顯示）。
- **我的片語**：新增（Ctrl＋Enter）、刪除、↑↓ 或拖曳把手排序（`phrase-move`）。拖曳用指標事件，不用 HTML5 拖放（各平台 WebView 的拖放處理不一致）。
- **最近變更**：最近 5 批（可展開全部）：時間、誰（你／AI 助手／打字時／學到的）、摘要、復原；點摘要看明細。
- **讓 AI 幫你加詞**（頁面最下方，預設收合）：開關旁一行「AI 助手讀得到你加的詞」；token 預設遮住，可顯示、複製、重新產生（先確認「已設定的 AI 工具要重新貼上設定」），列出建立與最後使用時間；「你用哪個 AI 工具？」只顯示選到的那段範例（網址、token、`0c-mcp` 路徑自動填好，未顯示 token 時範例裡也遮住，「複製」一律複製完整內容，含 token 的註明不要分享）；「測試連線」。埠被占用時可以換埠。
- 引擎沒有執行時只能查看：新增列停用、刪除與復原鈕藏起來，頁首說明。
- 這一頁的 DOM 只建一次，新增列與搜尋框在頁面存在期間不重建（組字中重建會打斷輸入法）；各區塊重畫時以 `data-k` 找回原本的鍵盤焦點，刪除後焦點交給底部提示的「復原」。

`0c-mcp` 隨套件內附，放在不隨版本變動的位置 `%LOCALAPPDATA%\Zuin\bin\0c-mcp.exe`（版本目錄那份的穩定副本）。那個位置沒有檔案（較舊的版本）時範例下方註明「這個版本還沒附 0c-mcp」。

## 模型（#180）

側欄「模型」在「更新」上面（`--page models`、`0c-update open-settings models`），總覽的「神經選字」列有「查看」連過來。只顯示資訊，沒有按鈕：

- 頁首一句狀態：內建模型（`source=bundled`）是「隨安裝包內附」；找不到內建模型（`source=none`，安裝損毀）說明目前用統計選字、重新安裝可以修復。
- 選字、補標點各一張卡片：引擎回報的狀態（文字同總覽）、版本、路徑、`model.onnx` 的 SHA-256（等寬字型、截斷顯示、可複製）；`0c-update models status` 給了 Hugging Face 網址（本機雜湊等於 `models.lock.json` 或 `equivalent.json` 時才有）就顯示連結與授權。

資料來自 `0c-update models status -json`（只讀、不連網）。Tauri 指令：

| 指令 | 參數 | 說明 |
|---|---|---|
| `models_command` | 無 | 對應 `0c-update models status -json`，結果原樣回傳；逾時 20 秒 |
| `open_url` | `url` | 交給系統瀏覽器（`rundll32 url.dll,FileProtocolHandler`）；只放行 `https://huggingface.co/` 開頭、路徑只有英數與 `/._-` 的網址。設定視窗本身照舊不離開本機頁面 |

## 清除學習紀錄（#301）

「我的詞彙」頁「靈犀學到的」分頁的最後一張卡：在學到的詞清單（含勾選列與批次按鈕）之後，「不再學的詞」子清單裡不顯示。`settings -json` 的 `features.personal_reset` 為真（0.2.12 起的 `0c-update`）才出現；引擎沒在跑、清單讀不到時照樣顯示（由 `0c-update` 直接刪檔）。「我的詞彙」要 `features.vocab` 為真才有（0.2.0 起恆為真）；為假的舊版 `0c-update` 也沒有 `personal_reset`，不另找地方放。不是開關：按「清除…」先在卡片下方確認（說明清除後選字回到剛安裝時的樣子、無法復原、你加的詞與片語不受影響；焦點先在「取消」），按「確認清除」才呼叫 Rust 的 `personal_command`（只允許 `reset`，對應 `0c-update personal reset -json`，逾時 120 秒）。成功後提示並把卡片說明換成「已清除學習紀錄…」，「靈犀學到的」先清空畫面、收起學到的詞的「復原」提示（引擎裡的復原複本也清掉了），再向引擎重讀 `learned`：清單變空、「不再學的詞（N）」入口消失；`busy` 提示稍後再試，其他錯誤顯示 `message`，都不動清單。引擎在不在跑、要刪哪些備份都由 Go 處理，輸出格式見 `cmd/0c-update/personal.go` 的 `personalResetResult`。

「我的詞彙」的 DOM 只建一次，這張卡放在清單旁的固定容器（`vocab.js` 的 `el.reset`），由 `personal-reset.js` 自己重畫：清單重新整理時確認不會被收掉，清單重畫保留焦點（`data-k`）也認得這幾個按鈕。

`test/personal_reset_test.js` 驗證確認文案、結果提示，以及只有成功時才通知「靈犀學到的」重讀。

## 建置

在 Windows x64 建置機上，用機器上已安裝的 MSVC 工具鏈 Rust（需要 1.90 以上；不安裝或修改工具鏈）：

```powershell
python scripts\build_settings_windows.py --version <版本> --test
```

輸出 `build\settings\windows-amd64\0c-settings.exe`（未簽章，約 3 MB）與 `NOTICES.txt`；`scripts/package_windows.py --settings` 把它放進套件、以版本資訊資源取代 Tauri 寫的那一份、與其他執行檔一起簽章。需要 `icons/icon.ico`（`tauri-build` 在 Windows 上要它產生資源檔）。直接用 `cargo build` 時**必須**加 `--features tauri/custom-protocol`，否則會編成開發模式並嘗試載入 `devUrl`。`Cargo.lock` 納入版本控制，建置一律 `--locked`。

## 安全界線

- 前端只能呼叫 `build.rs` 列出的指令；Rust 端限制鍵值的字元（不接受以 `-` 開頭的值，免得被當成旗標），Go 端再驗證一次。
- 視窗的 `on_navigation` 只允許自己的本機頁面；登入與付款以後一律開系統瀏覽器，不在這個 WebView 內進行。
- 所有畫面文字以 `textContent` 組成，不使用 `innerHTML`；CSP 不允許 inline script 與 style。
- 匯出的狀態摘要把家目錄換成 `~`，不含輸入內容、個人詞庫或使用者名稱。Windows 另含最近用過輸入法的程式檔名（不含路徑）與輸入法在各程式的判定（`shell.tsf_log`，#257）。

## 執行環境

- Windows 用 WebView2 Runtime（Windows 11 與近年的 Windows 10 內建，Evergreen 版）；缺少時由 `0c-update open-settings` 在啟動前偵測並說明。WebView2 的使用者資料夾放在 `%LOCALAPPDATA%\Zuin\settings-webview`，不另外建立以 identifier 命名的目錄。找不到 `0c-update` 時依序試 `ZC_UPDATER`、同資料夾（版本目錄）、`%LOCALAPPDATA%\Zuin\bin`（穩定副本）。匯出的狀態摘要把家目錄換成 `~`，連同 JSON 跳脫後的反斜線與正斜線寫法一起換。
- 單一實例：已開著就聚焦既有視窗並切到要求的頁面。

更新頁提供檢查更新、立即下載、背景下載加速及立即安裝。下載顯示實際百分比與已收到／總大小，每秒重讀執行狀態；核對和安裝顯示處理中的進度條。總覽與關於頁分開標示已安裝版本、目前執行的引擎及設定視窗版本，Windows 新外殼裝好後說明不必登出、已經開著的程式要重新開啟才會換成新版。後端命令及進度格式見 [JSON 契約實作](../../cmd/0c-update/contract.go)。

Windows 完整更新視窗驗收另有 [`settings-update-test.ps1`](../windows/test/settings-update-test.ps1)：在沒有安裝的專用測試機，以隔離個人資料目錄、已簽章測試 HTTPS 索引和真套件驗證立即下載、共用並加速背景下載、手動安裝，以及裝好後總覽的版本說明（不要求登出）。測試使用 e2etest 更新器；互動視窗以管理員執行，所以不包含原生 UAC 同意／取消對話框的真人驗證。

輸入設定的「按 Esc 清除整段組字」（`esc-clear`，`settings.json` 的 `esc_clear`）預設開啟，與多數注音輸入法相同。不論開關，Esc 都先清除還沒打完的注音；開啟時再按一次清除整段，關閉時保留已組好的字。引擎讀設定，套用（引擎重啟）後生效，外殼不必改。

輸入設定的「選字鍵」（`selection-keys`，`settings.json` 的 `selection_keys`）三選一：`1234567890`（預設，舊設定檔沒有欄位時也是）、`1234qweras`、`asdfghjkl;`，不開放自訂（#112）。候選窗依序用那一排鍵選第 1–9（每頁 9 個時）或 1–10 個字，鍵標由引擎隨候選送給外殼；換成字母鍵時數字 1–0 仍照位置選字（舊外殼、自己畫候選窗的程式只顯示數字也能選）。沒有候選窗時這些鍵照舊是注音鍵。畫在「選字」頁、每頁候選數之後。引擎讀設定，套用（引擎重啟）後生效。

輸入設定的「修改字詞的位置」（`candidate-cursor`）可選「游標前方」／「游標後方」。未指定時採後方，舊設定缺少欄位時也是；明確保存的 before／after 會保留。後方模式的游標在組字尾端時後面沒有字，改選結束在游標的詞（同前方模式），說明文字也註明這個例外。設定儲存在 `settings.json` 的 `candidate_cursor`，透過既有組字保護套用；不變更個人詞庫格式。

「單按 Shift 的時限」（`shift-tap-ms`，`settings.json` 的 `shift_tap_ms`，#319）是一組單選：0.2／0.3／0.4／0.5／0.6 秒（選項名稱在 `value_labels`），預設 0.3 秒，排在「中英切換鍵」之後。中英切換鍵選「不用 Shift」時反灰，寫「不用 Shift 切換時不作用」。引擎讀設定、在 health 的 `shift_tap_ms` 回報，套用（引擎重啟）後外殼下一次單按 Shift 就照新的時限。

「記住我選的字詞」（`personal-learning`，`settings.json` 的 `personal_learning`，#297）預設開啟，排在「選字」頁的按 Esc 清除整段組字之後、按鍵規則（組字時的 Tab 起）之前。關閉時照常選字（神經選字、依前文選字、已學到的照用），只是不再記下新的選擇、英文單字和最近用過的符號；「我的詞彙」照常。引擎讀設定，套用後生效。

輸入設定的「選字後游標移到下一個字」（`advance-after-select`，`settings.json` 的 `advance_after_select`）預設關閉，選字後游標留在原處。開啟後從候選窗選好字（數字鍵、Enter、滑鼠都算），游標移到所選詞的後面，可以接著按 ↓ 改下一個字；修改游標前方字詞時往後移一個字（同新酷音的 autoShiftCur），游標在句尾時不動（#133）。符號、Emoji 與原樣注音本來就插入在游標處、游標停在它們後面。引擎讀設定，套用（引擎重啟）後生效，外殼不必改。
