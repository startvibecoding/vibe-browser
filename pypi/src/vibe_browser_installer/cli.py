import os
import subprocess
import sys
from pathlib import Path


def _binary_name() -> str:
    if sys.platform.startswith("win"):
        return "vibe-browser.exe"
    return "vibe-browser"


def _binary_path() -> Path:
    return Path(__file__).resolve().parent / "bin" / _binary_name()


def main() -> int:
    binary = _binary_path()
    if not binary.is_file():
        sys.stderr.write("vibe-browser binary is missing from this PyPI installation.\n")
        sys.stderr.write("Reinstall with: pip install --force-reinstall vibe-browser-installer\n")
        return 1

    args = [str(binary), *sys.argv[1:]]
    if not sys.platform.startswith("win"):
        try:
            os.execv(str(binary), args)
        except OSError as exc:
            sys.stderr.write(f"Failed to execute vibe-browser binary: {exc}\n")
            return 1
        return 1

    return subprocess.call(args)
