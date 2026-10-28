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

官方新增或改了操作規則時，會另外開一個「官方規則變動待處理」issue，提醒決定新規則跟中英混打相不相容；檢查本身跑不起來時也開在同一個 issue。這種情況照常發布：還沒分類的規則照官方的做法停用混打，不會出錯。

## 整合風格有新功能時

整合風格是一份自訂風格，建好之後內容就固定了，之後版本加的新功能不會自動套進去。release 說明寫到整合風格有新功能時，每台電腦重新建一份，再刪掉舊的：

```powershell
.\0c-update.exe profile copy blend --name 整合風格 -apply
.\0c-update.exe settings -json   # 在 profiles.items 找到舊的那份整合風格的 id
.\0c-update.exe profile delete <舊的 id>
```

也可以在設定程式的「選用風格」刪掉舊的那份。

## 什麼時候需要按「允許」

- **只換引擎、字典、模型的更新**：在背景完成，不需要系統管理員權限。
- **連輸入法外殼（TSF DLL）都要換的更新**：要以同一個帳號按一次系統管理員確認，完成後要登出再登入。

## 手動操作

`0c-update.exe` 的用法跟官方相同。在 `%LOCALAPPDATA%\Zuin\versions\<目前版本>\` 執行：

- `.\0c-update.exe status -json`：目前版本與更新狀態
- `.\0c-update.exe check -json`：立刻查詢有沒有新版，不下載
- `.\0c-update.exe auto -now`：立刻下載並準備更新。切換會等電腦閒置，狀態顯示 `waiting_idle`
- `.\0c-update.exe finish`：不等閒置，現在就切換到已準備好的版本
- `.\0c-update.exe rollback`：回到上一版
- `.\0c-update.exe mode auto|ask|manual`：自動更新的模式
