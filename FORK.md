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
    - 符號照微軟新注音：中文模式的標點出半形（Shift＋, 是 <，中文逗號用 Ctrl＋,）；沒在打字時打的符號也留在組字區，混打才看得到 $20 這類上下文；` 單純輸出 `；Ctrl＋[ ] 打【】（Ctrl＋Shift＋; 9 0 照官方），英文模式的 Ctrl＋標點也打全形（`core/session/testdata/microsoft_symbols.tsv` 是對照的實測）
    - Shift＋空白切換全半形，全形時混打照常判斷，判成英數的段落轉全形（同華碩注音）
    - Ctrl＋Alt＋, 開華碩注音的符號表：鍵盤形狀，一般／數學／特殊三頁，Tab 換頁，按鍵或方向鍵加 Enter 選取
    - 混打打進組字區的字在 Enter 之前都找得回原按鍵（`core/session/mixed_span.go`）：↓ 與 Tab 換詞長可以跨過英數、原樣注音與數字鍵盤的數字，整段換成英文、注音或中文（$20␣ 換成「$單」，數字鍵盤打的 ㄍ2ㄍ 換回 e2e）；數字鍵盤的數字一律只是數字
  - 之後：
    - 中英混打改用上下文判斷
  - 跟混打的相容：官方的規則照官方逐條判斷（`internal/settings` 的 `styleConflicts`）；fork 新加的規則欄位是為混打設計的，在 `inputstyle.Rules` 標 `fork:"blend"`，不在官方的衝突表上。非標準排列的限制也不算這些欄位（`internal/settings/mixed_compat.go` 的 `officialRulesDefault`）。

安裝與更新說明見 `platforms/windows/hoshino/`。
