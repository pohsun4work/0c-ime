# hoshino 版（0c-biggo/0c-ime 的修改版）

這個 repo 是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的 fork，依 AGPL-3.0-only 發布修改後的版本。原作者的著作權與授權條款不變，見 `LICENSE` 與 `legal/`。

## 跟官方的差異

- **更新來源**：更新器信任的是 fork 自己的發行金鑰（`internal/update/trust_keys.go`），索引改從 fork 的 GitHub Releases 下載（`cmd/0c-update/auto.go` 的 `DefaultFeed`）。
- **程式碼簽章**：Windows 套件用 fork 的自簽憑證簽章（`platforms/windows/hoshino/codesign.cer`），發行者是「Hoshino 0c-ime fork」。打包腳本加了 `--sign-pfx` 和 `--publisher` 兩個選項。
- **版本號**：官方版號加上 `-hoshino.N`。
- **發布流程**：`.github/workflows/fork-release.yml` 自動合併官方新版、測試、打包、發布。
- **輸入行為**：新增「整合風格」的配方（`blend`，`core/inputstyle`），以靈犀注音為底，補上華碩注音與微軟新注音的長處。它不是內建風格，用 `0c-update profile copy blend --name 整合風格` 複製成自訂風格，設定畫面上的設定都能調，整合風格加的英文排序與學習規則不在設定畫面。原本的各個模式行為不變。
  - 已完成：
    - 英文補全與拼字修正依 wordfreq 語料詞頻排序（`internal/englishpack/wordfreq`）
    - 英文字典裡的詞確認 3 次就學會
    - 預設開啟中英混打與英文模式自動補字；= 不開 Emoji 選單
  - 之後：
    - 符號規則與 Shift＋空白全半形參考微軟新注音
    - 符號表參考華碩注音
    - 中英混打改用上下文判斷
  - 跟混打的相容：官方的規則照官方逐條判斷（`internal/settings` 的 `styleConflicts`）；fork 新加的規則欄位是為混打設計的，在 `inputstyle.Rules` 標 `fork:"blend"`，不在官方的衝突表上。非標準排列的限制也不算這些欄位（`internal/settings/mixed_compat.go` 的 `officialRulesDefault`）。

安裝與更新說明見 `platforms/windows/hoshino/`。
