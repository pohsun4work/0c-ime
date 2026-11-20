# hoshi 版（0c-biggo/0c-ime 的修改版）

這個 repo 是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的 fork，依 AGPL-3.0-only 發布修改後的版本。原作者的著作權與授權條款不變，見 `LICENSE` 與 `legal/`。只發布 Windows 版。

## 跟官方的差異

- **更新來源**：更新器只信任 fork 的發行金鑰（`internal/update/trust_keys.go`），索引從 fork 的 GitHub Releases 下載（`cmd/0c-update/auto.go` 的 `DefaultFeed`）。
- **不會被換回官方版**：fork 的更新器成為排程用的穩定副本時（`cmd/0c-update/fork_updates.go`）：
  - 在 `update/state.json` 記下 fork 的索引網址。官方的更新器被叫起來也會先讀這個網址，用官方金鑰驗不過，什麼都不裝。
  - 備用的上一版 `bin\0c-update.prev.exe` 是官方的就換成 fork 的。官方的啟動器在新版連續異常、或查詢更新連續失敗超過 72 小時而上一版查得到時，會改跑上一版。
  - 解除安裝時把記下的網址改回內建，之後裝回官方版照樣收得到官方更新。`rollback` 不清：剛換過來就退回官方版的話，那台電腦仍然只查 fork 的更新。
- **程式碼簽章**：不做 Authenticode 簽章，安裝時 UAC 顯示「發行者：未知」。更新的安全靠索引的 ed25519 簽名與逐檔雜湊。版本資訊的公司名是「Hoshi 0c-ime fork」，著作權照舊是原作者。
- **版本號**：官方版號加 `-hoshi.N`（`0.2.13-hoshi.1`）。semver 的後綴版排在同號正式版前面，所以從官方版第一次換過來要允許退版（安裝說明與本機安裝腳本都已處理）。

## 開發與發版

- 每個 Phase 一條分支，PR 到這個 repo 的 main。
- 開 PR 前在本機打未簽章的包、裝起來實測：

  ```powershell
  py -3.12 hoshi\windows\local_preview.py --allow-downgrade   # 從官方版第一次換過來時才要 --allow-downgrade
  powershell -ExecutionPolicy Bypass -File build\local-preview\install.ps1   # -Rollback 退回上一版
  ```

  本機版號是最新 release 再加一段（還沒發過時 `0.2.13-hoshi.0.1`），排在下一個 release 之前。
- 發版：PR 合併後，手動執行 Actions 的「Hoshi release」（`.github/workflows/hoshi-release.yml`）。它測試、打包、發布 `v<官方版號>-hoshi.N`，並簽好更新索引。索引有效 14 天，同一個 workflow 每天排程續簽（剩不到 7 天才簽）。
- 官方出新版：開 sync 分支合併官方的 tag，把 `hoshi/fork-state.json` 的 `upstream` 改成新的 tag，照樣本機實測、PR、發版。

安裝與更新說明見 `hoshi/windows/`。
