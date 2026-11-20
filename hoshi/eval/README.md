# 按鍵層級評測

把語料的按鍵逐鍵送進一個獨立的 `0c-server`（官方或 fork 的建置），用真的神經模型，記下每一鍵之後畫面上的字與引擎耗時，再比較兩次執行。官方的 `cmd/0c-eval` 送的是注音符號、不經過混打，也不開神經模型，量不到 fork 要改的東西。

- 工具：`hoshi/cmd/hoshi-eval`（`keys` 文字轉按鍵、`run` 重播、`report` 報告）
- 語料：`corpus/*.tsv`
- 設定：`profiles/*.json`（語料檔頭的 `# profile:` 選用哪一份）
- 基準報告：`reports/official-0.2.13.md`（官方 0.2.13 單獨跑的報告：`report -new official.json -out …`，不帶 `-base`）

## 怎麼跑

```powershell
go build -o build/hoshi-eval/hoshi-eval.exe ./hoshi/cmd/hoshi-eval

# 資料與神經模型用已安裝的官方版本（data 旁邊要有配對的 neural\）
$v = "$env:LOCALAPPDATA\Zuin\versions\0.2.13-73fd9193c77c"

# 官方引擎：已安裝版本目錄裡的 0c-server.exe；那個目錄被更新清掉時從官方 tag 建置
git worktree add --detach build/eval-upstream v0.2.13
go -C build/eval-upstream build -o ../hoshi-eval/0c-server-v0.2.13.exe ./cmd/0c-server
git worktree remove build/eval-upstream
build\hoshi-eval\hoshi-eval.exe run -server build\hoshi-eval\0c-server-v0.2.13.exe -data "$v\data" -label "官方 0.2.13" -out build\hoshi-eval\official.json (Get-ChildItem hoshi\eval\corpus\*.tsv)

# fork：這個 checkout 建置的引擎
go build -o build/hoshi-eval/0c-server.exe ./cmd/0c-server
build\hoshi-eval\hoshi-eval.exe run -server build\hoshi-eval\0c-server.exe -data "$v\data" -label fork -out build\hoshi-eval\fork.json (Get-ChildItem hoshi\eval\corpus\*.tsv)

build\hoshi-eval\hoshi-eval.exe report -new build\hoshi-eval\fork.json -base build\hoshi-eval\official.json -out build\hoshi-eval\fork.md
```

全部語料約 2,200 句，每句預設跑 3 次，一次執行約 7 分鐘；只看輸出、不比延遲時可以 `-repeat 1`（約 2 分鐘）。

- 每個設定開一個暫存的引擎：空白的個人資料、自己的管道，不碰使用者的引擎。一律關掉學習（`personal_learning=false`）；不用 `private`，因為它會連神經選字一起關掉。
- 每一句開新的 session，照 Windows 外殼（`platforms/windows/tsf`）的規則送鍵：
  - 沒在組字時，空白、具名鍵、不在引擎 `idle_keys` 裡的鍵交給應用程式（全形時空白、大寫、數字鍵盤照外殼的規則送進引擎）；引擎回 `handled=false` 的鍵也交給應用程式。
  - Ctrl＋標點只送外殼認得、引擎也列出的鍵；組字中按了不送的 Ctrl 鍵，外殼先送出組字。
  - 交給應用程式的鍵照一般文字框處理（Enter 換行、Backspace／Delete 刪字、←→ Home End 移游標）；送出的字之後，游標前面還是那段字才讓引擎拿它當前文。
  - 打完不送出，比的是畫面上的文字（已送出＋組字區）。
- 神經選字沒就緒時中止（`-allow-statistical` 才照跑）。某一句跑到一半神經選字逾時被停掉，會叫它回來再跑那一句，還是不行才算進報告的「統計選字」；一分鐘叫不回來就中止。同一句重跑出來的畫面不一樣時算進「重跑不同」。
- 耗時是引擎回報的處理時間（含神經選字、含等核心的鎖），不含管道往返。電腦忙的時候單一鍵會突然慢上百毫秒，同一份程式跑兩次就會有十幾鍵差超過 30 ms，所以每句跑好幾次、每鍵取最快的一次再比。兩次執行的 `-repeat` 要一樣，也要在同樣的狀況下跑（不要邊跑邊編譯）。

## 語料

| 檔案 | 句數 | 內容 | 怎麼算對 |
|---|---:|---|---|
| `spec.tsv` | 55 | 規格裡寫得成按鍵的例子，備註寫官方的行為與對應的 Phase | 跟 want 一樣 |
| `spec-fullwidth.tsv` | 2 | 全形時混打（設定 `fullwidth`） | 跟 want 一樣 |
| `zh-habit1.tsv` | 200 | 純中文，全形標點（習慣一） | **跟官方逐字一樣**（`# match: baseline`） |
| `zh-habit2.tsv` | 200 | 純中文，不打標點、用空白斷句（習慣二） | **跟官方逐字一樣** |
| `mixed-habit1.tsv` | 150 | 自寫的混打句，盤古之白＋全形標點 | 跟 want 一樣 |
| `mixed-habit2.tsv` | 150 | 自寫的混打句，不空格、用空白斷句 | 跟 want 一樣 |
| `asus-redundant.tsv` | 128 | 華碩冗餘音實測 | 跟華碩一樣（參考） |
| `asus-typo.tsv` | 1,033 | 華碩打錯鍵實測 | 跟華碩一樣（參考） |
| `asus-mixed.tsv` | 255 | 華碩混打其他情況實測 | 跟華碩一樣（參考） |

格式：`id<TAB>keys<TAB>want<TAB>note`，`#` 開頭是註解與檔頭（`# profile:`、`# match:`）。

- 按鍵：一個字元一鍵；`␣` 是空白鍵；`{Enter}` `{Down}` `{Left}` `{BackTab}` `{KP2}`（數字鍵盤）`{Ctrl+,}` 是具名鍵；大寫字母與 Shift 符號就寫那個字元；`{pick:它}` 是在開著的候選裡選「它」（這一頁沒有就往後翻）。
- want 裡的 `␣` 是半形空白。

自寫的句子（`zh-*`、`mixed-*`）由 Claude 產生，按鍵用 `hoshi-eval keys` 從文字轉：中文用字典的參考標音（`internal/eval` 的 `Reader`，多音字只看斷詞，可能標錯），英數照字面，全形標點用 Ctrl＋標點，空白一個 `␣`（一聲字的音節本來就以空白結尾，盤古之白要再按一次）。官方打錯的句子都人工核對過讀音。

華碩的三份是 `record/ime-probe/asus-ime/` 的實測（只在原作者的電腦上），want 是華碩打完按 Esc 前的畫面（「捰」改成「我」）。華碩選字跟靈犀不同，規格也有刻意不照華碩的部分（英文字搶字母、數字接量詞），所以只當參考：冗餘音、打錯鍵的 Phase 會照規則另外訂正解。

## 沒有放進語料的規格例子

要先做別的步驟、目前的工具表達不了，由各 Phase 的測試涵蓋：

- 選過要記住（「3nm」後面選過「製程」、`ssl` ↓ 選回、`url` ↓ 選回）：要開學習、送出後再打一次
- 自訂詞彙：`mp3vm3` → 允許
- ↓ 清單的內容：`3nm54t/6` ↓ 要有跨英數的切法、`Ctrl+[` ↓ 的順序、英文補全 `const` ↓ 的順序
- 符號表、我的片語熱鍵、候選開著時的 `Shift+Tab`
- ↑ 輪換的 bug（保留官方）
