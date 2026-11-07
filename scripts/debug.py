#!/usr/bin/env python3

"""开发辅助: 用隔离的数据目录启动调试实例.

数据目录与日志文件都放在项目内的 .tmp/scherun-debug/ 下, 不触碰日常数据;
日志级别提到 debug. 因为单实例锁按数据目录绑定, 调试实例可以与正式实例并行运行.

用法:
    uv run --no-project python scripts/debug.py [--no-browser]
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path

from output_encoding import configure_output_encoding

ROOT = Path(__file__).resolve().parent.parent
DEBUG_DIR = ROOT / ".tmp" / "scherun-debug"


def main() -> int:
    configure_output_encoding()
    parser = argparse.ArgumentParser(description="启动隔离的调试实例")
    parser.add_argument("--no-browser", action="store_true", help="不自动打开浏览器")
    parser.add_argument("--build-only", action="store_true", help="只构建不启动")
    args = parser.parse_args()

    DEBUG_DIR.mkdir(parents=True, exist_ok=True)

    env = os.environ.copy()
    env["SCHERUN_DATA_DIR"] = str(DEBUG_DIR / "data")
    env["SCHERUN_CONFIG_DIR"] = str(DEBUG_DIR / "config")
    env["SCHERUN_LOG_FILE"] = str(DEBUG_DIR / "data" / "logs" / "scherun.log")
    env["SCHERUN_LOG_LEVEL"] = "debug"

    binary = DEBUG_DIR / "scherun"
    print(f"构建调试实例 -> {binary.relative_to(ROOT)}")
    subprocess.run(
        [
            sys.executable,
            str(ROOT / "scripts" / "build.py"),
            "--output",
            str(binary),
            "--debug",
        ],
        cwd=ROOT,
        env=env,
        check=True,
    )
    if args.build_only:
        return 0

    command = [str(binary)]
    if args.no_browser:
        command.append("--no-browser")

    print(f"启动调试实例, 数据目录: {(DEBUG_DIR / 'data').relative_to(ROOT)}")
    # 前台运行, 让 Ctrl+C 直接作用在子进程上.
    return subprocess.run(command, cwd=ROOT, env=env).returncode


if __name__ == "__main__":
    sys.exit(main())
