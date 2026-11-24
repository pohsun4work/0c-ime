"""Shared checks and staging helpers for the Windows bundle (scripts/package_windows.py) and release tools.

Licenses, release docs, dictionary provenance and model pairing are validated here without rewriting anything.
"""

import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess


ROOT = Path(__file__).resolve().parents[1]


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def regular(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"需要一般檔案：{path}")
    return path


LICENSE_FILES = {
    "LICENSE": "LICENSE",
    "legal/README.md": "legal/README.md",
    "legal/CC-BY-SA-4.0.txt": "legal/CC-BY-SA-4.0.txt",
    "legal/UNICODE-NOTICE.txt": "legal/UNICODE-NOTICE.txt",
    "legal/TERMS.md": "TERMS.md",
}
DATA_FILES = ("lexicon.bin", "lm.bin", "manifest.json", "NOTICE.txt", "report.txt")
ENGLISH_FILES = ("words.bin", "manifest.json", "NOTICE.txt")


def check_licenses():
    for name in LICENSE_FILES:
        if not regular(ROOT / name).read_text(encoding="utf-8").strip():
            raise ValueError(f"授權文件不可為空：{name}")


def copy_licenses(stage):
    check_licenses()
    for source, destination in LICENSE_FILES.items():
        target = stage / destination
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / source, target)


RELEASE_DOC_FILES = ("INSTALL.md", "PRIVACY.md", "UPDATING.md")


def check_release_docs(directory):
    if directory is None:
        raise ValueError("完整安裝包需要 --release-docs 指定安裝說明、隱私與更新文件")
    directory = Path(directory)
    for name in RELEASE_DOC_FILES:
        if not regular(directory / name).read_text(encoding="utf-8").strip():
            raise ValueError(f"安裝包文件不可為空：{name}")
    return directory


def copy_release_docs(directory, stage):
    directory = check_release_docs(directory)
    for name in RELEASE_DOC_FILES:
        shutil.copy2(directory / name, stage / name)


def check_english(data):
    directory = data / "english"
    if not directory.exists() and not directory.is_symlink():
        return None  # Older Chinese packs remain usable.
    if directory.is_symlink() or not directory.is_dir():
        raise ValueError("英文資料包必須是一般目錄")
    manifest = json.loads(regular(directory / "manifest.json").read_text(encoding="utf-8"))
    count = manifest.get("words")
    if manifest.get("format") != 1 or type(count) is not int or not 0 < count <= 200000 or not manifest.get("ranking"):
        raise ValueError("英文資料包格式或詞數不合法")
    files = manifest.get("files", {})
    if set(files) != {"words.bin", "NOTICE.txt"}:
        raise ValueError("英文資料包檔案清單不合法")
    for name in ("words.bin", "NOTICE.txt"):
        path = regular(directory / name)
        info = files[name]
        if digest(path) != info.get("sha256") or path.stat().st_size != info.get("size"):
            raise ValueError(f"英文資料包雜湊或大小不符：{name}")
    if not manifest.get("sources"):
        raise ValueError("英文資料包缺少來源紀錄")
    for source in manifest["sources"]:
        for field in ("name", "version", "url", "sha256", "license", "attribution"):
            if not str(source.get(field, "")).strip():
                raise ValueError(f"英文資料包缺少來源 {field}")
    if not (directory / "NOTICE.txt").read_text(encoding="utf-8").strip():
        raise ValueError("英文資料包缺少授權聲明")
    return manifest


def copy_english(source, destination, go):
    """Stage an independent English pack before sealing/signing; no model re-pairing."""
    if check_english(source) is not None:
        if source.resolve() != destination.resolve():
            target = destination / "english"
            target.mkdir()
            for name in ENGLISH_FILES:
                shutil.copy2(regular(source / "english" / name), target / name)
    else:
        subprocess.run([go, "run", "./cmd/0c-english", "-source", str(ROOT / "data/english"),
                        "-out", str(destination / "english")], cwd=ROOT, check=True)
    check_english(destination)


def valid_version(version):
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[a-zA-Z0-9]+(?:\.[a-zA-Z0-9]+)*)?", version):
        return False
    return all(not (part.isdigit() and len(part) > 1 and part[0] == "0")
               for part in version.partition("-")[2].split("."))


def check_data(data, public):
    manifest = json.loads(regular(data / "manifest.json").read_text(encoding="utf-8"))
    if manifest.get("format") not in (1, 2):
        raise ValueError("資料包格式須為 1 或 2")
    for name in DATA_FILES:
        regular(data / name)
    for name in ("lexicon.bin", "lm.bin"):
        info = manifest.get("files", {}).get(name, {})
        if digest(data / name) != info.get("sha256") or (data / name).stat().st_size != info.get("size"):
            raise ValueError(f"資料包雜湊或大小不符：{name}")
    if public:
        check_public_sources(manifest, (data / "NOTICE.txt").read_text(encoding="utf-8"))
    check_english(data)
    return manifest


# Legacy identifiers are rejected for public packages, never silently removed.
PRIVATE_RESEARCH_SOURCE = "zuin-news-stratified-20260921"
PRIVATE_LICENSE = "私有"
NEURAL_PACKAGES = ("select", "punct")


def private_source(source):
    return isinstance(source, dict) and (source.get("ID") == PRIVATE_RESEARCH_SOURCE
                                         or str(source.get("License", "")).strip() == PRIVATE_LICENSE)


