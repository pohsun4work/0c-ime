# hoshi 版（0c-biggo/0c-ime 的修改版）

這個 repo 是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的 fork，依 AGPL-3.0-only 發布修改後的版本。原作者的著作權與授權條款不變，見 `LICENSE` 與 `legal/`。

## 跟官方的差異

- **更新來源**：更新器信任的是 fork 自己的發行金鑰（`internal/update/trust_keys.go`），索引改從 fork 的 GitHub Releases 下載（`cmd/0c-update/auto.go` 的 `DefaultFeed`）。
- **程式碼簽章**：Windows 套件用 fork 的自簽憑證簽章（`hoshi/windows/codesign.cer`），發行者是「Hoshi 0c-ime fork」。打包腳本加了 `--sign-pfx` 和 `--publisher` 兩個選項。
- **版本號**：官方版號加上 `-hoshi.N`。
- **發布流程**：`.github/workflows/fork-release.yml` 自動合併官方新版、測試、打包、發布。開 PR 前先用 `hoshi/windows/local_preview.py` 在本機打未簽章的包，再用它產生的 `build/local-preview/install.ps1` 安裝實測。
- **輸入行為**：新增「整合風格」，以靈犀注音為底，補上華碩注音與微軟新注音的長處。它不是一份風格：混打的開關、英文補全排序、符號表是全域設定（`internal/settings/blend.go`，設定檔的 `blend`），選哪個風格都照它；另外在官方的設定項加了選項（標點操作「微軟半形參考」、Shift＋空白的「切換全半形」用整合風格的全形）。作者的設定是把靈犀注音複製成自訂風格，開中英混打與英文模式自動補字、關 Emoji 選單、標點操作選微軟半形參考、Shift＋空白切換全半形（`hoshi/windows/INSTALL.md`）。原本的各個模式行為不變。
  - 已完成：
    - 英文補全與拼字修正依 wordfreq 語料詞頻排序（`internal/englishpack/wordfreq`）
    - 英文字典裡的詞確認 3 次就學會
    - 符號照微軟新注音：中文模式的標點出半形（Shift＋, 是 <，中文逗號用 Ctrl＋,）；沒在打字時打的符號也留在組字區，混打才看得到 $20 這類上下文；` 單純輸出 `；Ctrl＋[ ] 打最常用的「」，不是微軟的【】，按 ↓ 列微軟 Ctrl＋[ ] 的清單、「」與【】對調（「【『〔︹︻﹁﹃﹝［｢，要【就 ↓ 2）；Ctrl＋Shift＋; 9 0 照官方；英文模式的 Ctrl＋標點也打全形（`core/session/testdata/microsoft_symbols.tsv` 是對照的實測）
    - Shift＋空白切換全半形，全形時混打照常判斷，判成英數的段落轉全形（同華碩注音）
    - Ctrl＋Alt＋, 開華碩注音的符號表：鍵盤形狀，一般／數學／特殊三頁，Tab 換頁，按鍵或方向鍵加 Enter 選取
    - 混打打進組字區的字在 Enter 之前都找得回原按鍵（`core/session/mixed_span.go`）：↓ 與 Tab 換詞長可以跨過英數、原樣注音與數字鍵盤的數字，整段換成英文、注音或中文（$20␣ 換成「$單」）；Tab 換詞長同 Shift 標示，跨過空白到組字區頭尾（我hello␣你 換得到整段，最多 32 字），查不到詞的長度開整段的候選，但不切開一段按鍵（su 打的 ㄋㄧ 不換到只剩 ㄧ）；數字鍵盤的數字一律只是數字。多字的 ↓ 清單（含純中文）同官方單字的順序：畫面上的字（不是詞時才列，選了不換）→ 中文 → 原始按鍵 → 整段注音 → 單字（#161）；換成注音或英文時，被切開的詞外面那一半照畫面留著；字面的清單（mp3 的 3、npm␣ 的空白）Tab 換得到別的詞長時，標題一樣提示「Tab 切換詞長」，空白顯示成 ␣
    - 混打照上下文重看（`core/session/mixed_context.go`）：官方照常判斷，每個按鍵處理完與送出前把組字區再看一次（待定的按鍵還沒定案時只看游標前），規則成立才改。重複鍵（ggggk6、jji3 是什、我）、聲母衝突（gck6 是 g合）、反序打的英文字（url␣ 不是「交」）、不是國語音節的注音（npm␣ 不是 ㄙㄩㄣ）改成英文或中文；注音文（a␣ 2␣ 1␣ 緊接在中文或注音後面是 ㄇ ㄉ ㄅ，句首、空白或標點後要連著兩個以上，ㄇ 後面接著打英文字時改回 a；用 ↓ 選回原按鍵再送出，同一個前字以後不換，記在混打修正記憶，再選回注音就忘掉；單一個注音鍵的 ↓ 清單照官方 #208）；數字鍵盤夾在字母中間（e KP2 e）是一個英數詞 e2e，接聲調鍵照舊是中文（B1樓）。字母開頭的反序（u2、k5）在打聲調或空白之前照字面，Enter 送出字母（u2⏎ 是 u2、買了k2⏎ 是 買了k2），打了就是中文，句首、英文後面也是（u2␣ 是「低」、k54 是「這」、i16 是「博」）；數字開頭（70u4 是 70億）與官方表上的英數詞（ps4、x86）照官方，官方選回原按鍵學到的英數詞（#249）交給混打修正記憶，不再一律照字面。選過的字、從 ↓ 明確選的不改。const、sql 這類程式詞加到「我的詞彙」就照字面
    - 組字中的標示（`core/session/mark_span.go`）：Shift＋Home／End 一次標到頭尾（同一直按 Shift＋←／→，最多 32 字）；標示中按 ↓ 同從開頭按 ↓ 再按 Tab 換到那個長度，中間有空白或符號也能換整段；整段英文按 Enter 加成「我的詞彙」的英文詞
    - 候選開著時 Shift＋Tab 是 Tab 的反方向（操作規則 candidate_backtab）：換到較短的詞長，最短再按回到最長；待定的按鍵同 Tab 先照畫面定案，不會換到切開注音的長度
    - 混打修正記憶（操作規則 mixed_memory，`core/session/mixed_memory.go`、`core/personal/mixed_memory.go`）：用 ↓ 把一段按鍵換成別的解釋（mp3 出「允」選回 mp3、「我ㄇ」選回 a、$20␣ 換成「$單」）再按 Enter，記住「按鍵＋前一個字＋後一個音 → 每段當英數、中文或注音」，存在 user.json 的 `MixedMemory`。照天數淘汰，同選過的注音文：一次確認約 4 個月沒用到就不照它換，照記憶換到或留著時暫停衰減（最多 1024 筆，滿了先丟最久沒用到的）。照記憶換之前看中文那一側合不合理（有神經模型時照前文評分，沒有時用語言模型；每個字平均 log10 機率以 -4 為界，同一個位置每多確認一次、附近有英文都往不是中文那邊調）：前一個字、後一個音都對上的紀錄照它自己的確認次數（學過「我的i7」，「好的喔」不換），只有按鍵對上時每筆紀錄都要換成同樣子，照確認一次的門檻；條件不再成立時換回（mp3 接著打出「允許」）。照字面留著的段落（官方表上的 ps4）用 ↓ 選中文也記，同一個位置再打時打完聲調就換到畫面上（很ps4 → 很嫩），Enter 送出的就是畫面上的樣子。照記憶換的、這次用 ↓ 換過解釋的段落按 ↓ 直接開整段的候選（mp3／允／狁…），用 ↓ 改回會抵銷那筆記憶；候選窗反白畫面上的樣子時按 Shift＋Delete 刪掉記憶。中文選哪個字不記（官方的更正會記），私人欄位與暫停時照記憶換但不學
    - 打錯又補對的注音鍵（操作規則 mixed_overwrite，`core/session/mixed_overwrite.go`）：同一格（聲母、介音或韻母）按了兩次時照微軟新注音後按的蓋掉先按的（5jiy6 是「昨」、gjn␣ 是「蘇」），由神經模型照前文決定換不換：前面多一個聲母（gnj4、nh4）時看那個字母讓後面的字更合理還是更不合理（車子的g速 換成「速」、我講過n次 留著），其他看蓋完的中文字比畫面上照字面的樣子合理多少（ios7、json␣ 照字面很順，留著）；門檻偏嚴，寧可留著讓 ↓ 選也不把正常打的縮寫換成錯字（這個js要 不換成「尿」）。沒有神經模型、私人、暫停或沒有前文時照舊；英文字、我的詞彙、前面緊接英數的不換。↓ 清單不看神經模型也列蓋完的字，選了再送出就由混打修正記憶記住
  - 設定畫面（Phase 5-2）：
    - 「中英切換」頁在中英混打下面：↓／Tab 跨過英數換整段（mixed-resplit，開）、自動刪除冗餘注音（mixed-repeats，開）、反序打的英文字（mixed-reversed-english，開）、注音文（mixed-zhuyin-text，關）、組不成字的按鍵照字面（mixed-literal，開）、打錯又補對的注音鍵（mixed-overwrite，關）；英文模式自動補字下面是英文補全排序（english-ranking：常用詞優先／字母序）。「鍵盤與符號」頁的 Ctrl＋Alt＋, 開符號表（symbol-board，開）。混打的開關跟著中英混打停用（倚天、許氏、不打聲調時同中英混打的原因）。↓／Tab 跨過英數只管手動換，關掉時其他開關與混打修正記憶照樣作用（組字區照樣記原按鍵）
    - 不給開關、一律開著：組字中的標示、候選窗的 Shift＋Tab、英文字典詞確認 3 次就學會、混打修正記憶（中英混打開著就有）、聲母衝突（gck6 是 g合）
    - 標點操作多第 5 個選項「微軟半形參考」（操作規則 symbols＝microsoft，punctuation 照靈犀）：沒有反引號前綴，標點照微軟出半形；「Ctrl＋標點打全形」選它時多 Ctrl＋[ ] 打「」，英文模式也適用。可以跟不打聲調並用，這時組字中的 \ 出半形 \（打成「西\你」），頓號改用 Ctrl＋'
    - Shift＋空白併進官方的「切換全半形」：中英混打作用時用整合風格的全形（混打照常判斷，判成英數的段落轉全形），沒有時照官方。所以拿掉官方「Shift＋空白切換全半形時關閉中英混打」那條衝突，改了兩個官方測試（`profiles_test.go` 的 TestCustomRulesCannotBypassCompatibility 改用 Tab 調整斷詞、TestStyleRulesDisableOnlyConflictingFeatures 的 fullwidth 不再停用混打）；另外三個列出全部設定項的官方測試（TestItemsListEveryKeyWithCurrentValue、TestETenDisablesMixedAndEmojiItems、TestHsuDisablesOnlyMixedInput）把新加的項目算進去或略過
    - 「我的詞彙」多一頁「混打記憶」：按鍵｜前一個字｜後一個音｜換成｜確認次數，可以勾選多筆刪除；下面列自動學會的英文字，可以移除或不再學（引擎指令 mixed、mixed-remove，`core/engine/mixed_memory.go`）
  - 整合風格跑官方測試：`go test -tags blendall ./core/session` 讓每個 session 都套用整合風格；預期不同的測試與原因列在 `core/session/testdata/blend_expected_diffs.txt`，`py -3.12 scripts/blend_official_diff.py` 核對其他官方測試都通過、列出的測試關掉對應的規則後也通過
  - 跟混打的相容：官方的規則照官方逐條判斷（`internal/settings` 的 `styleConflicts`，拿掉了 Shift＋空白那條）；fork 加的規則欄位在 `inputstyle.Rules` 標 `fork:"blend"`，風格只存 symbols，其他由全域開關在引擎啟動時填上（`Settings.EngineRules`）。非標準排列的限制照官方：規則不是預設值的風格（含整合風格）只在標準排列可選。

安裝與更新說明見 `hoshi/windows/`。
