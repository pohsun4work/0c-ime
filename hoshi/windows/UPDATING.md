# 靈犀注音 hoshi 版：更新

## 更新來源

hoshi 版的更新器只接受 fork 發行金鑰簽署的索引：

- 索引網址：`https://github.com/pohsun4work/0c-ime/releases/latest/download/public-beta.json`
- 簽章：同一個網址加上 `.sig`

安裝 hoshi 版時，更新器會在這台電腦記下這個網址，並把備用的上一版更新器（`bin\0c-update.prev.exe`）換成 hoshi 版的。所以之後不會被官方版本蓋過去。

## 新版本怎麼來

1. 修改合併進 fork 的 main 之後，由維護者手動執行 GitHub Actions 的「Hoshi release」。
2. 它跑完整測試、打包，發布新的 release（`<官方版號>-hoshi.N`），並簽好更新索引。
3. 索引有效 14 天。同一個 workflow 每天檢查一次，剩不到 7 天時自動續簽；索引過期的話，更新器會拒收，電腦維持原本的版本。

發布或續簽失敗時，會在 fork 開一個 issue 通知。電腦上維持原本的版本，照常可以使用。

## 什麼時候需要按「允許」

- **只換引擎、字典、模型的更新**：在背景完成，不需要系統管理員權限。
- **連輸入法外殼（TSF DLL、候選窗）都要換的更新**：要以同一個帳號按一次系統管理員確認，不必登出。已經開著的應用程式要重新開啟，才會換成新版。

## 手動操作

`0c-update.exe` 的用法跟官方相同。在 `%LOCALAPPDATA%\Zuin\versions\<目前版本>\` 執行：

- `.\0c-update.exe status`：目前版本與更新狀態（記下的更新索引在 `%LOCALAPPDATA%\Zuin\update\state.json` 的 `feed`）
- `.\0c-update.exe check -json`：立刻查詢有沒有新版，不下載
- `.\0c-update.exe auto -now`：立刻下載並準備更新。切換會等電腦閒置，狀態顯示 `waiting_idle`
- `.\0c-update.exe finish`：不等閒置，現在就切換到已準備好的版本
- `.\0c-update.exe rollback`：回到上一版
- `.\0c-update.exe mode auto|ask|manual`：自動更新的模式
