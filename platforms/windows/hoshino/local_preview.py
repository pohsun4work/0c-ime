#!/usr/bin/env python3
"""在本機打包目前所在的 checkout，開 PR 前先裝起來實測，不必等 fork-release 發版。

套件是未簽章的 internal-preview（scripts/package_windows.py --preview）。版本號是 fork 目前的版號再加一段
（0.2.7-hoshino.4.1、.4.2…），排在下一個正式版之前，正式版發布後自動更新照常蓋過去。
字典取官方同版本的原始碼 tarball；neural/ 沿用已安裝版本的那份（prepare_neural 要在 Linux 編 bytecode）。

打包完，套件解在 build\\local-preview\\bundle\\，旁邊的 install.ps1（local_install.ps1 的複本）負責安裝與
重建整合風格，會跳 UAC。--install 打包完直接執行它。

  py -3.12 platforms\\windows\\hoshino\\local_preview.py             # 在要測的 checkout 根目錄執行
  powershell -ExecutionPolicy Bypass -File build\\local-preview\\install.ps1

回到上一版：在系統管理員 PowerShell 執行 & "$env:LOCALAPPDATA\\Zuin\\bin\\0c-update.exe" rollback
"""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request
import zipfile

ROOT = Path(subprocess.check_output(["git", "rev-parse", "--show-toplevel"], text=True).strip())
sys.path.insert(0, str(ROOT / "scripts"))
import fetch_assets  # noqa: E402
from install_package import digest  # noqa: E402
from source_release import extract_dictionary  # noqa: E402

ZUIN = Path(os.environ["LOCALAPPDATA"]) / "Zuin"
OUT = ROOT / "build/local-preview"
PUBLISHER = "Hoshino 0c-ime fork"  # 同 fork-release.yml
SHELL_PATHS = ("platforms/windows/tsf", "platforms/windows/ui", "platforms/windows/common",
               "platforms/windows/Cargo.toml", "platforms/windows/Cargo.lock")  # 同 fork-release.yml 判斷 shell 更新


def fetch(url, target, expected):
    """取代 fetch_assets.fetch：它在暫存檔還開著時改名，Windows 上會 WinError 32。"""
    if target.exists() and digest(target) == expected:
        return target
    print(f"下載 {url}", flush=True)
    pending = target.with_name(".download-" + target.name)
    with urllib.request.urlopen(url, timeout=120) as response, pending.open("wb") as stream:
        shutil.copyfileobj(response, stream, 1 << 20)
    if digest(pending) != expected:
        pending.unlink()
        raise ValueError(f"SHA-256 不符：{target.name}")
    os.replace(pending, target)
    return target


def ensure_data(tag):
    """官方同版本原始碼 tarball 裡的字典，依這個 checkout 的字典 lock 核對。
    不用 fetch_assets.install：整包驗證會比對執行權限位元，只能在 POSIX 跑。"""
    target = OUT / "data" / tag
    if not target.is_dir():
        fetch_assets.fetch = fetch
        tarball, _ = fetch_assets.download(tag, ROOT / "build/cache/releases", models=False)
        extract_dictionary(tarball, target, ROOT)
    return target


def ensure_neural(tag, receipt, data):
    """依安裝收據逐檔複製已安裝版本的 neural/ 並核對雜湊，執行期多出來的檔案不會帶進套件。"""
    target = OUT / "neural" / tag
    if target.is_dir():
        return target
    if receipt is None:
        raise ValueError("這台沒有安裝靈犀注音，取不到 neural/")
    source = ZUIN / receipt["directory"]
    select = json.loads((source / "neural/select/manifest.json").read_text(encoding="utf-8"))
    if select.get("dictionary_sha256") != digest(data / "manifest.json"):
        raise ValueError(f"已安裝的 {receipt['version']} 跟這個 checkout 的字典不配對；先裝 {tag} 的 fork 正式版")
    stage = target.with_name(target.name + ".partial")
    shutil.rmtree(stage, ignore_errors=True)
    for name, sha in receipt["files"].items():
        if not name.startswith("neural/"):
            continue
        dest = stage / name.removeprefix("neural/")
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source / name, dest)
        if digest(dest) != sha:
            raise ValueError(f"已安裝的檔案跟安裝收據不符：{name}")
    stage.rename(target)
    return target


def next_version(base, installed):
    """base 加一段序號，大於留著的包，也大於已安裝的本機版（rollback 之後也不會撞號）。"""
    used = [int(m.group(1)) for p in (OUT / "release").glob("*.zip")
            if (m := re.fullmatch(rf"0c-{re.escape(base)}\.(\d+)-windows-amd64-preview\.zip", p.name))]
    if m := re.fullmatch(rf"{re.escape(base)}\.(\d+)", installed or ""):
        used.append(int(m.group(1)))
    return f"{base}.{max(used, default=0) + 1}"


