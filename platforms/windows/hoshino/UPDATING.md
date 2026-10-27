# 靈犀注音 hoshino 版：更新

## 更新來源

hoshino 版的更新器只接受 fork 發行金鑰簽署的索引：
- 索引網址：`https://github.com/pohsun4work/0c-ime/releases/latest/download/public-beta.json`
- 簽章：同一個網址加上 `.sig`

官方的索引與金鑰都已經換掉，所以裝了 hoshino 版之後，不會再被官方版本蓋過去。

## 新版本怎麼來

fork 的 GitHub Actions（`.github/workflows/fork-release.yml`）每 12 小時執行一次：

1. 查官方最新的 release。有新版就把它合併進 fork。
2. 抓官方同版本的字典與模型，跑完整測試，打包並簽章。
3. 發布新的 release（`<官方版號>-hoshino.N`），並更新簽署過的索引。
4. 官方沒出新版時，只續簽索引。索引有到期時間，過期的話更新器會拒收。

合併有衝突或測試失敗時，不會發布新版，而是在 fork 開一個 issue 通知。電腦上維持原本的版本，照常可以使用。

## 什麼時候需要按「允許」

- **只換引擎、字典、模型的更新**：在背景完成，不需要系統管理員權限。
- **連輸入法外殼（TSF DLL）都要換的更新**：要以同一個帳號按一次系統管理員確認，完成後要登出再登入。

## 手動操作

`0c-update.exe` 的用法跟官方相同，例如 `status`、`check`、`rollback`、`mode auto|ask|manual`。在版本目錄或解壓縮的套件目錄執行 `.\0c-update.exe help` 可以查看全部命令。
