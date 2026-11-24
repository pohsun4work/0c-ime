"""Check release rejection and tampering in temporary directories."""

import json
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import make_installer
import package_release as packager
import package_windows
import package_windows_installer
import release_feed
import site_manifest


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)

    @staticmethod
    def data_snapshot(stage):
        return {p.relative_to(stage): (p.read_bytes(), p.stat().st_mtime_ns)
                for p in stage.rglob("*") if p.is_file()}

    @staticmethod
    def source_notice(sources, statements=None):
        statements = statements or ["Permission is granted to redistribute this test fixture."] * len(sources)
        return "資料包\n\n" + "\n".join(
            f"== {s['Name']}（{s['ID']}）==\n授權：{s['License']}\n版本：{s['Version']}\n"
            f"網址：{s['URL']}\n姓名標示：{s['Attribution']}\n\n{statement}\n"
            for s, statement in zip(sources, statements))

    def write_paired_data(self, stage, *, public=False):
        """A paired fixture, including legacy mixed data that must never be sanitized for release."""
        source = {"ID": "fixture", "Name": "測試詞條", "URL": "https://example.org/fixture",
                  "Version": "1.0", "License": "MIT", "Attribution": "Fixture authors", "Distributable": True}
        manifest = {"format": 2, "files": {}, "counts": {"words": 3},
                    "options": {"Public": public}, "sources": [source], "runtime": {"context_weight": 0.075}}
        if not public:
            source = dict(source, ID=packager.PRIVATE_RESEARCH_SOURCE, License="研究資料", Distributable=False)
            manifest["sources"].append(source)
            manifest["research_news_mix"] = {"source": source, "base_manifest_sha256": "c" * 64,
                                             "bigram": {"max_row_mass_change": 1e-12}}
        data, neural = stage / "data", stage / "neural"
        data.mkdir(parents=True)
        for name in ("lexicon.bin", "lm.bin"):
            (data / name).write_bytes(("fixture " + name).encode())
            manifest["files"][name] = {"sha256": packager.digest(data / name), "size": (data / name).stat().st_size}
        (data / "report.txt").write_text("fixture\n")
        (data / "manifest.json").write_bytes(packager.json_bytes(manifest))
        notice = self.source_notice(manifest["sources"])
        if not public:
            notice += "研究來源：" + json.dumps(source, ensure_ascii=False) + "\n"
        (data / "NOTICE.txt").write_text(notice, encoding="utf-8")
        old = packager.digest(data / "manifest.json")
        for kind in ("select", "punct"):
            (neural / kind).mkdir(parents=True)
            (neural / kind / "model.onnx").write_bytes(kind.encode())
            (neural / kind / "vocab.json").write_bytes(packager.json_bytes(
                {"schema": "zuin-phonetic-v1", "characters": ["一", "ㄅ"], "dictionary_sha256": old}))
            model = {"schema": kind, "files": {"model.onnx": packager.digest(neural / kind / "model.onnx"),
                                               "vocab.json": packager.digest(neural / kind / "vocab.json")}}
            if kind == "select":
                model["dictionary_sha256"] = old
            (neural / kind / "manifest.json").write_bytes(packager.json_bytes(model))
        return data, neural, manifest, old

    def test_public_collect_sources_retain_attribution_and_license(self):
        root = Path(__file__).resolve().parents[1]
        sources, statements = [], []
        for line in (root / "data/sources/ledger.tsv").read_text(encoding="utf-8").splitlines():
            fields = line.split("\t")
            if fields[0] in ("0c_collect", "0c_collect-ns", "0c_collect-ss"):
                source = dict(zip(("ID", "Name", "URL", "Version", "License", "Distributable", "Attribution"), fields))
                self.assertEqual(source["License"], "CC-BY-SA-4.0")
                self.assertEqual(source["Distributable"], "yes")
                source["Distributable"] = True
                sources.append(source)
                statements.append((root / "data/sources/licenses" / (source["ID"] + ".txt")).read_text(encoding="utf-8"))
        self.assertEqual(len(sources), 3)
        data, _, manifest, _ = self.write_paired_data(self.root / "collect", public=True)
        manifest["sources"] = sources
        (data / "manifest.json").write_bytes(packager.json_bytes(manifest))
        (data / "NOTICE.txt").write_text(self.source_notice(sources, statements), encoding="utf-8")
        before = self.data_snapshot(data)
        self.assertEqual(packager.prepare_shipped_data(data), packager.digest(data / "manifest.json"))
        self.assertEqual(self.data_snapshot(data), before)

    def test_ledger_sources_have_complete_public_notices(self):
        sources, statements = [], []
        for line in (packager.ROOT / "data/sources/ledger.tsv").read_text(encoding="utf-8").splitlines():
            if not line or line.startswith("#"):
                continue
            values = line.split("\t")
            self.assertEqual(len(values), 7)
            source = dict(zip(("ID", "Name", "URL", "Version", "License", "Distributable", "Attribution"), values))
            self.assertEqual(source["Distributable"], "yes")
            source["Distributable"] = True
            sources.append(source)
            statement = (packager.ROOT / "data/sources/licenses" / (source["ID"] + ".txt")).read_text(encoding="utf-8")
            self.assertTrue(statement.strip(), source["ID"])
            statements.append(statement)
        packager.check_public_sources({"options": {"Public": True}, "sources": sources},
                                     self.source_notice(sources, statements))

    def test_preview_preserves_private_sources_notices_and_model_pairing(self):
        stage = self.root / "stage"
        data, neural, _, old = self.write_paired_data(stage)
        before = self.data_snapshot(stage)
        self.assertEqual(packager.prepare_shipped_data(data, neural, public=False), old)
        self.assertEqual(self.data_snapshot(stage), before)
        with self.assertRaisesRegex(ValueError, "-public"):
            packager.prepare_shipped_data(data, neural)
        self.assertEqual(self.data_snapshot(stage), before)

    def test_public_manifest_and_models_are_never_rebound(self):
        stage = self.root / "stage"
        data, neural, _, old = self.write_paired_data(stage, public=True)
        before = self.data_snapshot(stage)
        for models in (None, neural):
            self.assertEqual(packager.prepare_shipped_data(data, models), old)
            self.assertEqual(self.data_snapshot(stage), before)

    def test_shipped_manifest_rejects_broken_pairing_without_writing(self):
        def snapshot(stage):
            return {p: p.read_bytes() for p in stage.rglob("*") if p.is_file()}
        cases = {
            "選字 manifest": ("select/manifest.json", "dictionary_sha256", "不配對"),
            "選字 vocab": ("select/vocab.json", "dictionary_sha256", "vocab.json 與模型 manifest 不符"),
            "補標點 vocab": ("punct/vocab.json", "dictionary_sha256", "vocab.json 與模型 manifest 不符"),
        }
        for label, (name, key, error) in cases.items():
            with self.subTest(label):
                stage = self.root / ("broken-" + name.replace("/", "-"))
                data, neural, _, _ = self.write_paired_data(stage)
                path = neural / name
                value = json.loads(path.read_text(encoding="utf-8"))
                value[key] = "d" * 64
                path.write_bytes(packager.json_bytes(value))
                before = snapshot(stage)
                with self.assertRaisesRegex(ValueError, error):
                    packager.prepare_data_manifest(data, neural)
                self.assertEqual(snapshot(stage), before)
        # vocab.json 雜湊照樣登記在模型 manifest、只是記的字典不同：配對不成立。
        for kind in ("select", "punct"):
            with self.subTest(kind + " 登記過的 vocab"):
                stage = self.root / ("stale-" + kind)
                data, neural, _, _ = self.write_paired_data(stage)
                vocab = json.loads((neural / kind / "vocab.json").read_text(encoding="utf-8"))
                vocab["dictionary_sha256"] = "d" * 64
                (neural / kind / "vocab.json").write_bytes(packager.json_bytes(vocab))
                model = json.loads((neural / kind / "manifest.json").read_text(encoding="utf-8"))
                model["files"]["vocab.json"] = packager.digest(neural / kind / "vocab.json")
                (neural / kind / "manifest.json").write_bytes(packager.json_bytes(model))
                before = snapshot(stage)
                with self.assertRaisesRegex(ValueError, "不配對"):
                    packager.prepare_data_manifest(data, neural)
                self.assertEqual(snapshot(stage), before)

    def test_seal_data_rejects_before_obfuscation_or_metadata_changes(self):
        stage = self.root / "stage"
        self.write_paired_data(stage)
        before = self.data_snapshot(stage)
        with patch.object(packager.subprocess, "run") as run:
            with self.assertRaisesRegex(ValueError, "-public"):
                packager.seal_data(stage, "go")
        run.assert_not_called()
        self.assertEqual(self.data_snapshot(stage), before)

    def test_seal_data_preserves_provenance_and_pairing(self):
        for public in (False, True):
            with self.subTest(public=public):
                stage = self.root / str(public)
                self.write_paired_data(stage, public=public)
                before = self.data_snapshot(stage)
                with patch.object(packager.subprocess, "run") as run:
                    packager.seal_data(stage, "go", public=public)
                run.assert_called_once_with(["go", "run", "./cmd/0c-compile", "-seal", str(stage / "data")],
                                            cwd=packager.ROOT, check=True)
                self.assertEqual(self.data_snapshot(stage), before)

    def test_english_staging_keeps_chinese_hash_and_detects_tampering(self):
        source = self.root / "data"
        source.mkdir()
        (source / "manifest.json").write_text("chinese dictionary manifest")
        before = packager.digest(source / "manifest.json")
        packager.copy_english(source, source, "go")
        self.assertEqual(packager.digest(source / "manifest.json"), before)
        manifest = packager.check_english(source)
        self.assertEqual(manifest["words"], 82015)
        copied = self.root / "copied-data"
        copied.mkdir()
        with patch.object(packager.subprocess, "run", side_effect=AssertionError("Existing packs must be copied")):
            packager.copy_english(source, copied, "go")
        for name in packager.ENGLISH_FILES:
            self.assertEqual(packager.digest(source / "english" / name), packager.digest(copied / "english" / name))
        (copied / "english/words.bin").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "雜湊"):
            packager.check_english(copied)

    def test_one_line_installer_pins_bootstrap_and_rejects_unsafe_values(self):
        boot = self.root / "0c-update.exe"
        boot.write_bytes(b"bootstrap")
        url = "https://example.test/releases/v/0c-update.exe"
        out = make_installer.generate(self.root / "out", "https://example.test/feeds", "https://example.test/feeds/public-beta.json",
                                      "public-beta", (boot, url))
        ps1 = out["install.ps1"].read_text(encoding="utf-8")
        self.assertIn(make_installer.sha256(boot), ps1)
        self.assertIn(url, ps1)
        # irm | iex 以 UTF-8 取得腳本；ParseFile 在 Windows PowerShell 5.1 會把沒有 BOM 的檔案當 ANSI 讀
        check = ("$e = $null; [void][System.Management.Automation.Language.Parser]::ParseInput([IO.File]::ReadAllText("
                 f"'{out['install.ps1']}', [Text.Encoding]::UTF8), [ref]$null, [ref]$e); if ($e) {{ $e; exit 1 }}")
        subprocess.run(["powershell", "-NoProfile", "-NonInteractive", "-Command", check], check=True)
        self.assertIn(b"\r\n", out["install.cmd"].read_bytes())
        with self.assertRaises(ValueError):
            make_installer.generate(self.root / "bad", "https://example.test/feeds", "https://x/'; rm -rf ~", "public-beta")

    def test_windows_packaging_checks_the_prebuilt_settings_program(self):
        def pe(machine=b"\x64\x86"):
            return b"MZ" + bytes(0x3a) + (0x40).to_bytes(4, "little") + b"PE\0\0" + machine + bytes(40)

        def prebuilt(name, body=None, notices="Tauri 授權文字"):
            directory = self.root / name
            directory.mkdir()
            (directory / "0c-settings.exe").write_bytes(pe() if body is None else body)
            (directory / "NOTICES.txt").write_text(notices)
            return directory
        program, notices = package_windows.check_settings(prebuilt("win-ok"))
        self.assertEqual((program.name, notices), ("0c-settings.exe", "Tauri 授權文字"))
        with self.assertRaisesRegex(ValueError, "x64"):
            package_windows.check_settings(prebuilt("win-arm", pe(b"\x64\xaa")))
        with self.assertRaisesRegex(ValueError, "PE"):
            package_windows.check_settings(prebuilt("win-elf", b"\x7fELF" + bytes(80)))
        with self.assertRaisesRegex(ValueError, "授權聲明"):
            package_windows.check_settings(prebuilt("win-notices", notices=" \n"))
        with self.assertRaisesRegex(ValueError, "一般檔案"):
            package_windows.check_settings(self.root / "win-missing")
        self.assertIn("0c-settings.exe", package_windows.DESCRIPTIONS)
        info = package_windows.version_info("0c-settings.exe", "0.1.0-dev.43", False)
        self.assertIn("靈犀注音設定程式".encode("utf-16-le"), info)
        self.assertIn("0c-settings".encode("utf-16-le"), info)
        # 0c-mcp（#126）同樣寫入版本資訊並簽章；說明出現在檔案內容與 SmartScreen 報告。
        info = package_windows.version_info("0c-mcp.exe", "0.2.0-dev.1", False)
        self.assertIn(package_windows.DESCRIPTIONS["0c-mcp.exe"].encode("utf-16-le"), info)
        # 0c-updatew（#289）：排程與通知按鈕執行的啟動器必須是 GUI 子系統（2），主控台程式（3）會閃出視窗。
        self.assertIn("0c-updatew.exe", package_windows.DESCRIPTIONS)

        def with_subsystem(value):
            body = bytearray(pe() + bytes(64))
            struct.pack_into("<H", body, 0x40 + 24 + 68, value)
            target = self.root / f"subsystem-{value}.exe"
            target.write_bytes(bytes(body))
            return target
        self.assertEqual(package_windows.pe_subsystem(with_subsystem(2)), package_windows.PE_SUBSYSTEM_GUI)
        self.assertEqual(package_windows.pe_subsystem(with_subsystem(3)), 3)

    def test_windows_binary_architecture_validation(self):
        import struct
        file = self.root / "fixture.exe"
        file.write_bytes(b"MZ" + b"\0" * 58 + struct.pack("<I", 64) + b"PE\0\0\x64\x86")
        package_windows.check_pe(file)
        file.write_bytes(file.read_bytes()[:-2] + b"\x4c\x01")
        with self.assertRaisesRegex(ValueError, "x64"):
            package_windows.check_pe(file)
        # 32 位元應用程式用的 x86/0c_tsf.dll（#112）要是 i686，不能誤放 x64 那份。
        package_windows.check_pe(file, "x86")
        file.write_bytes(file.read_bytes()[:-2] + b"\x64\x86")
        with self.assertRaisesRegex(ValueError, "x86"):
            package_windows.check_pe(file, "x86")
        self.assertEqual(package_windows.DLL32, "x86/0c_tsf.dll")

    def test_windows_binaries_do_not_need_the_visual_cpp_redistributable(self):
        # #261：0.2.7／0.2.8 的 x64 0c_tsf.dll 與 0c-ui.exe 匯入 VCRUNTIME140.dll，沒裝 VC++ 可轉散發套件的電腦
        # regsvr32 載不了 DLL（exit status 3），安裝精靈在註冊那步失敗。UCRT（api-ms-win-crt-*）是 Windows 10 內建，可以。
        import struct

        def fake_pe(imports, delayed=(), pe32=False):
            # 一個 .idata 區段（RVA 0x1000、檔案位移 0x200）放匯入描述、延遲載入描述與 DLL 名稱。
            rva = 0x1000
            table, delay_table = 20 * (len(imports) + 1), 32 * (len(delayed) + 1)
            strings, names = b"", []
            for name in (*imports, *delayed):
                names.append(rva + table + delay_table + len(strings))
                strings += name.encode() + b"\0"
            section = b"".join(struct.pack("<5I", 0, 0, 0, n, 0) for n in names[:len(imports)]) + bytes(20)
            section += b"".join(struct.pack("<8I", 1, n, 0, 0, 0, 0, 0, 0) for n in names[len(imports):]) + bytes(32)
            section += strings
            magic, directories = (0x10B, 96) if pe32 else (0x20B, 112)
            optional = bytearray(directories + 16 * 8)
            struct.pack_into("<H", optional, 0, magic)
            struct.pack_into("<I", optional, directories - 4, 16)
            struct.pack_into("<II", optional, directories + 8, rva, table)
            if delayed:
                struct.pack_into("<II", optional, directories + 13 * 8, rva + table, delay_table)
            coff = struct.pack("<HHIIIHH", 0x14C if pe32 else 0x8664, 1, 0, 0, 0, len(optional), 0x2022)
            header = struct.pack("<8sIIIIIIHHI", b".idata", len(section), rva, len(section), 0x200, 0, 0, 0, 0, 0)
            return (b"MZ" + bytes(58) + struct.pack("<I", 64) + b"PE\0\0" + coff + bytes(optional) + header).ljust(0x200, b"\0") + section

        file = self.root / "0c_tsf.dll"
        dynamic = ("KERNEL32.dll", "msctf.dll", "VCRUNTIME140.dll", "api-ms-win-crt-runtime-l1-1-0.dll")
        file.write_bytes(fake_pe(dynamic))
        self.assertEqual(package_windows.pe_imports(file), list(dynamic))
        with self.assertRaisesRegex(ValueError, "VCRUNTIME140.dll"):
            package_windows.check_static_crt(file)
        for imports, delayed, pe32 in ((("kernel32.dll",), ("MSVCP140.dll",), False),
                                       (("kernel32.dll", "vcruntime140_1.dll"), (), False),
                                       (("KERNEL32.dll", "VCRUNTIME140.dll"), (), True)):
            file.write_bytes(fake_pe(imports, delayed, pe32))
            self.assertEqual(package_windows.pe_imports(file), [*imports, *delayed])
            with self.assertRaisesRegex(ValueError, "Visual C\\+\\+"):
                package_windows.check_static_crt(file)
        static = ("kernel32.dll", "ole32.dll", "msctf.dll", "api-ms-win-crt-heap-l1-1-0.dll", "ucrtbase.dll")
        for pe32 in (False, True):
            file.write_bytes(fake_pe(static, ("oleacc.dll",), pe32))
            package_windows.check_static_crt(file)
        file.write_bytes(fake_pe(()))  # Go 程式只匯入 kernel32；沒有匯入也不算錯
        self.assertEqual(package_windows.pe_imports(file), [])
        file.write_bytes(b"MZ" + bytes(80))
        with self.assertRaisesRegex(ValueError, "PE"):
            package_windows.pe_imports(file)
        # 靜態連結寫在 Cargo 設定裡（每個 MSVC target，含 cargo test、驗收腳本自己的建置），不靠打包腳本的環境變數。
        import tomllib
        config = tomllib.loads((package_windows.PLATFORM / ".cargo/config.toml").read_text(encoding="utf-8"))
        flags = config["target"]['cfg(all(windows, target_env = "msvc"))']["rustflags"]
        self.assertIn("target-feature=+crt-static", " ".join(flags))

    def test_windows_version_info_resource(self):
        import struct
        def parse(data, offset=0):
            length, value_length, kind = struct.unpack_from("<HHH", data, offset)
            end = data.index(b"\0\0", offset + 6)
            while (end - offset - 6) % 2:
                end = data.index(b"\0\0", end + 1)
            key = data[offset + 6:end].decode("utf-16-le")
            cursor = (end + 2 + 3) & ~3
            size = value_length * 2 if kind == 1 else value_length
            value = data[cursor:cursor + size]
            cursor = (cursor + size + 3) & ~3
            children = []
            while cursor < offset + length:
                child, cursor = parse(data, cursor)
                children.append(child)
            return (key, kind, value, children), (offset + length + 3) & ~3

        data = package_windows.version_info("0c_tsf.dll", "0.1.0-dev.37", dll=True)
        (key, _, fixed, children), end = parse(data)
        self.assertEqual((key, end), ("VS_VERSION_INFO", len(data)))
        fields = struct.unpack("<13I", fixed)
        self.assertEqual(fields[0], 0xFEEF04BD)
        self.assertEqual(fields[2:4], (0 << 16 | 1, 0 << 16 | 0))   # 0.1.0.0
        self.assertEqual(fields[9], 2)                              # VFT_DLL
        (string_info, _, _, [table]), (var_info, _, _, [translation]) = children
        self.assertEqual((string_info, table[0], var_info), ("StringFileInfo", "040404B0", "VarFileInfo"))
        strings = {k: v.decode("utf-16-le").rstrip("\0") for k, kind, v, _ in table[3] if kind == 1}
        self.assertEqual(strings["CompanyName"], "FUNMULA PTE. LTD.")
        self.assertEqual(strings["FileVersion"], "0.1.0-dev.37")
        self.assertEqual(strings["OriginalFilename"], "0c_tsf.dll")
        self.assertEqual(strings["FileDescription"], package_windows.DESCRIPTIONS["0c_tsf.dll"])
        self.assertEqual(translation[2], struct.pack("<HH", 0x0404, 1200))
        exe = struct.unpack("<13I", parse(package_windows.version_info("0c-update.exe", "1.2.3", dll=False))[0][2])
        self.assertEqual((exe[2], exe[3], exe[9]), (1 << 16 | 2, 3 << 16 | 0, 1))

    def test_fork_publisher(self):
        data = package_windows.version_info("0c-ui.exe", "0.2.13-hoshi.1", False, "Hoshi 0c-ime fork")
        self.assertIn("Hoshi 0c-ime fork".encode("utf-16-le"), data)
        self.assertIn("© FUNMULA PTE. LTD.".encode("utf-16-le"), data)

    def test_authenticode_leaf_signer_ignores_timestamp_chain(self):
        output = """Signing Certificate Chain:
    Issued to: Microsoft Identity Verification Root Certificate Authority 2020
        Issued to: Microsoft ID Verified Code Signing PCA 2021
            Issued to: Microsoft ID Verified CS AOC CA 03
                Issued to: FUNMULA PTE. LTD.
The signature is timestamped: Fri Oct 02 00:19:52 2026
Timestamp Verified by:
    Issued to: Microsoft Identity Verification Root Certificate Authority 2020
        Issued to: Microsoft Public RSA Timestamping CA 2020
            Issued to: Microsoft Public RSA Time Stamping Authority
Successfully verified: 0c-update.exe
"""
        self.assertEqual(package_windows.leaf_signer(output), "FUNMULA PTE. LTD.")
        self.assertIsNone(package_windows.leaf_signer("SignTool Error: No signature found."))

    def test_setup_rejects_tampered_or_extra_payload_before_bootstrap(self):
        import hashlib
        import zipfile
        names = ('0c-update.exe', '0c-server.exe', '0c-ui.exe', '0c_tsf.dll', 'x86/0c_tsf.dll', 'TERMS.md', 'LICENSE', 'legal/README.md', 'THIRD_PARTY_NOTICES.txt',
                 'data/manifest.json', 'data/lexicon.bin', 'data/lm.bin', 'data/NOTICE.txt', 'data/report.txt')
        payload = {name: name.encode() for name in names}
        release = {'schema': 1, 'product': '0c', 'version': '0.1.0', 'channel': 'internal-preview',
                   'os': 'windows', 'arch': 'amd64',
                   'files': {name: hashlib.sha256(data).hexdigest() for name, data in payload.items()}}
        archive = self.root / 'windows.zip'
        def save():
            with zipfile.ZipFile(archive, 'w') as package:
                package.writestr('bundle/release.json', json.dumps(release))
                for name, data in payload.items():
                    package.writestr('bundle/' + name, data)
        save()
        _, updater, terms = package_windows_installer.read_bundle(archive)
        self.assertEqual(updater, payload['0c-update.exe'])
        self.assertEqual(terms, 'TERMS.md')
        payload['0c-update.exe'] = b'tampered'
        save()
        with self.assertRaisesRegex(ValueError, '雜湊'):
            package_windows_installer.read_bundle(archive)
        payload['0c-update.exe'] = b'0c-update.exe'
        payload['unexpected.dll'] = b'extra'
        save()
        with self.assertRaisesRegex(ValueError, '未列出'):
            package_windows_installer.read_bundle(archive)
        del payload['unexpected.dll']
        # 沒有 32 位元 DLL 的套件做不出能給 32 位元應用程式用的安裝精靈（#112）。
        saved = payload.pop('x86/0c_tsf.dll'), release['files'].pop('x86/0c_tsf.dll')
        save()
        with self.assertRaisesRegex(ValueError, '缺少必要檔案'):
            package_windows_installer.read_bundle(archive)
        payload['x86/0c_tsf.dll'], release['files']['x86/0c_tsf.dll'] = saved
        for name in ('TERMS.md', 'LICENSE', 'legal/README.md'):
            with self.subTest(missing=name):
                saved = payload.pop(name), release['files'].pop(name)
                save()
                with self.assertRaisesRegex(ValueError, '缺少必要檔案'):
                    package_windows_installer.read_bundle(archive)
                payload[name], release['files'][name] = saved

    def test_bundle_keeps_license_texts_and_terms_together(self):
        stage = self.root / "licensed-bundle"
        packager.copy_licenses(stage)
        for source, destination in packager.LICENSE_FILES.items():
            self.assertEqual((stage / destination).read_bytes(), (packager.ROOT / source).read_bytes())
        self.assertIn("GNU AFFERO GENERAL PUBLIC LICENSE", (stage / "LICENSE").read_text())
        self.assertIn("AGPL-3.0-only", (stage / "TERMS.md").read_text())

    def test_missing_license_stops_staging_before_any_copy(self):
        source = self.root / "license-source"
        source.mkdir()
        for name in packager.LICENSE_FILES:
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture license")
        for name in packager.LICENSE_FILES:
            with self.subTest(name=name):
                path = source / name
                path.unlink()
                stage = self.root / "incomplete-license-bundle"
                with patch.object(packager, "ROOT", source), self.assertRaises((ValueError, OSError)):
                    packager.copy_licenses(stage)
                self.assertFalse(stage.exists())
                path.write_text("fixture license")

    def test_notices_cover_shipped_go_and_rust(self):
        import collect_notices
        text = collect_notices.collect()
        self.assertIn("Go 工具鏈與標準函式庫", text)
        self.assertIn("Rust crate windows ", text)  # Windows 外殼的 crate
        self.assertIn("TERMS.md", text)
        self.assertIn("LICENSE", text)
        self.assertIn("legal/README.md", text)
        self.assertIn("wordfreq 3.1.1", text)  # 英文詞頻編在 0c-server 裡
        self.assertIn("Apache License", text)

    def test_versions_match_updater_constraints(self):
        for version in ("0.1.0", "1.2.3-beta.1", "1.0.0-rc.10"):
            self.assertTrue(packager.valid_version(version))
        for version in ("01.2.3", "1.0.0-01", "../escape", "1.0.0-", "1.0.0-beta..1"):
            self.assertFalse(packager.valid_version(version))

    CHANGELOG = """# 更新紀錄

## 未發布

- 還沒上架的內容不進官網。

## 0.1.0-dev.39（2026-10-03，public-beta）

### 公開摘要

- 新功能一句話，見[說明](https://0c.biggo.com/help/)。
- 第二項跨行
  接續。

### 開發紀錄

- `internal/path` 這類內部細節不出現在官網。

## 0.1.0-dev.38（2026-10-02，public-beta）

### 公開摘要

- 上一版。
"""
    BASE = "https://s3.biggo.com/0c-releases/releases/0.1.0-dev.39/"

    def site_feed(self, withdrawn=None):
        name = "0c-0.1.0-dev.39-windows-amd64.zip"
        return {"version": "0.1.0-dev.39", "channel": "public-beta", "withdrawn": withdrawn,
                "assets": [{"os": "windows", "arch": "amd64", "url": self.BASE + name, "sha256": "b" * 64, "size": 20}]}

    def site_uploads(self, feed):
        uploads = [(a["url"].rsplit("/", 1)[1], a["url"], a["sha256"], a["size"]) for a in feed["assets"]]
        name = "0c-0.1.0-dev.39-windows-amd64-setup.exe"
        uploads.append((name, self.BASE + name, "e" * 64, 5))
        return uploads

    def test_changelog_public_summary(self):
        entries = site_manifest.parse_changelog(self.CHANGELOG)
        self.assertEqual([e["version"] for e in entries], ["0.1.0-dev.39", "0.1.0-dev.38"])
        self.assertEqual(entries[0], {"version": "0.1.0-dev.39", "date": "2026-10-03", "stage": "public-beta",
                                      "items": ["新功能一句話，見[說明](https://0c.biggo.com/help/)。", "第二項跨行接續。"]})
        for bad in (self.CHANGELOG.replace("### 公開摘要\n\n- 上一版。\n", ""),  # 已上架版本缺公開摘要
                    self.CHANGELOG.replace("（2026-10-02，public-beta）", "(2026-10-02, public-beta)"),  # 標題格式
                    self.CHANGELOG.replace("https://0c.biggo.com/help/", "http://0c.biggo.com/help/"),
                    self.CHANGELOG.replace("新功能一句話", "**新功能**一句話"),
                    self.CHANGELOG.replace("- 上一版。", "上一版。")):
            with self.assertRaises(ValueError):
                site_manifest.parse_changelog(bad)
        preview = self.CHANGELOG.replace("（2026-10-03，public-beta）", "（2026-10-03，internal-preview）")
        self.assertEqual([e["version"] for e in site_manifest.parse_changelog(preview)], ["0.1.0-dev.38"])

    def test_site_manifest_matches_signed_feed(self):
        feed = self.site_feed()
        changelog = site_manifest.parse_changelog(self.CHANGELOG)
        site = site_manifest.build(feed, self.site_uploads(feed), changelog, "2026-10-03T08:00:00Z")
        self.assertEqual((site["schema"], site["version"], site["withdrawn"], site["changelog"]), (1, "0.1.0-dev.39", None, changelog))
        self.assertEqual(site["platforms"], {"windows-amd64": {
            "installer": {"url": self.BASE + "0c-0.1.0-dev.39-windows-amd64-setup.exe", "size": 5, "sha256": "e" * 64},
            "archive": {"url": feed["assets"][0]["url"], "size": 20, "sha256": "b" * 64}}})
        uploads = self.site_uploads(feed)
        uploads[0] = (uploads[0][0], uploads[0][1], "0" * 64, uploads[0][3])  # ZIP 與已簽章索引不符
        with self.assertRaises(ValueError):
            site_manifest.build(feed, uploads, changelog, "t")
        with self.assertRaises(ValueError):  # 沒有 setup.exe
            site_manifest.build(feed, self.site_uploads(feed)[:1], changelog, "t")
        linux = dict(feed, assets=feed["assets"] + [dict(feed["assets"][0], os="linux")])
        with self.assertRaisesRegex(ValueError, "linux/amd64"):  # 只收 Windows
            site_manifest.build(linux, self.site_uploads(feed), changelog, "t")
        with self.assertRaises(ValueError):  # CHANGELOG 最新的已上架版本不是索引版本
            site_manifest.build(feed, self.site_uploads(feed), changelog[1:], "t")

    def test_site_manifest_withdrawn(self):
        feed = self.site_feed(withdrawn=["0.1.0-dev.39"])
        changelog = site_manifest.parse_changelog(self.CHANGELOG)
        with self.assertRaises(ValueError):
            site_manifest.build(feed, self.site_uploads(feed), changelog, "t")
        site = site_manifest.build(feed, self.site_uploads(feed), changelog, "t", "此版有問題，請暫停下載。")
        self.assertEqual(site["withdrawn"], {"reason": "此版有問題，請暫停下載。"})
        with self.assertRaises(ValueError):  # 沒有撤回卻填理由
            site_manifest.build(self.site_feed(), self.site_uploads(feed), changelog, "t", "理由")

    def old_platforms(self, version="0.1.0-dev.38", setup=True):
        base = f"https://s3.biggo.com/0c-releases/releases/{version}/0c-{version}-windows-amd64"
        f = lambda suffix, size: {"url": base + suffix, "size": size, "sha256": "a" * 64}
        windows = {"archive": f(".zip", 6)}
        if setup:
            windows["installer"] = f("-setup.exe", 9)
        return {"windows-amd64": windows}

    def test_site_manifest_carries_past_downloads(self):
        # #81：舊版的下載資料從線上清單帶下來；上一版取線上清單最上層；目前版本不附。
        feed = self.site_feed()
        changelog = site_manifest.parse_changelog(self.CHANGELOG)
        history = {"version": "0.1.0-dev.38", "withdrawn": None, "platforms": self.old_platforms(),
                   "changelog": [{"version": "0.1.0-dev.38", "items": ["上一版。"]}]}
        site = site_manifest.build(feed, self.site_uploads(feed), changelog, "t", history=history)
        self.assertNotIn("platforms", site["changelog"][0])
        self.assertEqual(site["changelog"][1]["platforms"], self.old_platforms())
        # 之後的版本：已在 changelog 裡的資料原樣保留，即使它已不是線上清單的最上層。
        later = {"version": "0.1.0-dev.39", "platforms": site["platforms"], "changelog": site["changelog"]}
        again = site_manifest.build(feed, self.site_uploads(feed), changelog, "t", history=later)
        self.assertEqual(again["changelog"][1]["platforms"], self.old_platforms())
        # 沒有線上清單時與以前相同：各版只有說明。
        plain = site_manifest.build(feed, self.site_uploads(feed), changelog, "t")
        self.assertEqual(plain["changelog"], changelog)
        # backfill 只補線上清單沒有的版本；只有 archive 的舊版照列。
        filled = site_manifest.build(feed, self.site_uploads(feed), changelog, "t",
                                     backfill={"0.1.0-dev.38": self.old_platforms(setup=False)})
        self.assertEqual(filled["changelog"][1]["platforms"], {"windows-amd64": {"archive": self.old_platforms()["windows-amd64"]["archive"]}})
        self.assertEqual(site_manifest.build(feed, self.site_uploads(feed), changelog, "t", history=history,
                                             backfill={"0.1.0-dev.38": {}})["changelog"][1]["platforms"], self.old_platforms())

    def test_site_manifest_drops_withdrawn_past_downloads(self):
        feed = self.site_feed(withdrawn=["0.1.0-dev.38"])
        changelog = site_manifest.parse_changelog(self.CHANGELOG)
        history = {"version": "0.1.0-dev.38", "platforms": self.old_platforms(), "changelog": []}
        site = site_manifest.build(feed, self.site_uploads(feed), changelog, "t", history=history)
        self.assertNotIn("platforms", site["changelog"][1])
        self.assertEqual(site["changelog"][1]["items"], ["上一版。"])  # 說明照常保留

    def test_site_manifest_rejects_bad_past_downloads(self):
        feed = self.site_feed()
        changelog = site_manifest.parse_changelog(self.CHANGELOG)
        def bad(edit):
            platforms = self.old_platforms()
            edit(platforms)
            with self.assertRaises(ValueError):
                site_manifest.build(feed, self.site_uploads(feed), changelog, "t", backfill={"0.1.0-dev.38": platforms})
        setup = lambda p: p["windows-amd64"]["installer"]
        bad(lambda p: setup(p).update(url=setup(p)["url"].replace("dev.38", "dev.37")))
        bad(lambda p: setup(p).update(url=setup(p)["url"].replace("https", "http")))
        bad(lambda p: p["windows-amd64"].update(installer=p["windows-amd64"]["archive"]))  # 角色與檔名不符
        bad(lambda p: setup(p).update(size=0))
        bad(lambda p: setup(p).update(size=True))
        bad(lambda p: setup(p).update(sha256="A" * 64))
        bad(lambda p: p.update({"linux-amd64": p["windows-amd64"]}))  # 只收 Windows
        bad(lambda p: p.update({"windows-amd64": {}}))
        bad(lambda p: p["windows-amd64"].update(extra=setup(p)))

    def test_repository_changelog_has_public_summaries(self):
        entries = site_manifest.parse_changelog((Path(__file__).resolve().parents[1] / "CHANGELOG.md").read_text(encoding="utf-8"))
        self.assertIn("0.1.0-dev.18", [e["version"] for e in entries])
        self.assertNotIn("0.1.0-dev.17", [e["version"] for e in entries])

    def test_release_docs_are_explicit_and_copied_without_rewriting(self):
        with self.assertRaisesRegex(ValueError, "--release-docs"):
            packager.check_release_docs(None)
        external = self.root / "external-docs"
        external.mkdir()
        stage = self.root / "documented-package"
        stage.mkdir()
        for name in packager.RELEASE_DOC_FILES:
            (external / name).write_bytes(("# " + name + "\r\nExternal package instructions.\r\n").encode())
        packager.copy_release_docs(external, stage)
        for name in packager.RELEASE_DOC_FILES:
            self.assertEqual((external / name).read_bytes(), (stage / name).read_bytes())
        for name in packager.RELEASE_DOC_FILES:
            with self.subTest(name=name):
                original = (external / name).read_bytes()
                (external / name).write_text("   ")
                with self.assertRaisesRegex(ValueError, "不可為空"):
                    packager.check_release_docs(external)
                (external / name).unlink()
                with self.assertRaises((ValueError, OSError)):
                    packager.check_release_docs(external)
                (external / name).write_bytes(original)

    def test_windows_build_requires_external_docs_before_building(self):
        data, _, _, _ = self.write_paired_data(self.root / "external-docs-gate", public=True)
        out = self.root / "output"
        args = SimpleNamespace(version="0.2.0", preview=False, data=data, out=out, release_docs=None)
        with patch.object(package_windows.sys, "platform", "win32"), \
                patch.object(package_windows.platform, "machine", return_value="AMD64"), \
                patch.object(subprocess, "run") as run, patch.object(subprocess, "check_output") as output:
            with self.assertRaisesRegex(ValueError, "--release-docs"):
                package_windows.build(args)
        run.assert_not_called()
        output.assert_not_called()
        self.assertFalse(out.exists())

    def test_update_feed_preserves_hashes_and_rejects_mixed_versions(self):
        import zipfile
        def archive(name, version, os_name="windows"):
            file = self.root / (name + ".zip")
            release = {"schema": 1, "product": "0c", "version": version,
                       "channel": "internal-preview", "os": os_name, "arch": "amd64"}
            with zipfile.ZipFile(file, "w") as package:
                package.writestr("bundle/release.json", json.dumps(release))
            return file
        windows = archive("windows", "0.1.0")
        feed = release_feed.make_feed([(windows, "https://example.org/windows.zip")])
        self.assertEqual(feed["assets"], [{"os": "windows", "arch": "amd64", "url": "https://example.org/windows.zip",
                                           "sha256": packager.digest(windows), "size": windows.stat().st_size}])
        newer = archive("newer", "0.2.0")
        with self.assertRaisesRegex(ValueError, "版本"):
            release_feed.make_feed([(windows, "https://example.org/windows.zip"), (newer, "https://example.org/newer.zip")])
        with self.assertRaisesRegex(ValueError, "重複"):
            release_feed.make_feed([(windows, "https://example.org/windows.zip")] * 2)
        with self.assertRaisesRegex(ValueError, "HTTPS"):
            release_feed.make_feed([(windows, "http://example.org/windows.zip")])
        with self.assertRaisesRegex(ValueError, "平台"):
            release_feed.make_feed([(archive("linux", "0.1.0", "linux"), "https://example.org/linux.zip")])

    def test_public_data_rejects_incomplete_or_private_provenance(self):
        data, _, baseline, _ = self.write_paired_data(self.root / "data-gate", public=True)
        cases = []
        for value in (None, {}, {"Public": False}, {"Public": 1}, {"Public": "true"}):
            cases.append(("options", value))
        for value in (None, [], {}, [None]):
            cases.append(("sources", value))
        for value in (None, False, 0, 1, "yes", "true"):
            cases.append(("Distributable", value))
        for field in ("ID", "Name", "URL", "Version", "License", "Attribution"):
            for value in (None, "", "-", " unknown ", "未定", 123, "line\nbreak"):
                cases.append((field, value))
        cases += [("License", "私有"), ("ID", packager.PRIVATE_RESEARCH_SOURCE),
                  ("ID", "../fixture"), ("research_news_mix", {}),
                  ("sources", baseline["sources"] * 2)]
        for field, value in cases:
            with self.subTest(field=field, value=value):
                manifest = json.loads(json.dumps(baseline))
                target = manifest if field in ("options", "sources", "research_news_mix") else manifest["sources"][0]
                if value is None:
                    target.pop(field, None)
                else:
                    target[field] = value
                (data / "manifest.json").write_bytes(packager.json_bytes(manifest))
                before = self.data_snapshot(data)
                with self.assertRaises(ValueError):
                    packager.prepare_shipped_data(data)
                self.assertEqual(self.data_snapshot(data), before)
        (data / "manifest.json").write_bytes(packager.json_bytes(baseline))
        (data / "lm.bin").write_bytes(b"tampered")
        for public in (False, True):
            with self.assertRaisesRegex(ValueError, "雜湊"):
                packager.check_data(data, public)

    def test_public_notice_requires_matching_sources_metadata_and_license_statements(self):
        data, _, manifest, _ = self.write_paired_data(self.root / "notice-gate", public=True)
        notice = (data / "NOTICE.txt").read_text(encoding="utf-8")
        cases = ["", "資料包\n", notice + notice, notice.replace("（fixture）", "（other）"),
                 notice.replace("測試詞條", "other"), notice.replace("授權：MIT", "授權：私有"),
                 notice.replace("版本：1.0", "版本：2.0"), notice.replace("https://example.org/fixture", "other"),
                 notice.replace("姓名標示：Fixture authors", "姓名標示：other"),
                 notice[:notice.index("Permission")], notice + "研究來源：{}\n"]
        for broken in cases:
            with self.subTest(notice=broken):
                (data / "NOTICE.txt").write_text(broken, encoding="utf-8")
                before = self.data_snapshot(data)
                with self.assertRaises(ValueError):
                    packager.prepare_shipped_data(data)
                self.assertEqual(self.data_snapshot(data), before)
        (data / "NOTICE.txt").write_bytes(notice.replace("\n", "\r\n").encode())
        packager.check_data(data, True)

    def test_removing_legacy_source_markers_does_not_make_data_public(self):
        data, _, manifest, _ = self.write_paired_data(self.root / "stripped")
        # Reproduce old packaging: private sources and the nested marker disappear,
        # while the binary dictionary and mixed statistics remain untouched.
        manifest["sources"] = manifest["sources"][:1]
        del manifest["research_news_mix"]["source"]
        (data / "NOTICE.txt").write_text(self.source_notice(manifest["sources"]), encoding="utf-8")
        for public in (False, True):
            manifest["options"]["Public"] = public
            (data / "manifest.json").write_bytes(packager.json_bytes(manifest))
            before = self.data_snapshot(data)
            with self.assertRaisesRegex(ValueError, "重新編譯"):
                packager.prepare_shipped_data(data)
            self.assertEqual(self.data_snapshot(data), before)

    def test_windows_build_rejects_private_data_before_building(self):
        data, _, manifest, _ = self.write_paired_data(self.root / "platform-gate", public=True)
        manifest["sources"][0]["Distributable"] = False
        (data / "manifest.json").write_bytes(packager.json_bytes(manifest))
        before = self.data_snapshot(data)
        out = self.root / "output"
        args = SimpleNamespace(version="0.2.0", preview=False, data=data, out=out)
        with patch.object(package_windows.sys, "platform", "win32"), \
                patch.object(package_windows.platform, "machine", return_value="AMD64"), \
                patch.object(subprocess, "run") as run, patch.object(subprocess, "check_output") as output:
            with self.assertRaisesRegex(ValueError, "不可散布"):
                package_windows.build(args)
        run.assert_not_called()
        output.assert_not_called()
        self.assertFalse(out.exists())
        self.assertEqual(self.data_snapshot(data), before)

    def test_windows_neural_bundles_cpp_runtime_next_to_python(self):
        # #80：onnxruntime 需要 MSVCP140.dll／MSVCP140_1.dll，沒裝 Visual C++ 可轉散發套件的 Windows 載入失敗。
        import collect_notices
        import prepare_neural
        import zipfile
        url, sha256, names = prepare_neural.RUNTIME["crt"]
        self.assertEqual(names, package_windows.NEURAL_CRT)
        self.assertTrue(url.endswith("-cp314-cp314-win_amd64.whl"))
        with tempfile.TemporaryDirectory() as temp:
            temp = Path(temp)
            wheel = temp / "msvc_runtime.whl"
            with zipfile.ZipFile(wheel, "w") as z:
                for name in ("msvcp140.dll", "msvcp140_1.dll", "concrt140.dll"):
                    z.writestr("msvc_runtime-14.44.35112.data/data/" + name, name)
                    z.writestr("msvc_runtime-14.44.35112.data/data/Scripts/" + name, "scripts " + name)
            python = temp / "neural/python"
            (python / "Lib/site-packages").mkdir(parents=True)
            prepare_neural.install_crt(wheel, names, python)
            self.assertEqual(sorted(p.name for p in python.glob("*.dll")), ["msvcp140.dll", "msvcp140_1.dll"])
            self.assertEqual((python / "msvcp140.dll").read_text(), "msvcp140.dll")
            (python / "LICENSE.txt").write_text("PSF")
            text = "\n".join(collect_notices.python_sections(temp / "neural"))
            self.assertIn("Microsoft Visual C++ 執行階段（msvcp140.dll、msvcp140_1.dll）", text)
            with zipfile.ZipFile(wheel, "a") as z:
                z.writestr("other.data/data/msvcp140.dll", "duplicate")
            with self.assertRaisesRegex(ValueError, "msvcp140.dll"):
                prepare_neural.install_crt(wheel, names, python)

    def test_bundle_name_is_not_rebound_while_packaging(self):
        # dev.60 前 #80 的迴圈 `for name in NEURAL_CRT` 蓋掉了套件名稱，Windows ZIP 的頂層目錄變成 msvcp140_1.dll/。
        import ast
        tree = ast.parse(Path(package_windows.__file__).read_text(encoding="utf-8"))
        build = next(f for f in tree.body if isinstance(f, ast.FunctionDef) and f.name == "build")
        binds = [n.lineno for n in ast.walk(build) if isinstance(n, ast.Name) and n.id == "name" and isinstance(n.ctx, ast.Store)]
        self.assertLessEqual(len(binds), 1, f"package_windows.build() 重新指派了套件名稱 name：第 {binds} 行")

    def test_settings_ui_script_parses(self):
        # dev.60 前曾重複宣告 const REASON，整支設定程式畫面載入失敗；node 可用時檢查語法。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        for name in ("app.js", "models.js", "vocab.js", "vocab-text.js", "personal-reset.js"):
            app = Path(__file__).resolve().parents[1] / "platforms/settings/ui" / name
            result = subprocess.run([node, "--check", str(app)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, name + "：" + result.stderr)

    def test_settings_ui_engine_state(self):
        # 總覽「輸入法仍未全部切換」的判斷：核心更新後不誤報、外殼等重新登入時要提示（0.2.1 候選版冒煙）。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        test = Path(__file__).resolve().parents[1] / "platforms/settings/test/engine_state_test.js"
        result = subprocess.run([node, "--test", str(test)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_settings_ui_personal_reset(self):
        # #301「我的詞彙」頁「靈犀學到的」底下的清除學習紀錄：確認文案、0c-update personal reset -json 結果的提示，成功時才通知清單重讀。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        test = Path(__file__).resolve().parents[1] / "platforms/settings/test/personal_reset_test.js"
        result = subprocess.run([node, "--test", str(test)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_settings_ui_vocab_text(self):
        # #255「我的詞彙」匯出與匯入的文字格式：匯出再匯入詞彙相同（破音字讀音、英文詞、含換行與 Tab 的片語），
        # 貼上「詞＋Tab＋讀音」不再把讀音切成另一行，合併只加不刪，超過 500 個分批。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        test = Path(__file__).resolve().parents[1] / "platforms/settings/test/vocab_text_test.js"
        result = subprocess.run([node, "--test", str(test)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_settings_ui_models(self):
        # #180「模型」頁：內建只顯示資訊、可下載／下載中／已安裝／重新下載各自的按鈕、缺推論元件與網址白名單。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        test = Path(__file__).resolve().parents[1] / "platforms/settings/test/models_test.js"
        result = subprocess.run([node, "--test", str(test)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_settings_ui_update_mode(self):
        # #244 更新方式的名稱與說明：Windows 的「自動」要照實說外殼更新會詢問，值 auto／ask／manual 不變。
        node = shutil.which("node")
        if not node:
            self.skipTest("沒有 node")
        test = Path(__file__).resolve().parents[1] / "platforms/settings/test/update_mode_test.js"
        result = subprocess.run([node, "--test", str(test)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_pairing_lock_matches_0c_ime_pair_models(self):
        # #180：發版產生的 pairing.lock 與 0c-ime 以 scripts/pair_models.py 產生的逐位元組相同（兩份不同的字典）。
        import pairing_lock
        testdata = Path(__file__).resolve().parent / "testdata"
        for name in ("0c-ime-dfsg-pairing.lock.json", "0c-ime-0.2.3-pairing.lock.json"):
            expected = (testdata / name).read_bytes()
            dictionary = json.loads(expected)["dictionary_manifest_sha256"]
            self.assertEqual(pairing_lock.pairing_lock(dictionary), expected, name)


class HFModelsTests(unittest.TestCase):
    """#180：官網套件內建 Hugging Face 公開版模型（scripts/hf_models.py），0.2.2 以前的內部匯出視為等價。"""

    OLD = {"select": "cef689f1e706537e54eaed4241a776f1ea41b4cad2b504061d1e27cd0c1ca342",
           "punct": "2d98c7c6c75dc9d46081e967dbd2fccef67f73a1ed9b2da44d7e02002e760cf5"}

    def setUp(self):
        import hf_models
        import pairing_lock
        self.hf, self.pairing = hf_models, pairing_lock
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        # 小的假權重：models.lock 只換 model.onnx、LICENSE、README.md 的雜湊；manifest.json、vocab.json 仍是上游原檔。
        lock = json.loads(hf_models.LOCK.read_bytes())
        self.weights = self.root / "hf"
        for role in hf_models.ROLES:
            for name in ("model.onnx", "LICENSE", "README.md"):
                content = f"{role} {name}\n".encode() * 100
                (self.weights / role).mkdir(parents=True, exist_ok=True)
                (self.weights / role / name).write_bytes(content)
                lock["models"][role]["files"][name] = {"sha256": packager.digest(self.weights / role / name), "size": len(content)}
        self.lock = self.root / "models.lock.json"
        self.lock.write_text(json.dumps(lock, indent=2) + "\n", encoding="utf-8")
        self.data = self.root / "data"
        self.data.mkdir()
        (self.data / "manifest.json").write_bytes(b'{"format": 2, "dictionary": "release"}\n')

    def tearDown(self):
        self.temp.cleanup()

    def test_identity_accepts_hugging_face_and_old_shipped_hashes(self):
        lock = json.loads(self.hf.LOCK.read_bytes())
        for role in self.hf.ROLES:
            self.assertEqual(self.hf.model_identity(role, lock["models"][role]["files"]["model.onnx"]["sha256"]), "upstream")
            self.assertIsNone(self.hf.model_identity(role, "c" * 64))
        # 0.2.12 select 換成 d512-cv2-20261111：0.2.2 以前與 0.2.3–0.2.11 內建的 select 都不再是上游，punct 照舊
        self.assertEqual(self.hf.model_identity("punct", self.OLD["punct"]), "equivalent")
        self.assertIsNone(self.hf.model_identity("select", self.OLD["select"]))
        self.assertIsNone(self.hf.model_identity("select", "71af1f0e77b2752cc71cbc04bf19c8e8f2e765662246f766e65a2822bab4e66e"))
        self.assertIsNone(self.hf.model_identity("punct", self.OLD["select"]))  # 不跨角色
        # models.lock 換了 revision：舊的等價紀錄不再適用
        lock["models"]["punct"]["revision"] = "e" * 40
        bumped = self.root / "bumped.lock.json"
        bumped.write_text(json.dumps(lock), encoding="utf-8")
        self.assertIsNone(self.hf.model_identity("punct", self.OLD["punct"], bumped))
        # 等價清單與 Go 編進 0c-update 的是同一份，記的上游與 models.lock 一致
        equivalent = json.loads(self.hf.EQUIVALENT.read_text(encoding="utf-8"))
        real = json.loads(self.hf.LOCK.read_bytes())["models"]
        for role, item in equivalent["models"].items():
            self.assertEqual((item["repository"], item["revision"], item["model.onnx"]),
                             (real[role]["repository"], real[role]["revision"], real[role]["files"]["model.onnx"]["sha256"]))

    def test_fetch_verifies_source_and_caches_downloads(self):
        weights = self.hf.fetch(self.weights, lock_path=self.lock)
        self.assertEqual(sorted(weights["select"]), ["LICENSE", "README.md", "model.onnx"])
        (self.weights / "punct/model.onnx").write_bytes(b"tampered")
        with self.assertRaisesRegex(ValueError, "punct/model.onnx 與 models.lock 不符"):
            self.hf.fetch(self.weights, lock_path=self.lock)
        (self.weights / "punct/model.onnx").write_bytes("punct model.onnx\n".encode() * 100)
        # 沒有 --from：從鏡像（Hugging Face 的 <repo>/resolve/<revision>/<檔名>）下載進快取，第二次不再下載
        lock = json.loads(self.lock.read_bytes())
        urls = []

        def fake_download(url, target):
            urls.append(url)
            role = "select" if "/0c-sequence-d512/" in url else "punct"
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(self.weights / role / url.rsplit("/", 1)[1], target)

        with patch.object(self.hf, "download", fake_download):
            cache = self.root / "cache"
            got = self.hf.fetch(cache=cache, base_url="https://mirror.test/", lock_path=self.lock)
            self.assertEqual(len(urls), 6)
            self.assertIn(f"https://mirror.test/Funmula/0c-sequence-d512/resolve/{lock['models']['select']['revision']}/model.onnx", urls)
            self.assertEqual(got["punct"]["model.onnx"], cache / "punct" / lock["models"]["punct"]["revision"] / "model.onnx")
            self.hf.fetch(cache=cache, base_url="https://mirror.test", lock_path=self.lock)
            self.assertEqual(len(urls), 6)

    def test_replace_models_pairs_public_models_and_matches_pairing_lock(self):
        # 0.2.2 形狀的 neural/：可攜 Python、worker、內部匯出的模型（含 export-validation.json）
        old = self.root / "old-neural"
        for path in ("python/bin/python3", "sequence_runtime.py", "punct_runtime.py", "README.txt",
                     "select/model.onnx", "select/manifest.json", "select/vocab.json", "select/export-validation.json",
                     "punct/model.onnx", "punct/manifest.json", "punct/vocab.json", "pairing.lock.json"):
            (old / path).parent.mkdir(parents=True, exist_ok=True)
            (old / path).write_text("old " + path)
        weights = self.hf.fetch(self.weights, lock_path=self.lock)
        out = self.hf.replace_models(old, self.data, self.root / "new/neural", weights, self.lock)
        self.assertEqual((out / "python/bin/python3").read_text(), "old python/bin/python3")
        # worker 不沿用舊 neural/ 的：一律是 repo 的版本（例如關掉 onnxruntime 遙測的修正才會出貨）
        for name in self.hf.WORKER_NAMES:
            self.assertEqual((out / name).read_bytes(), (self.hf.WORKERS / name).read_bytes(), name)
        self.assertFalse((out / "pairing.lock.json").exists())
        self.assertEqual(sorted(p.name for p in (out / "select").iterdir()), ["LICENSE", "README.md", "manifest.json", "model.onnx", "vocab.json"])
        dictionary = packager.digest(self.data / "manifest.json")
        expected = json.loads(self.pairing.pairing_lock(dictionary, self.lock))
        for role in self.hf.ROLES:
            self.assertEqual((out / role / "model.onnx").read_bytes(), (self.weights / role / "model.onnx").read_bytes())
            self.assertEqual(json.loads((out / role / "vocab.json").read_bytes())["dictionary_sha256"], dictionary)
            for name in ("manifest.json", "vocab.json"):
                self.assertEqual(packager.digest(out / role / name), expected["packages"][role][name]["sha256"], f"{role}/{name}")
        self.assertEqual(self.hf.check_shipped(out, self.data, self.lock), self.pairing.pairing_lock(dictionary, self.lock))
        with self.assertRaisesRegex(ValueError, "輸出已存在"):
            self.hf.replace_models(old, self.data, out, weights, self.lock)
        # 內部匯出的 neural/（manifest 不是上游的）不能當公開包出貨
        with self.assertRaisesRegex(ValueError, "不是公開版模型配對出貨字典的結果"):
            self.hf.check_shipped(old, self.data, self.lock)
        # worker 跟 repo 不同（沿用舊版或手改）不能出貨
        for name in self.hf.WORKER_NAMES:
            original = (out / name).read_bytes()
            (out / name).write_bytes(original + b"\n")
            with self.assertRaisesRegex(ValueError, f"neural/{name} 與 platforms/neural/runtime/{name} 不同"):
                self.hf.check_shipped(out, self.data, self.lock)
            (out / name).write_bytes(original)
        # 只換掉權重（例如沿用舊的 model.onnx）也擋下
        shutil.copyfile(old / "select/model.onnx", out / "select/model.onnx")
        with self.assertRaisesRegex(ValueError, "官網套件要內建 Hugging Face 公開版"):
            self.hf.check_shipped(out, self.data, self.lock)

    def test_shipped_repairing_keeps_the_pairing_lock_consistent(self):
        # 打包時 prepare_data_manifest 拿掉不公開來源、把模型改配對到出貨 manifest：結果仍等於出貨字典的 pairing.lock。
        news = {"ID": "zuin-news-stratified-20260921", "Name": "新聞", "Distributable": False}
        (self.data / "manifest.json").write_bytes(packager.json_bytes(
            {"format": 2, "sources": [{"ID": "zhwiki", "Distributable": True}, news]}))
        weights = self.hf.fetch(self.weights, lock_path=self.lock)
        neural = self.root / "neural"
        neural.mkdir()
        self.hf.write_packages(weights, packager.digest(self.data / "manifest.json"), neural, self.lock)
        for name in self.hf.WORKER_NAMES:
            shutil.copyfile(self.hf.WORKERS / name, neural / name)
        shipped = packager.prepare_data_manifest(self.data, neural)
        self.assertEqual(shipped, packager.digest(self.data / "manifest.json"))
        self.assertEqual(json.loads(self.hf.check_shipped(neural, self.data, self.lock))["dictionary_manifest_sha256"], shipped)

    def test_missing_in_vocab_lists_unknown_characters(self):
        # #172 的 18 個人名字都在公開版模型的 vocab 裡；沒有的字逐一列出（兩個模型各一）
        self.assertEqual(self.hf.missing_in_vocab(list("凃姸晗峯鳯伃珢瀞㵢彣烱瓈慜諪琹坣佀啓")), [])
        self.assertEqual(self.hf.missing_in_vocab(["𠀀"]), ["select:𠀀", "punct:𠀀"])

    def test_prepare_neural_takes_models_from_the_lock(self):
        import prepare_neural
        import sys
        parser_help = subprocess.run([sys.executable, "-B", str(Path(prepare_neural.__file__)), "--help"],
                                     capture_output=True, text=True).stdout
        self.assertIn("--models", parser_help)
        self.assertNotIn("--select", parser_help)


if __name__ == "__main__":
    unittest.main()
