#!/usr/bin/env python3

"""开发模式: 同时启动 Vite 开发服务器与 Go 后端.

后端使用隔离的 .tmp/scherun-dev/ 数据目录, 并关闭自动打开浏览器;
浏览器里访问 Vite 的地址即可, /api 请求会被代理到后端.
任一进程退出时另一个也一并停止, Ctrl+C 会转发给两者.
"""

import argparse
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

from output_encoding import configure_output_encoding

ROOT = Path(__file__).resolve().parent.parent
DEV_DIR = ROOT / ".tmp" / "scherun-dev"


def main() -> int:
    configure_output_encoding()
    parser = argparse.ArgumentParser(description="启动开发环境")
    parser.add_argument("--port", type=int, default=20008, help="后端监听端口")
    args = parser.parse_args()

    DEV_DIR.mkdir(parents=True, exist_ok=True)

    env = os.environ.copy()
    env["SCHERUN_DATA_DIR"] = str(DEV_DIR / "data")
    env["SCHERUN_CONFIG_DIR"] = str(DEV_DIR / "config")
    env["SCHERUN_LOG_FILE"] = str(DEV_DIR / "data" / "logs" / "scherun.log")
    env["SCHERUN_LOG_LEVEL"] = "debug"

    binary = DEV_DIR / "scherun"
    print("构建后端...")
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

    processes: list[subprocess.Popen] = []

    def shutdown(*_: object) -> None:
        for process in processes:
            if process.poll() is None:
                process.terminate()
        for process in processes:
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()

    signal.signal(signal.SIGINT, lambda *_: (shutdown(), sys.exit(0)))
    signal.signal(signal.SIGTERM, lambda *_: (shutdown(), sys.exit(0)))

    print(f"启动后端 (端口 {args.port})")
    processes.append(
        subprocess.Popen(
            [
                str(binary),
                "--no-browser",
                "--bind",
                f"127.0.0.1:{args.port}",
            ],
            cwd=ROOT,
            env=env,
        )
    )

    # 等后端端口就绪再起前端, 避免代理先于服务.
    time.sleep(1.5)

    print("启动前端开发服务器 http://127.0.0.1:5173")
    processes.append(
        subprocess.Popen(
            ["bun", "run", "dev"],
            cwd=ROOT / "web",
            env=env,
        )
    )

    # 任一进程退出就整体收尾.
    while True:
        for process in processes:
            code = process.poll()
            if code is not None:
                print(f"子进程退出 (code {code}), 停止其余进程")
                shutdown()
                return code
        time.sleep(0.5)


if __name__ == "__main__":
    sys.exit(main())
