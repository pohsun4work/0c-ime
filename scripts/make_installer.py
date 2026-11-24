#!/usr/bin/env python3
"""Generate the one-line install scripts (install.ps1, install.cmd; issue #19).

install.ps1 pins the SHA-256 of the bootstrap 0c-update.exe, downloads it over HTTPS and runs
`0c-update install -online`, which verifies the signed feed and the full package itself. The bootstrap is the
exact 0c-update.exe inside the published package; install.cmd hands over to install.ps1.
"""
import argparse
import hashlib
from pathlib import Path
import re

TEMPLATES = Path(__file__).resolve().parent / "installer"


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def render(name, values):
    text = (TEMPLATES / (name + ".in")).read_text(encoding="utf-8")
    for key, value in values.items():
        if re.search(r"[\s'\"`$;&|<>]", value):
            raise ValueError(f"{key} 含不允許的字元：{value!r}")
        text = text.replace(f"@{key}@", value)
    missing = re.findall(r"@[A-Z0-9_]+@", text)
    if missing:
        raise ValueError(f"{name} 還有未填的欄位：{missing}")
    return text


def generate(out, base_url, feed, channel, bootstrap=None):
    """bootstrap：套件內 0c-update.exe 的 (本機檔, 公開網址)；這一版沒有 Windows 套件時為 None。回傳 {檔名: 路徑}。"""
    path, url = bootstrap or (None, "")
    values = {"FEED": feed, "CHANNEL": channel, "PS1_URL": f"{base_url}/install.ps1",
              "URL_WINDOWS_AMD64": url, "SHA_WINDOWS_AMD64": sha256(path) if path else ""}
    out.mkdir(parents=True, exist_ok=True)
    written = {}
    for name in ("install.ps1", "install.cmd"):
        text = render(name, dict(values, SELF_URL=f"{base_url}/{name}"))
        target = out / name
        target.write_text(text if name != "install.cmd" else text.replace("\n", "\r\n"), encoding="utf-8", newline="")
        written[name] = target
    return written


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--base-url", required=True, help="腳本公開位置（不含檔名）")
    p.add_argument("--feed", required=True)
    p.add_argument("--channel", default="public-beta")
    p.add_argument("--bootstrap", metavar="PATH=URL", help="套件內 0c-update.exe 的本機檔與公開網址")
    args = p.parse_args()
    bootstrap = None
    if args.bootstrap:
        path, _, url = args.bootstrap.partition("=")
        bootstrap = (Path(path), url)
    for path in generate(args.out, args.base_url.rstrip("/"), args.feed, args.channel, bootstrap).values():
        print(path)


if __name__ == "__main__":
    main()
