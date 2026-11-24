#!/usr/bin/env python3
"""Assemble the bundled neural/ directory: portable Python, pinned wheels, workers and model packages.

The result sits next to the paired system dictionary in a release bundle; 0c-server finds it there.
Downloads are pinned by SHA-256 and cached; nothing is installed on this machine.

模型是 models/models.lock.json 記的 Hugging Face 公開版（#180）：由 scripts/hf_models.py 取得原檔（--models 指定的
目錄、快取或下載，逐檔核對 models.lock），依 --dictionary 配對；結果等於這份字典的 pairing.lock.json。
"""

import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import zipfile

import hf_models

ROOT = Path(__file__).resolve().parents[1]
# CPython 3.14.7; python.org stopped shipping Windows binaries for 3.12.
RUNTIME = {
    "version": "3.14",
    "python": ("https://www.python.org/ftp/python/3.14.7/python-3.14.7-embed-amd64.zip",
               "d297e5ff019966817ad8502465176139f2d3d840fa4ed84b13bed399a6ab1f15"),
    "wheels": [
        ("https://files.pythonhosted.org/packages/a4/73/d2c08231e4fde7e415501fd02c715d96e98599b2d8384445933944152984/"
         "numpy-2.5.3-cp314-cp314-win_amd64.whl",
         "2c25dfa72943e4336ddb6b0ee4277b47a0c85bede0807530ec68103bf58e2c10"),
        ("https://files.pythonhosted.org/packages/4d/0a/748b86000fbf9518b5dfc5bdc1b924eeaca976203740fd9d30649e568ee6/"
         "onnxruntime-1.30.0-cp314-cp314-win_amd64.whl",
         "3bdd1752a8502ac1a7ccc6e878d16db6943df574d54ed7f01058a6806ae05be4"),
    ],
    # onnxruntime 需要 MSVCP140.dll／MSVCP140_1.dll；embeddable Python 只帶 vcruntime140*.dll，numpy 的 msvcp140
    # 改過名只給自己用。沒裝 Visual C++ 可轉散發套件的電腦（例如精簡版 Windows）會載入失敗、退回統計選字（#80），
    # 系統裝的版本太舊時 onnxruntime 也可能當掉。這個 wheel 只取這兩個 Microsoft 簽章的 DLL，放在 python.exe 旁，
    # 擴充模組載入時優先於 System32 找到。14.44 的匯出涵蓋 onnxruntime 1.30.0 用到的全部符號，所需的 vcruntime140
    # 匯出也都在內附的 14.51 版裡。
    "crt": ("https://files.pythonhosted.org/packages/d2/48/7fb574a4bfa697574a7dc7105bc15ef6882125e7c77c794c5e73429de832/"
            "msvc_runtime-14.44.35112-cp314-cp314-win_amd64.whl",
            "af179a6c552070e660493765efd5856283af3fed971ddc3577203a7d3f1ee8e7",
            ("msvcp140.dll", "msvcp140_1.dll")),
}
# 編譯 bytecode 用的同版本 Linux 直譯器（bytecode 跨平台、只跟 CPython 版本有關），所以這個工具在 Linux x86-64 上執行。
COMPILER = ("https://github.com/astral-sh/python-build-standalone/releases/download/20260924/"
            "cpython-3.14.7%2B20260924-x86_64-unknown-linux-gnu-install_only_stripped.tar.gz",
            "bd0d0568ccded07bbf1c87727230dc5dd0187e706a87da23c4de78388a229b78")