def check_public_sources(manifest, notice):
    """Validate public provenance without rewriting the manifest or its notices."""
    options = manifest.get("options")
    if not isinstance(options, dict) or options.get("Public") is not True:
        raise ValueError("公開包需要以 0c-compile -public 重新編譯的資料包")
    # Old post-compilation mixes may have had their source removed by older packagers.
    if "research_news_mix" in manifest:
        raise ValueError("舊混合資料的來源無法完整確認，請以 -public 重新編譯")
    sources = manifest.get("sources")
    if not isinstance(sources, list) or not sources:
        raise ValueError("資料包缺少來源紀錄或來源格式不合法")
    seen = set()
    for source in sources:
        if not isinstance(source, dict):
            raise ValueError("來源紀錄格式不合法")
        source_id = source.get("ID")
        if source.get("Distributable") is not True or private_source(source):
            raise ValueError(f"來源 {source_id!r} 不可散布或未明確標示可散布；請重建資料包")
        for field in ("ID", "Name", "URL", "Version", "License", "Attribution"):
            value = source.get(field)
            if not isinstance(value, str) or value.strip().lower() in ("", "-", "未定", "unknown") or any(c in value for c in "\r\n"):
                raise ValueError(f"來源 {source_id!r} 缺少或具有無效的 {field}")
        if source_id in (".", "..") or any(c in source_id for c in "/\\"):
            raise ValueError(f"來源 ID 無效：{source_id!r}")
        if source_id in seen:
            raise ValueError(f"重複來源 ID：{source_id}")
        seen.add(source_id)
    notice = notice.replace("\r\n", "\n")
    if any(line.lstrip().startswith("研究來源：") for line in notice.splitlines()):
        raise ValueError("NOTICE 含舊混合來源紀錄，請以 -public 重新編譯，不得移除標記後出貨")
    headings = list(re.finditer(r"(?m)^== (.+)（([^（）\n]+)）==$", notice))
    sections = {}
    for index, heading in enumerate(headings):
        name, source_id = heading.groups()
        if source_id in sections:
            raise ValueError(f"NOTICE 有重複來源：{source_id}")
        end = headings[index + 1].start() if index + 1 < len(headings) else len(notice)
        sections[source_id] = (name, notice[heading.end():end].strip())
    if set(sections) != seen:
        raise ValueError("NOTICE 的逐來源授權聲明與 manifest 不一致")
    for source in sources:
        name, section = sections[source["ID"]]
        metadata, separator, statement = section.partition("\n\n")
        expected = [f"{label}：{source[field]}" for label, field in (
            ("授權", "License"), ("版本", "Version"), ("網址", "URL"), ("姓名標示", "Attribution"))]
        if name != source["Name"] or any(line not in metadata.splitlines() for line in expected):
            raise ValueError(f"NOTICE 來源 {source['ID']} 的授權資訊與 manifest 不一致")
        if not separator or not statement.strip():
            raise ValueError(f"NOTICE 來源 {source['ID']} 缺少授權全文或聲明")


def json_bytes(value):
    # 與 0c-compile／模型匯出寫出的格式相同（兩格縮排、不轉義中文、結尾換行）。
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def prepare_data_manifest(data, neural=None):
    """Verify model binding and return the unchanged dictionary manifest hash.

    Packaging must not delete source metadata or rebind models to conceal provenance.
    """
    path = regular(data / "manifest.json")
    original = path.read_bytes()
    old = hashlib.sha256(original).hexdigest()
    if neural is not None:
        neural = Path(neural)
        for kind in NEURAL_PACKAGES:
            if kind != "select" and not (neural / kind / "manifest.json").exists():
                continue
            package_manifest = json.loads(regular(neural / kind / "manifest.json").read_text(encoding="utf-8"))
            vocab = json.loads(regular(neural / kind / "vocab.json").read_text(encoding="utf-8"))
            if package_manifest.get("files", {}).get("vocab.json") != digest(neural / kind / "vocab.json"):
                raise ValueError(f"神經模型 {kind} 的 vocab.json 與模型 manifest 不符")
            recorded = [m["dictionary_sha256"] for m in (package_manifest, vocab) if "dictionary_sha256" in m]
            if kind == "select" and len(recorded) != 2 or any(value != old for value in recorded):
                raise ValueError("神經選字模型與本套件字典不配對" if kind == "select" else f"神經模型 {kind} 與本套件字典不配對")
    return old


def prepare_shipped_data(data, neural=None, *, public=True):
    """Validate staged data and model pairing without changing provenance or bindings."""
    check_data(data, public)
    return prepare_data_manifest(data, neural)


def seal_data(stage, go, *, public=True):
    """Validate before obfuscation; source metadata and model bindings stay unchanged."""
    neural = stage / "neural"
    prepare_shipped_data(stage / "data", neural if neural.is_dir() else None, public=public)
    subprocess.run([go, "run", "./cmd/0c-compile", "-seal", str(stage / "data")], cwd=ROOT, check=True)


def copy_neural(source, stage):
    """Add the bundled neural models; the selection model must be paired with this bundle's dictionary."""
    source = source.resolve()
    if any(p.is_symlink() for p in source.rglob("*")):
        raise ValueError("神經套件不接受符號連結")
    select = json.loads(regular(source / "select/manifest.json").read_text(encoding="utf-8"))
    if select.get("dictionary_sha256") != digest(stage / "data/manifest.json"):
        raise ValueError("神經選字模型與本套件字典不配對")
    for name in ("sequence_runtime.py", "punct_runtime.py", "punct/manifest.json"):
        regular(source / name)
    shutil.copytree(source, stage / "neural")