def shell_changed(since):
    """跟 fork-release 一樣看 TSF DLL、候選窗有沒有變，另外算上還沒提交的改動。"""
    changed = subprocess.check_output(["git", "diff", "--name-only", since, "--", *SHELL_PATHS], cwd=ROOT, text=True)
    untracked = subprocess.check_output(["git", "ls-files", "--others", "--exclude-standard", "--", *SHELL_PATHS],
                                        cwd=ROOT, text=True)
    return bool((changed + untracked).strip())


def stage_install(archive, shell):
    """套件解到 bundle\\（套件目錄不能有多的檔案），安裝腳本放在它旁邊。"""
    bundle, partial = OUT / "bundle", OUT / "bundle.partial"
    for directory in (bundle, partial):
        shutil.rmtree(directory, ignore_errors=True)
    with zipfile.ZipFile(archive) as z:
        z.extractall(partial)
    (partial / archive.stem).rename(bundle)
    partial.rmdir()
    template = (Path(__file__).parent / "local_install.ps1").read_bytes()  # 有 BOM：PowerShell 5.1 才讀得懂中文
    script = template.replace(b"$shellChanged = $null", b"$shellChanged = $" + (b"true" if shell else b"false"), 1)
    (OUT / "install.ps1").write_bytes(script)
    return OUT / "install.ps1"


def install(script):
    """以系統管理員執行安裝腳本（跳 UAC），輸出經 cmd 寫到記錄檔再印出來。"""
    log = OUT / "install.log"
    log.unlink(missing_ok=True)
    # 參數經環境變數交給 PowerShell，避開命令列的引號跳脫；UAC 取消時 Start-Process 擲錯，Stop 讓它非零結束。
    command = f'/c "powershell -NoProfile -ExecutionPolicy Bypass -File "{script}" > "{log}" 2>&1"'
    elevate = ("$ErrorActionPreference = 'Stop'; $p = Start-Process cmd.exe -ArgumentList $env:ZC_INSTALL"
               " -Verb RunAs -Wait -PassThru -WindowStyle Hidden; exit $p.ExitCode")
    run = subprocess.run(["powershell", "-NoProfile", "-Command", elevate], env=dict(os.environ, ZC_INSTALL=command))
    if log.exists():
        print(log.read_text(encoding="utf-8", errors="replace"))
    if run.returncode != 0:
        raise ValueError("安裝沒有完成" + ("" if log.exists() else "（UAC 取消了？）"))


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--version", help="指定版本號（預設自動遞增）")
    parser.add_argument("--install", action="store_true", help="打包完直接執行安裝腳本（會跳 UAC）")
    args = parser.parse_args()
    os.environ["PYTHONUTF8"] = "1"  # 打包腳本讀寫的中文檔案都是 UTF-8
    state = json.loads((ROOT / ".github/fork-state.json").read_text(encoding="utf-8"))
    tag = state["upstream"]
    receipt_path = ZUIN / "install-receipt.json"
    receipt = json.loads(receipt_path.read_text(encoding="utf-8")) if receipt_path.exists() else None
    version = args.version or next_version(f"{tag[1:]}-hoshino.{state['build']}", receipt and receipt["version"])
    started = time.monotonic()
    try:
        data = ensure_data(tag)
        neural = ensure_neural(tag, receipt, data)
        settings = OUT / "settings"
        subprocess.run([sys.executable, "scripts/build_settings_windows.py", "--version", version, "--out", str(settings)],
                       cwd=ROOT, check=True)
        subprocess.run([sys.executable, "scripts/package_windows.py", "--preview", "--data", str(data),
                        "--neural", str(neural), "--settings", str(settings / "windows-amd64"),
                        "--release-docs", "platforms/windows/hoshino", "--version", version,
                        "--out", str(OUT / "release"), "--publisher", PUBLISHER], cwd=ROOT, check=True)
        archive = OUT / "release" / f"0c-{version}-windows-amd64-preview.zip"
        # 一份約 350 MB，只留最新的；序號照樣從它和已安裝的版本往上數。
        for old in (OUT / "release").glob("0c-*-windows-amd64-preview.zip*"):
            if not old.name.startswith(archive.name):
                old.unlink()
        shell = shell_changed(state["shell_commit"])
        script = stage_install(archive, shell)
        print(f"已打包 {version}（{time.monotonic() - started:.0f} 秒）：{archive}", flush=True)
        print("這版改到 TSF DLL 或候選窗，裝完要登出再登入。" if shell else "這版只改引擎，裝完不用登出。")
        if args.install:
            install(script)
        else:
            print(f"安裝：powershell -ExecutionPolicy Bypass -File {script}")
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"local preview：{error}\n")


if __name__ == "__main__":
    main()
