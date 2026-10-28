#!/usr/bin/env python3
"""重產 core/session/testdata/microsoft_symbols.tsv：整合風格（blend）的符號規則要對照的微軟新注音實測。

紀錄是在 Windows 11 的微軟新注音逐鍵實測（SendInput 送鍵、讀 IMM 的組字），一例一列：模式、前置按鍵、按鍵，
以及按完之後再按 Enter 直到沒有組字（最多兩次）時文件裡的文字與剩下的組字。只在重新實測時用，不是建置的一部分：

    py -3.12 -I scripts/export_ime_probe.py <實測紀錄 full.jsonl>
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "core/session/testdata/microsoft_symbols.tsv"


def main():
    if len(sys.argv) != 2:
        raise SystemExit(__doc__)
    lines = [
        "# 微軟新注音（Windows 11）符號實測；由 scripts/export_ime_probe.py 產生，不要手改",
        "# 編號\t模式\t前置按鍵\t按鍵\t文件裡的文字\t剩下的組字（後兩欄是 JSON 字串）",
    ]
    for raw in Path(sys.argv[1]).read_text(encoding="utf-8").splitlines():
        row = json.loads(raw)
        if "n" not in row:
            continue
        final = row["enter2"] or row["enter1"] or row["after"]
        text, left = (json.dumps(final[k], ensure_ascii=False) for k in ("text", "comp"))
        lines.append("\t".join([str(row["n"]), row["block"], row["prefix"], row["keys"], text, left]))
    OUT.write_text("\n".join(lines) + "\n", encoding="utf-8", newline="\n")
    print(f"{OUT.relative_to(ROOT).as_posix()}：{len(lines) - 2} 例")


if __name__ == "__main__":
    main()
