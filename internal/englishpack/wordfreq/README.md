# wordfreq 英文詞頻

英文補全與拼字修正照語料詞頻排序，詞頻取自 [wordfreq](https://github.com/rspeer/wordfreq) **3.1.1**（Robyn Speer）的英文 Zipf 詞頻。資料依 CC BY-SA 4.0 散布，出處見 [data/sources/licenses/wordfreq.txt](../../../data/sources/licenses/wordfreq.txt)；來源登錄在 [data/sources/ledger.tsv](../../../data/sources/ledger.tsv) 的 `wordfreq`，第三方聲明由 `scripts/collect_notices.py` 附上。

- `wordfreq.tsv`：`詞<TAB>Zipf`，只收 `data/english/esdb.tsv` 與 `technical.tsv` 裡 wordfreq 有詞頻的詞，檔頭記著產生時兩份詞表的 sha256。沒收的詞當作 0。
- 用 `go:embed` 編進 `0c-server`，不改英文資料包：引擎載入官方的英文子包後換成照詞頻排序的同一份詞表（`englishpack.Frequency`，`datadir.Open` 呼叫）。判斷像不像常用詞拼錯（`NearCommon`）仍看原本的先驗。
- 排序：詞頻高的先，一樣時短詞先，再照字母序。

## 重產

官方更新 `data/english` 詞表（檔頭的 sha256 對不上）或換 wordfreq 版本時重產；沒重產時新加的詞詞頻當 0、排在最後，不會出錯。

```sh
py -3.12 -m venv build/wordfreq-venv
build/wordfreq-venv/Scripts/python -m pip install wordfreq==3.1.1
build/wordfreq-venv/Scripts/python -I scripts/export_wordfreq.py
```

換 wordfreq 版本時一併更新 `scripts/export_wordfreq.py` 的版本、`data/sources/licenses/wordfreq.txt`、這份說明與 `ledger.tsv`。
