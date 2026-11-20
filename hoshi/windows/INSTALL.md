# 靈犀注音 hoshi 版：Windows 安裝

這是 [0c-biggo/0c-ime](https://github.com/0c-biggo/0c-ime) 的修改版，由 [pohsun4work/0c-ime](https://github.com/pohsun4work/0c-ime) 發布，版本號是官方版號加上 `-hoshi.N`。跟官方版的差異見 repo 裡的 `hoshi/README.md`。

## 第一次安裝（每台電腦做一次）

官方版只信任官方的更新來源，所以第一次要手動安裝，之後這台電腦就從 fork 自動更新。

程式沒有 Authenticode 簽章，系統管理員確認的視窗會顯示「發行者：未知」。更新的安全由更新索引的簽名把關：更新器只接受 fork 發行金鑰簽署的索引，並逐檔核對雜湊。

1. 到 [Releases](https://github.com/pohsun4work/0c-ime/releases/latest) 下載 `0c-<版本>-windows-amd64.zip` 與同名的 `.sha256`，核對雜湊之後解壓縮。
2. 以**系統管理員身分**開啟 PowerShell，切換到解壓縮的目錄。
3. 打完正在輸入的文字，再安裝。hoshi 版的版本號比官方同版號「小」（semver 的後綴版），從官方版換過來時要加 `-allow-downgrade`：

   ```powershell
   .\0c-update.exe verify -bundle .
   .\0c-update.exe install -bundle . -channel public-beta -allow-downgrade
   ```

4. 不必登出，用 Win＋空白切換到「靈犀注音」。已經開著的應用程式要重新開啟，才會載入新版。

## 之後的更新

自動更新保持開啟，來源是 fork 的最新 release，說明見 `UPDATING.md`。

## 回復與移除

- 回到上一版：`.\0c-update.exe rollback`。剛從官方版換過來時，上一版就是官方版；退回去之後這台電腦照樣只查 fork 的更新，收不到官方的新版。要用回官方版請照「改回官方版」做。
- 移除：從「設定 → 應用程式 → 已安裝的應用程式」解除安裝「靈犀注音（0c）」。個人詞庫會保留，記下的 fork 更新來源會清掉。沒有用過官方安裝精靈的電腦沒有這一項，改以系統管理員身分執行 `& "$env:LOCALAPPDATA\Zuin\bin\0c-update.exe" uninstall`。
- 改回官方版：先解除安裝，再安裝官方的安裝精靈。官方版不接受 fork 的更新，fork 版也不接受官方的。
