# 靈犀注音 hoshino 版：Windows 安裝

這是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的修改版，由 [pohsun4work/0c-ime](https://github.com/pohsun4work/0c-ime) 發布，版本號是官方版號加上 `-hoshino.N`。跟官方版的差異見 repo 根目錄的 `FORK.md`。

## 第一次安裝（每台電腦做一次）

官方版只信任官方的更新來源，所以第一次要手動安裝。之後這台電腦就會從 fork 自動更新。

1. 到 [Releases](https://github.com/pohsun4work/0c-ime/releases/latest) 下載三個檔案：
   - `0c-<版本>-windows-amd64.zip`
   - 同名的 `.sha256`
   - `codesign.cer`

   核對雜湊之後解壓縮。`codesign.cer` 留在下載目錄，**不要**放進解壓縮出來的目錄：套件目錄裡只要多一個檔案，`0c-update.exe verify` 就會拒絕。
2. 以**系統管理員身分**開啟 PowerShell，切換到解壓縮的目錄。
3. 匯入 fork 的程式碼簽章憑證。這張是自簽憑證，只用來驗證這個 fork 發布的檔案：

   ```powershell
   Import-Certificate -FilePath ..\codesign.cer -CertStoreLocation Cert:\LocalMachine\Root
   Import-Certificate -FilePath ..\codesign.cer -CertStoreLocation Cert:\LocalMachine\TrustedPublisher
   ```

4. 安裝。hoshino 版的版本號比官方同版號「小」（semver 的預發行版本），從官方版換過來時必須加 `-allow-downgrade`：

   ```powershell
   .\0c-update.exe verify -bundle .
   .\0c-update.exe install -bundle . -channel public-beta -allow-downgrade
   ```

5. 完成正在輸入的文字，**登出再登入**。
6. 建立並選用「整合風格」。在 `%LOCALAPPDATA%\Zuin\versions\<目前版本>\` 執行：

   ```powershell
   .\0c-update.exe profile copy blend --name 整合風格 -apply
   ```

   整合風格是一份自訂風格，設定畫面上的設定都可以調整（英文的詞頻排序與學習規則不在設定畫面）。官方的操作規則（例如 Tab 輪換字詞）改成非預設值時，會照官方的做法停用中英混打。整合風格有新功能時，要重新建一份，見 `UPDATING.md`。

## 之後的更新

自動更新保持開啟，來源是 fork 的最新 release，說明見 `UPDATING.md`。

## 回復與移除

- 回到上一版：`.\0c-update.exe rollback`
- 移除：從「設定 → 應用程式 → 已安裝的應用程式」解除安裝「靈犀注音（0c）」。個人詞庫會保留。
- 改回官方版：先解除安裝，再安裝官方的安裝精靈。官方版不會接受 fork 的更新，反過來也一樣。
