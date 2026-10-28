#!/usr/bin/env python3
"""重產 internal/englishpack/wordfreq/wordfreq.tsv：整合風格（blend）英文補全排序用的語料詞頻。

替英文詞表（data/english/esdb.tsv、technical.tsv）的每個詞查 wordfreq 的英文 Zipf 詞頻，
只在換 wordfreq 版本或詞表改版時用，不是建置的一部分。需要固定版本的 wordfreq：

    py -3.12 -m venv build/wordfreq-venv
    build/wordfreq-venv/Scripts/python -m pip install wordfreq==3.1.1
    build/wordfreq-venv/Scripts/python -I scripts/export_wordfreq.py

詞頻為 0（wordfreq 沒收）的詞不寫，執行期當作 0。授權與出處見同目錄的 README.md。
"""
import hashlib
from importlib import metadata
from pathlib import Path

from wordfreq import zipf_frequency

VERSION = "3.1.1"
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "internal/englishpack/wordfreq/wordfreq.tsv"


def words(path, columns):
    for line in path.read_text(encoding="utf-8").splitlines():
        if line and not line.startswith("#"):
            fields = line.split("\t")
            if len(fields) != columns:
                raise ValueError(f"{path.name} 欄位數不符：{line!r}")
            yield fields[0]


def main():
    if metadata.version("wordfreq") != VERSION:
        raise SystemExit(f"需要 wordfreq=={VERSION}")
    sources = {ROOT / "data/english/esdb.tsv": 3, ROOT / "data/english/technical.tsv": 2}
    vocabulary = sorted(set().union(*(words(p, n) for p, n in sources.items())))
    kept = [(w, z) for w in vocabulary if (z := zipf_frequency(w, "en")) > 0]
    lines = [f"# wordfreq {VERSION} 英文 Zipf 詞頻（CC BY-SA 4.0，出處見 README.md）；由 scripts/export_wordfreq.py 產生，不要手改"]
    lines += [f"# {p.relative_to(ROOT).as_posix()} sha256 {hashlib.sha256(p.read_bytes()).hexdigest()}" for p in sources]
    lines += [f"{w}\t{z:.2f}" for w, z in kept]
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    print(f"{OUT.relative_to(ROOT).as_posix()}：{len(kept)} / {len(vocabulary)} 詞有詞頻")


if __name__ == "__main__":
    main()
