#!/usr/bin/env python3
"""在本機打包目前的 checkout，開 PR 前先裝起來實測，不必等發版。

套件是未簽章的 internal-preview（scripts/package_windows.py --preview）。版號是 fork 最新 release 再加一段：
還沒發過時是 0.2.13-hoshi.0.1、.0.2…，發過 0.2.13-hoshi.1 之後是 0.2.13-hoshi.1.1…，都排在下一個 release 之前，
release 發布後自動更新照常蓋過本機版。
字典取官方同版本的原始碼 tarball；neural/ 沿用已安裝版本的那份（prepare_neural 要在 Linux 編 bytecode）。

打包完在 build\\local-preview\\ 留下 install.ps1（local_install.ps1 的複本），會跳 UAC 安裝；-Rollback 退回上一版。

  py -3.12 hoshi\\windows\\local_preview.py [--install] [--allow-downgrade]
  powershell -ExecutionPolicy Bypass -File build\\local-preview\\install.ps1 [-Rollback]
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
import zipfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import fetch_assets  # noqa: E402
from package_release import prepare_data_manifest  # noqa: E402
from source_release import digest, extract_dictionary, git  # noqa: E402

ZUIN = Path(os.environ["LOCALAPPDATA"]) / "Zuin"
OUT = ROOT / "build/local-preview"
FORK = json.loads((ROOT / "hoshi/fork-state.json").read_text(encoding="utf-8"))  # 跟 .github/workflows/hoshi-release.yml 共用


def released_build(tag):
    """fork 已發布的 <tag>-hoshi.N 裡最大的 N；還沒發過是 0。"""
    out = git(ROOT, "ls-remote", "--tags", "origin", f"refs/tags/{tag}-hoshi.*")
    pattern = re.compile(rf"refs/tags/{re.escape(tag)}-hoshi\.(\d+)$")
    return max((int(m.group(1)) for line in out.splitlines() if (m := pattern.search(line))), default=0)


def dictionary_source():
    return OUT / "data" / digest(ROOT / "data/dictionary.lock.json")[:12]


def ensure_data(tag, receipt):
    """官方同版本原始碼 tarball 裡的字典，依這個 checkout 的字典 lock 核對；快取以 lock 的雜湊區分。
    不用 fetch_assets.install：整包驗證會比對執行權限位元，只能在 POSIX 跑。

    官方安裝檔的字典檔跟 tarball 的相同，manifest 卻是非公開建置的那份（Public=false），已安裝的 neural/ 綁的是它；
    本機包沿用已安裝的 neural/，所以兩份 manifest 記的字典檔一樣時，換上已安裝版本的 manifest。"""
    source = dictionary_source()
    if not source.is_dir():
        tarball, _ = fetch_assets.download(tag, ROOT / "build/cache/releases", models=False)
        extract_dictionary(tarball, source, ROOT)
    installed = receipt and ZUIN / receipt["directory"] / "data/manifest.json"
    if not installed or not installed.exists() or digest(installed) == digest(source / "manifest.json"):
        return source
    files = lambda path: json.loads(path.read_text(encoding="utf-8")).get("files")
    if files(installed) != files(source / "manifest.json"):
        return source  # 字典檔不同：由 ensure_neural 回報不配對
    target = OUT / "data" / f"installed-{digest(installed)[:12]}"
    if not target.is_dir():
        stage = target.with_name(target.name + ".partial")
        shutil.rmtree(stage, ignore_errors=True)
        shutil.copytree(source, stage)
        shutil.copy2(installed, stage / "manifest.json")
        stage.rename(target)
    return target


def ensure_neural(tag, receipt, data):
    """依安裝收據逐檔複製已安裝版本的 neural/ 並核對雜湊，執行期多出來的檔案不會帶進套件；快取以配對的字典區分。"""
    target = OUT / "neural" / digest(data / "manifest.json")[:12]
    if target.is_dir():
        return target
    if receipt is None:
        raise ValueError("這台沒有安裝靈犀注音，取不到 neural/")
    source = ZUIN / receipt["directory"]
    try:
        prepare_data_manifest(data, source / "neural")
    except ValueError as error:
        raise ValueError(f"已安裝的 {receipt['version']} 跟這個 checkout 的字典不配對（{error}）；先裝 {tag} 的官方版或 fork 版") from None
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


def prune_cache(keep):
    """字典與 neural/ 的快取只留這次用到的；官方每升一版多約 400 MB。"""
    for parent in {path.parent for path in keep}:
        for old in parent.iterdir():
            if old not in keep:
                shutil.rmtree(old, ignore_errors=True)


def archive_name(version):
    return f"0c-{version}-windows-amd64-preview.zip"


def next_version(base, installed):
    """base 加一段序號，大於留著的包，也大於已安裝的本機版（rollback 之後也不會撞號）。"""
    built = [p.name.removeprefix("0c-").removesuffix("-windows-amd64-preview.zip")
             for p in (OUT / "release").glob(archive_name("*"))]
    used = [int(m.group(1)) for v in (*built, installed or "") if (m := re.fullmatch(rf"{re.escape(base)}\.(\d+)", v))]
    return f"{base}.{max(used, default=0) + 1}"


def semver(version):
    """semver 的先後：預覽版排在正式版前；預覽欄位數字比大小、排在文字前，前面都一樣時欄位多的較新。"""
    core, _, pre = version.partition("-")
    ids = tuple((0, int(p), "") if p.isdigit() else (1, 0, p) for p in pre.split(".")) if pre else ()
    return tuple(map(int, core.split("."))), not pre, ids


def shell_changed(receipt):
    """TSF DLL、候選窗跟已安裝那版的 commit 比有沒有變，算上還沒提交的改動；比不出來時當成有變。"""
    if receipt is None:
        return True
    release = json.loads((ZUIN / receipt["directory"] / "release.json").read_text(encoding="utf-8"))
    if release.get("dirty") or subprocess.run(["git", "cat-file", "-e", release["revision"] + "^{commit}"],
                                              cwd=ROOT, capture_output=True).returncode != 0:
        return True
    paths = FORK["shell_paths"]
    return bool(git(ROOT, "diff", "--name-only", release["revision"], "--", *paths)
                or git(ROOT, "ls-files", "--others", "--exclude-standard", "--", *paths))


def stage_install(archive, shell, allow_downgrade):
    """安裝腳本與它讀的 install.json 放在 build\\local-preview\\；用新版的 0c-update 安裝，先從套件取出來。"""
    updater = OUT / "updater/0c-update.exe"
    updater.parent.mkdir(exist_ok=True)
    with zipfile.ZipFile(archive) as z, z.open(f"{archive.stem}/0c-update.exe") as source, updater.open("wb") as target:
        shutil.copyfileobj(source, target)
    sha256 = archive.with_suffix(".zip.sha256").read_text(encoding="utf-8").split()[0]
    (OUT / "install.json").write_text(json.dumps({"archive": str(archive), "sha256": sha256, "shell_changed": shell,
                                                  "allow_downgrade": allow_downgrade}), encoding="utf-8")
    script = OUT / "install.ps1"
    shutil.copyfile(Path(__file__).parent / "local_install.ps1", script)
    return script


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--version", help="指定版本號（預設自動遞增）")
    parser.add_argument("--install", action="store_true", help="打包完直接執行安裝腳本（會跳 UAC）")
    parser.add_argument("--allow-downgrade", action="store_true",
                        help="版號比已安裝的舊時照樣裝（從官方版第一次換過來時一定要，fork 的版號排在官方同版號前面）")
    args = parser.parse_args()
    os.environ["PYTHONUTF8"] = "1"  # 打包腳本讀寫的中文檔案都是 UTF-8
    tag = FORK["upstream"]
    receipt_path = ZUIN / "install-receipt.json"
    receipt = json.loads(receipt_path.read_text(encoding="utf-8")) if receipt_path.exists() else None
    installed = receipt and receipt["version"]
    started = time.monotonic()
    try:
        base = f"{tag[1:]}-hoshi.{released_build(tag)}"
        version = args.version or next_version(base, installed)
        if installed and semver(version) < semver(installed) and not args.allow_downgrade:
            raise ValueError(f"{version} 比已安裝的 {installed} 舊，裝不上去；從官方版換過來或要退版時加 --allow-downgrade")
        data = ensure_data(tag, receipt)
        neural = ensure_neural(tag, receipt, data)
        prune_cache({dictionary_source(), data, neural})
        settings = OUT / "settings"
        # 設定程式的版本只用在「關於」頁（檔案版本資訊由打包腳本蓋上）：固定成 base，沒改設定程式時 cargo 不必重編。
        subprocess.run([sys.executable, "scripts/build_settings_windows.py", "--version", base, "--out", str(settings)],
                       cwd=ROOT, check=True)
        subprocess.run([sys.executable, "scripts/package_windows.py", "--preview", "--data", str(data),
                        "--neural", str(neural), "--settings", str(settings / "windows-amd64"),
                        "--release-docs", "hoshi/windows", "--version", version,
                        "--out", str(OUT / "release"), "--publisher", FORK["publisher"]], cwd=ROOT, check=True)
        archive = OUT / "release" / archive_name(version)
        # 一份約 350 MB，只留最新的；序號照樣從它和已安裝的版本往上數。
        for old in (OUT / "release").glob(archive_name("*") + "*"):
            if not old.name.startswith(archive.name):
                old.unlink()
        shell = shell_changed(receipt)
        script = stage_install(archive, shell, args.allow_downgrade)
        print(f"已打包 {version}（{time.monotonic() - started:.0f} 秒）：{archive}", flush=True)
        print("這版改到 TSF DLL 或候選窗，裝完已經開著的應用程式要重新開啟。" if shell else "這版只改引擎，裝完直接生效。", flush=True)
        if args.install:
            subprocess.run(["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(script)], check=True)
        else:
            print(f"安裝：powershell -ExecutionPolicy Bypass -File {script}")
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f"local preview：{error}\n")


if __name__ == "__main__":
    main()
