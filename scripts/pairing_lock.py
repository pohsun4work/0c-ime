"""出貨字典的 pairing.lock.json 內容（#180）：models/models.lock.json 記的上游模型配對到某份字典的結果。

scripts/hf_models.py 依它產生與核對官網套件內建的模型。配對規則與 0c-ime 的 scripts/pair_models.py 的 metadata()
相同：只改 vocab.json、manifest.json 的 dictionary_sha256 與 manifest 記的 vocab 雜湊，權重不動。上游的
manifest.json、vocab.json 在 models/upstream。
"""

import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LOCK = ROOT / "models/models.lock.json"
UPSTREAM = ROOT / "models/upstream"
ROLES = ("select", "punct")


def json_bytes(value):
    # 與 0c-compile／模型匯出寫出的格式相同（兩格縮排、不轉義中文、結尾換行）。
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def digest_bytes(content):
    return hashlib.sha256(content).hexdigest()


def metadata(manifest, vocab, dictionary):
    """0c-ime scripts/pair_models.py 的 metadata()，一字不改。"""
    manifest, vocab = dict(manifest), dict(vocab)
    manifest["files"] = dict(manifest["files"])
    vocab["dictionary_sha256"] = dictionary
    if "dictionary_sha256" in manifest:
        manifest["dictionary_sha256"] = dictionary
    vocab_bytes = json_bytes(vocab)
    manifest["files"]["vocab.json"] = hashlib.sha256(vocab_bytes).hexdigest()
    return {"vocab.json": vocab_bytes, "manifest.json": json_bytes(manifest)}


def pairing_lock(dictionary, lock_path=LOCK, upstream=UPSTREAM):
    """回傳 dictionary（字典 manifest 的 SHA-256）的 pairing.lock.json 內容；上游 metadata 先核對 models.lock。"""
    if len(dictionary) != 64 or any(c not in "0123456789abcdef" for c in dictionary):
        raise ValueError("字典雜湊不正確")
    lock_bytes = Path(lock_path).read_bytes()
    lock = json.loads(lock_bytes)
    if lock.get("schema") != 1 or set(lock.get("models", {})) != set(ROLES):
        raise ValueError("models.lock.json 格式不符")
    packages = {}
    for role in ROLES:
        files = {}
        for name in ("manifest.json", "vocab.json"):
            content = (Path(upstream) / role / name).read_bytes()
            want = lock["models"][role]["files"][name]
            if digest_bytes(content) != want["sha256"] or len(content) != want["size"]:
                raise ValueError(f"models/upstream/{role}/{name} 與 models.lock.json 不符")
            files[name] = json.loads(content)
        derived = metadata(files["manifest.json"], files["vocab.json"], dictionary)
        packages[role] = {name: {"sha256": digest_bytes(content), "size": len(content)} for name, content in derived.items()}
    doc = {"schema": 1, "dictionary_manifest_sha256": dictionary, "models_lock_sha256": digest_bytes(lock_bytes), "packages": packages}
    return (json.dumps(doc, indent=2, sort_keys=True) + "\n").encode("utf-8")
