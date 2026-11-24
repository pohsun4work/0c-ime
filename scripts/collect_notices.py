#!/usr/bin/env python3
"""Collect third-party license texts for what a bundle actually ships.

Sources are the shipped components themselves: the Go toolchain and modules linked into the
Windows binaries, the bundled CPython and its packages, and the Rust crates of the Windows shell.
The dictionary's source attributions stay in data/NOTICE.txt.
"""

import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
LICENSE_NAMES = ("LICENSE", "LICENCE", "COPYING", "NOTICE")


def license_files(directory):
    found = []
    for path in sorted(Path(directory).iterdir()):
        if path.is_file() and path.name.upper().startswith(LICENSE_NAMES):
            found.append(path)
    return found


def section(title, texts):
    body = "\n\n".join(t.strip() for t in texts if t.strip())
    if not body:
        raise ValueError(f"找不到授權文字：{title}")
    return f"{'=' * 78}\n{title}\n{'=' * 78}\n\n{body}\n"


def read(path):
    return Path(path).read_text(encoding="utf-8", errors="replace")


def go_sections(go, packages):
    env = dict(os.environ, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0")
    goroot = subprocess.check_output([go, "env", "GOROOT"], text=True, encoding="utf-8", env=env).strip()
    version = subprocess.check_output([go, "env", "GOVERSION"], text=True, encoding="utf-8", env=env).strip()
    license = Path(goroot) / "LICENSE"
    if not license.is_file():
        raise ValueError(f"找不到 Go 的 LICENSE（GOROOT {goroot}）；發版用官方的 Go")
    out = [section(f"Go 工具鏈與標準函式庫（{version}）", [read(license)])]
    listing = subprocess.check_output(
        [go, "list", "-deps", "-f", "{{with .Module}}{{if not .Main}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}{{end}}", *packages],
        text=True, encoding="utf-8", env=env, cwd=ROOT)
    for line in sorted(set(filter(None, listing.splitlines()))):
        path, version, directory = line.split("\t")
        out.append(section(f"Go 模組 {path} {version}", [read(p) for p in license_files(directory)]))
        if path == "github.com/clipperhouse/uax29/v2":
            out.append(section("Unicode 17 grapheme properties（uax29 產生的字元分段資料）", [read(ROOT / "legal/UNICODE-NOTICE.txt")]))
    # 「輸出簡體字」的 OpenCC 字典編在 0c-server 裡（internal/simplified，#118），不是 Go 模組，另列。
    out.append(section("OpenCC ver.1.4.2 字典（tw2s：TSCharacters、TSPhrases、TWVariants、TWVariantsRevPhrases、"
                       "CJK_Compatibility_Ideographs，https://github.com/BYVoid/OpenCC）",
                       [read(ROOT / "internal/simplified/opencc/LICENSE")]))
    # 英文補全排序用的詞頻也編在 0c-server 裡（internal/englishpack/wordfreq）。
    out.append(section("wordfreq 3.1.1 英文詞頻（https://github.com/rspeer/wordfreq）",
                       [read(ROOT / "data/sources/licenses/wordfreq.txt")]))
    return out


def python_sections(neural):
    python = Path(neural) / "python"
    site = python / "Lib/site-packages"
    out = [section("CPython 3.14", [read(python / "LICENSE.txt")])]
    crt = [p.name for p in sorted(python.glob("msvcp140*.dll"))]
    if crt:
        # prepare_neural.py 隨 onnxruntime 附上的 C++ 執行階段（#80）。
        out.append(section("Microsoft Visual C++ 執行階段（" + "、".join(crt) + "）", [
            "Microsoft Visual Studio 2022 可轉散發程式碼，未經修改隨程式散布；"
            "見 https://learn.microsoft.com/visualstudio/releases/2022/redistribution#visual-c-runtime-files"]))
    for info in sorted(site.glob("*.dist-info")):
        name = info.name.removesuffix(".dist-info").replace("-", " ", 1)
        texts = [read(p) for p in sorted((info / "licenses").rglob("*")) if p.is_file()] if (info / "licenses").is_dir() else []
        package = site / info.name.split("-")[0]
        texts += [read(p) for p in sorted(package.glob("*")) if p.is_file() and
                  (p.name.upper().startswith(LICENSE_NAMES) or p.name == "ThirdPartyNotices.txt")]
        out.append(section(f"Python 套件 {name}", texts))
    return out


def rust_sections(cargo, workspace, target="x86_64-pc-windows-msvc"):
    meta = json.loads(subprocess.check_output([cargo, "metadata", "--format-version", "1", "--locked",
                                               "--filter-platform", target], text=True, encoding="utf-8", cwd=workspace))
    used = {node["id"] for node in meta["resolve"]["nodes"]}
    out = []
    for package in sorted(meta["packages"], key=lambda p: (p["name"], p["version"])):
        if package["id"] not in used or package.get("source") is None:
            continue  # workspace crates are ours
        directory = Path(package["manifest_path"]).parent
        texts = [f"授權：{package.get('license') or '見下列檔案'}"] + [read(p) for p in license_files(directory)]
        out.append(section(f"Rust crate {package['name']} {package['version']}", texts))
    return out


def collect(neural=None, go="go", cargo="cargo"):
    """Return the notices text for the Windows bundle; raises when an expected license text is missing."""
    header = ("靈犀注音（0c）第三方元件聲明\n\n"
              "自有程式依套件內 LICENSE 授權，授權範圍見 legal/README.md 與 TERMS.md。以下是隨套件出貨的第三方元件及其授權條款；"
              "字典資料的來源與授權另見 data/NOTICE.txt。\n\n")
    parts = go_sections(go, ["./cmd/0c-server", "./cmd/0c-update"])
    parts += rust_sections(cargo, ROOT / "platforms/windows")
    if neural:
        parts += python_sections(neural)
    return header + "\n".join(parts)


if __name__ == "__main__":
    # 只輸出某個 Rust 工作區的授權文字（build_settings_windows.py 用）：collect_notices.py rust --workspace DIR --target TRIPLE
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("what", choices=["rust"])
    parser.add_argument("--workspace", required=True, type=Path)
    parser.add_argument("--target", required=True)
    parser.add_argument("--cargo", default="cargo")
    options = parser.parse_args()
    print("\n".join(rust_sections(options.cargo, options.workspace, options.target)))
