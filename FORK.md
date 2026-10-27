# hoshino 版（0c-biggo/0c-ime 的修改版）

這個 repo 是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的 fork，依 AGPL-3.0-only 發布修改後的版本。原作者的著作權與授權條款不變，見 `LICENSE` 與 `legal/`。

## 跟官方的差異

- **更新來源**：更新器信任的是 fork 自己的發行金鑰（`internal/update/trust_keys.go`），索引改從 fork 的 GitHub Releases 下載（`cmd/0c-update/auto.go` 的 `DefaultFeed`）。
- **程式碼簽章**：Windows 套件用 fork 的自簽憑證簽章（`platforms/windows/hoshino/codesign.cer`），發行者是「Hoshino 0c-ime fork」。打包腳本加了 `--sign-pfx` 和 `--publisher` 兩個選項。
- **版本號**：官方版號加上 `-hoshino.N`。
- **發布流程**：`.github/workflows/fork-release.yml` 自動合併官方新版、測試、打包、發布。
- **輸入行為**：之後會新增一個內建模式「整合風格」（`blend`），提供以下行為：
  - 符號規則參考微軟新注音
  - 符號表參考華碩注音
  - 改良英文補全與中英混打
  
  原本的各個模式行為不變。

安裝與更新說明見 `platforms/windows/hoshino/`。
