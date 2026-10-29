# hoshino 版（0c-biggo/0c-ime 的修改版）

這個 repo 是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的 fork，依 AGPL-3.0-only 發布修改後的版本。原作者的著作權與授權條款不變，見 `LICENSE` 與 `legal/`。

## 跟官方的差異

- **更新來源**：更新器信任的是 fork 自己的發行金鑰（`internal/update/trust_keys.go`），索引改從 fork 的 GitHub Releases 下載（`cmd/0c-update/auto.go` 的 `DefaultFeed`）。
- **程式碼簽章**：Windows 套件用 fork 的自簽憑證簽章（`platforms/windows/hoshino/codesign.cer`），發行者是「Hoshino 0c-ime fork」。打包腳本加了 `--sign-pfx` 和 `--publisher` 兩個選項。
- **版本號**：官方版號加上 `-hoshino.N`。
- **發布流程**：`.github/workflows/fork-release.yml` 自動合併官方新版、測試、打包、發布。開 PR 前先用 `platforms/windows/hoshino/local_preview.py` 在本機打未簽章的包，再用它產生的 `build/local-preview/install.ps1` 安裝實測（配方有新規則時會重建整合風格）。
- **輸入行為**：新增「整合風格」的配方（`blend`，`core/inputstyle`），以靈犀注音為底，補上華碩注音與微軟新注音的長處。它不是內建風格，用 `0c-update profile copy blend --name 整合風格` 複製成自訂風格，設定畫面上的設定都能調，整合風格加的英文排序與學習規則不在設定畫面。原本的各個模式行為不變。
  - 已完成：
    - 英文補全與拼字修正依 wordfreq 語料詞頻排序（`internal/englishpack/wordfreq`）
    - 英文字典裡的詞確認 3 次就學會
    - 預設開啟中英混打與英文模式自動補字；= 不開 Emoji 選單
    - 符號照微軟新注音：中文模式的標點出半形（Shift＋, 是 <，中文逗號用 Ctrl＋,）；沒在打字時打的符號也留在組字區，混打才看得到 $20 這類上下文；` 單純輸出 `；Ctrl＋[ ] 打最常用的「」，不是微軟的【】，按 ↓ 列微軟 Ctrl＋[ ] 的清單、「」與【】對調（「【『〔︹︻﹁﹃﹝［｢，要【就 ↓ 2）；Ctrl＋Shift＋; 9 0 照官方；英文模式的 Ctrl＋標點也打全形（`core/session/testdata/microsoft_symbols.tsv` 是對照的實測）
    - Shift＋空白切換全半形，全形時混打照常判斷，判成英數的段落轉全形（同華碩注音）
    - Ctrl＋Alt＋, 開華碩注音的符號表：鍵盤形狀，一般／數學／特殊三頁，Tab 換頁，按鍵或方向鍵加 Enter 選取
    - 混打打進組字區的字在 Enter 之前都找得回原按鍵（`core/session/mixed_span.go`）：↓ 與 Tab 換詞長可以跨過英數、原樣注音與數字鍵盤的數字，整段換成英文、注音或中文（$20␣ 換成「$單」）；數字鍵盤的數字一律只是數字。多字的 ↓ 清單（含純中文）同官方單字的順序：畫面上的字（不是詞時才列，選了不換）→ 中文 → 原始按鍵 → 整段注音 → 單字（#161）；換成注音或英文時，被切開的詞外面那一半照畫面留著
    - 混打照上下文重看（`core/session/mixed_context.go`）：官方照常判斷，每個按鍵處理完與送出前把組字區再看一次（待定的按鍵還沒定案時只看游標前），規則成立才改。重複鍵（ggggk6、jji3 是什、我）、聲母衝突（gck6 是 g合）、反序打的英文字（url␣ 不是「交」）、不是國語音節的注音（npm␣ 不是 ㄙㄩㄣ）改成英文或中文；注音文（a␣ 2␣ 1␣ 緊接在中文或注音後面是 ㄇ ㄉ ㄅ，句首、空白或標點後要連著兩個以上，ㄇ 後面接著打英文字時改回 a）；數字鍵盤夾在字母中間（e KP2 e）是一個英數詞 e2e，接聲調鍵照舊是中文（B1樓）。選過的字、從 ↓ 明確選的不改。const、sql 這類程式詞加到「我的詞彙」就照字面
    - 組字中的標示（`core/session/mark_span.go`）：Shift＋Home／End 一次標到頭尾（同一直按 Shift＋←／→，最多 32 字）；標示中按 ↓ 同從開頭按 ↓ 再按 Tab 換到那個長度，中間有空白或符號也能換整段；整段英文按 Enter 加成「我的詞彙」的英文詞
    - 候選開著時 Shift＋Tab 是 Tab 的反方向（操作規則 candidate_backtab）：換到較短的詞長，最短再按回到最長；待定的按鍵同 Tab 先照畫面定案，不會換到切開注音的長度
    - 混打修正記憶（操作規則 mixed_memory，`core/session/mixed_memory.go`、`core/personal/mixed_memory.go`）：用 ↓ 把一段按鍵換成別的解釋（mp3 出「允」選回 mp3、「我ㄇ」選回 a、$20␣ 換成「$單」）再按 Enter，記住「按鍵＋前一個字＋後一個音 → 每段當英數、中文或注音」，存在 user.json 的 `MixedMemory`（最多 1024 筆，最久沒用到的先丟）。前一個字、後一個音都對上就照記憶換；只有按鍵對上時看中文那一側合不合理（有神經模型時照前文評分，沒有時用語言模型；每個字平均 log10 機率以 -4 為界，每多確認一次、附近有英文都往不是中文那邊調），條件不再成立時換回（mp3 接著打出「允許」）。照記憶換的地方用 ↓ 改回會抵銷那筆記憶；候選窗反白畫面上的樣子時按 Shift＋Delete 刪掉記憶。中文選哪個字不記（官方的更正會記），私人欄位與暫停時照記憶換但不學
  - 整合風格跑官方測試：`go test -tags blendall ./core/session` 讓每個 session 都套用整合風格；預期不同的測試與原因列在 `core/session/testdata/blend_expected_diffs.txt`，`py -3.12 scripts/blend_official_diff.py` 核對其他官方測試都通過、列出的測試關掉對應的規則後也通過
  - 跟混打的相容：官方的規則照官方逐條判斷（`internal/settings` 的 `styleConflicts`）；fork 新加的規則欄位是為混打設計的，在 `inputstyle.Rules` 標 `fork:"blend"`，不在官方的衝突表上。非標準排列的限制也不算這些欄位（`internal/settings/mixed_compat.go` 的 `officialRulesDefault`）。

安裝與更新說明見 `platforms/windows/hoshino/`。
