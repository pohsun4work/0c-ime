#!/usr/bin/env python3
"""Run the official session tests under the blend input style and check every difference is expected.

core/session/testdata/blend_expected_diffs.txt lists the tests that should differ and the blend rules
(json field names) behind them. Everything else must pass; the listed tests must fail, and pass again once
their rules are turned off (BLENDALL_OFF), so the list cannot hide a regression from another rule.
"""

from concurrent.futures import ThreadPoolExecutor
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parent.parent
LIST = ROOT / "core" / "session" / "testdata" / "blend_expected_diffs.txt"


def read_expected(text):
    expected = {}
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        name, rest = line.split(None, 1)
        fields = rest.split("：", 1)[0]
        expected[name] = tuple(sorted(fields.split(",")))
    return expected


def pattern(names):
    return "^(" + "|".join(sorted(names)) + ")$"


def failed_tests(binary, run=None, skip=None, off=()):
    """Failing top-level tests. A panic also stops the tests after it, so listed tests run one by one."""
    args = [binary, "-test.count=1", "-test.v"]
    if run:
        args += ["-test.run", pattern(run)]
    if skip:
        args += ["-test.skip", pattern(skip)]
    env = dict(os.environ, BLENDALL_OFF=",".join(off))
    out = subprocess.run(args, cwd=ROOT / "core" / "session", env=env, capture_output=True, text=True,
                         encoding="utf-8", errors="replace")
    failed = set(re.findall(r"^--- FAIL: (\S+)", out.stdout, re.M))
    if out.returncode and not failed:
        raise SystemExit("test binary failed without a failing test:\n" + out.stdout + out.stderr)
    return failed


def main():
    expected = read_expected(LIST.read_text(encoding="utf-8"))
    with tempfile.TemporaryDirectory() as tmp:
        binary = str(Path(tmp) / "session.test.exe")
        subprocess.run(["go", "test", "-c", "-tags", "blendall", "-o", binary, "./core/session"], cwd=ROOT, check=True)
        return check(binary, expected)


def check(binary, expected):
    with ThreadPoolExecutor() as pool:
        unexpected = pool.submit(failed_tests, binary, skip=expected)
        listed = {name: (pool.submit(failed_tests, binary, run=[name]),
                         pool.submit(failed_tests, binary, run=[name], off=fields))
                  for name, fields in expected.items()}
    problems = []
    if unexpected := unexpected.result():
        problems.append("official tests that differ but are not listed:\n  " + "\n  ".join(sorted(unexpected)))
    if passing := sorted(name for name, (on, _) in listed.items() if not on.result()):
        problems.append("listed tests that no longer differ (remove them):\n  " + "\n  ".join(passing))
    if still := sorted(name for name, (_, off) in listed.items() if off.result()):
        problems.append("listed tests that still fail with their rules off:\n  " +
                        "\n  ".join(f"{name} ({','.join(expected[name])})" for name in still))
    if problems:
        print("\n".join(problems))
        return 1
    print(f"ok: {len(expected)} expected differences, everything else passes")
    return 0


if __name__ == "__main__":
    sys.exit(main())
