# wordfreq 英文詞頻

整合風格（`blend`）的英文補全與拼字修正依語料詞頻排序，詞頻取自 [wordfreq](https://github.com/rspeer/wordfreq) **3.1.1**（Robyn Speer）的英文 Zipf 詞頻。資料依 CC BY-SA 4.0 散布，出處與 SUBTLEX 作者見 [data/sources/licenses/wordfreq.txt](../../../data/sources/licenses/wordfreq.txt)；來源登錄在 [data/sources/ledger.tsv](../../../data/sources/ledger.tsv) 的 `wordfreq`，第三方元件聲明由 `scripts/collect_notices.py` 附上。

- `wordfreq.tsv`：`詞<TAB>Zipf`，只收 `data/english/esdb.tsv` 與 `technical.tsv` 裡 wordfreq 有詞頻的詞（82,015 詞中的 70,296 詞），檔頭記著產生時兩份詞表的 sha256。沒收的詞當作 0。
- 用 `go:embed` 編進 `0c-server`，不改英文資料包：打包時沿用官方發布的英文子包，選用整合風格時引擎載入後換成依詞頻排序的同一份詞表（`englishpack.Frequency`）。判斷像不像常用詞拼錯（`NearCommon`）仍看原本的先驗。
- 排序：詞頻高的先，詞頻一樣時短詞先，再照字母序。其他風格照舊用 ESDB 的 size 先驗。

## 重產

`data/english` 改版（官方更新詞表）或換 wordfreq 版本時重產；沒重產時，新加的詞詞頻當 0、排在最後，不會出錯。

```sh
py -3.12 -m venv build/wordfreq-venv
build/wordfreq-venv/Scripts/python -m pip install wordfreq==3.1.1
build/wordfreq-venv/Scripts/python -I scripts/export_wordfreq.py
```

換 wordfreq 版本時一併更新 `scripts/export_wordfreq.py` 的版本、`data/sources/licenses/wordfreq.txt`、這份說明與 `ledger.tsv`。