def digest(path):
    h = hashlib.sha256()
    with open(path, "rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def fetch(url, sha256, cache, local=None):
    """Return a verified file: from --wheels/--python-archive overrides, the cache, or the network."""
    name = url.rsplit("/", 1)[-1].replace("%2B", "+")
    target = cache / name
    if local:
        target = Path(local)
    elif not target.exists():
        cache.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as out, urllib.request.urlopen(url) as reply:
            shutil.copyfileobj(reply, out)
        os.replace(out.name, target)
    if digest(target) != sha256:
        raise ValueError(f"下載檔雜湊不符：{name}")
    return target


def install_python(archive, dest):
    with zipfile.ZipFile(archive) as z:
        z.extractall(dest)
    site = dest / "Lib/site-packages"
    site.mkdir(parents=True)
    # The embeddable distribution fixes sys.path with its ._pth file; add the bundled packages.
    (dest / "python314._pth").write_text("python314.zip\n.\nLib\\site-packages\n", encoding="ascii")
    return site


def install_compiler(archive, dest):
    """Unpack the Linux interpreter that compiles the bytecode; the archive's top-level directory is python/."""
    with tarfile.open(archive) as tar:
        tar.extractall(dest, filter="data")
    return dest / f"python/bin/python{RUNTIME['version']}"


def install_crt(wheel, names, dest):
    """Copy only the named C++ runtime DLLs from the wheel's data directory next to python.exe."""
    with zipfile.ZipFile(wheel) as z:
        for name in names:
            members = [m for m in z.namelist() if m.endswith(".data/data/" + name)]
            if len(members) != 1:
                raise ValueError(f"C++ 執行階段 wheel 缺少 {name}")
            (dest / name).write_bytes(z.read(members[0]))


def install_wheel(wheel, site):
    with zipfile.ZipFile(wheel) as z:
        for member in z.namelist():
            parts = Path(member).parts
            if member.startswith("/") or ".." in parts:
                raise ValueError(f"wheel 路徑不安全：{member}")
            # Package test suites are not needed at runtime.
            if "tests" in parts[:-1] and parts[0] in ("numpy", "onnxruntime"):
                continue
            z.extract(member, site)


def precompile(tree, compiler):
    """Bake hash-based bytecode that Python uses without checking source times.

    Workers run with -B, so installed files never change after installation (receipts stay valid).
    Bytecode is platform independent for one CPython version; any interpreter of that release can build it.
    """
    for cache in tree.rglob("__pycache__"):
        shutil.rmtree(cache)
    subprocess.run([str(compiler), "-I", "-m", "compileall", "-q", "-f", "-j", "0",
                    "--invalidation-mode", "unchecked-hash", str(tree)], check=True)


def build(args):
    dictionary = digest(args.dictionary / "manifest.json")
    weights = hf_models.fetch(args.models, args.models_cache, log=sys.stderr)
    out = args.out
    if out.exists():
        raise ValueError(f"輸出已存在：{out}")
    stage = Path(tempfile.mkdtemp(prefix=".neural-", dir=out.parent))
    try:
        archive = fetch(*RUNTIME["python"], args.cache, args.python_archive)
        site = install_python(archive, stage / "python")
        for url, sha256 in RUNTIME["wheels"]:
            local = next((w for w in args.wheel if Path(w).name == url.rsplit("/", 1)[-1]), None)
            install_wheel(fetch(url, sha256, args.cache, local), site)
        url, sha256, names = RUNTIME["crt"]
        local = next((w for w in args.wheel if Path(w).name == url.rsplit("/", 1)[-1]), None)
        install_crt(fetch(url, sha256, args.cache, local), names, stage / "python")
        # The Windows stdlib ships as bytecode inside python314.zip; compile the packages with the
        # pinned Linux interpreter of the same CPython release.
        with tempfile.TemporaryDirectory(prefix=".compiler-", dir=out.parent) as temp:
            compiler = install_compiler(fetch(*COMPILER, args.cache, args.compiler_archive), Path(temp))
            precompile(site, compiler)
        for name in hf_models.WORKER_NAMES:
            shutil.copy2(hf_models.WORKERS / name, stage / name)
        hf_models.write_packages(weights, dictionary, stage)
        (stage / "README.txt").write_text(
            "靈犀注音內附的神經選字與提交時補標點：可攜 Python " + RUNTIME["version"] +
            "（numpy、onnxruntime）、worker 與模型。只在本機執行，不連網。\n"
            "切換：0c-update set selection statistical|neural、0c-update set punctuation on|off。\n",
            encoding="utf-8")
        os.replace(stage, out)
    except BaseException:
        shutil.rmtree(stage, ignore_errors=True)
        raise
    return out


def main():
    parser = argparse.ArgumentParser(description="組出 Windows 套件內附的 neural/ 目錄（可攜 Python、worker、模型）；"
                                     "在 Linux x86-64 執行（編 bytecode 的是同版本的 Linux 版 Python）")
    parser.add_argument("--models", type=Path, help="Hugging Face 原檔目錄（<dir>/<select|punct>/model.onnx、LICENSE、README.md，"
                        "例如 0c-ime 的 models/）；省略則用 --models-cache 或從 Hugging Face 下載。一律核對 models.lock")
    parser.add_argument("--models-cache", type=Path, default=hf_models.CACHE, help="模型下載快取")
    parser.add_argument("--dictionary", required=True, type=Path, help="將一起出貨的系統字典目錄（模型配對到它）")
    parser.add_argument("--out", required=True, type=Path, help="輸出目錄（不可已存在）")
    parser.add_argument("--cache", type=Path, default=ROOT / "build/cache/neural", help="下載快取")
    parser.add_argument("--python-archive", help="已下載的 Python 壓縮檔（仍核對雜湊）")
    parser.add_argument("--wheel", action="append", default=[], help="已下載的 wheel（仍核對雜湊）")
    parser.add_argument("--compiler-archive", help="已下載的編 bytecode 用 Linux Python 壓縮檔（仍核對雜湊）")
    args = parser.parse_args()
    try:
        print(build(args))
    except (OSError, ValueError, KeyError) as error:
        print(f"準備神經套件失敗：{error}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
