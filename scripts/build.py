#!/usr/bin/env python3

"""构建 scherun 二进制.

版本号按 git tag 推导: 正好在 tag 上就是 tag, 否则追加短 hash,
工作区有未提交改动时用 ^ 连接. 仓库没有 tag 时回退到 0.0.0 加短 hash.

跨平台实现, justfile 只负责调用它.
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path

from output_encoding import configure_output_encoding

ROOT = Path(__file__).resolve().parent.parent
PACKAGE = "./cmd/scherun"
MODULE = "github.com/azazo1/scherun"


def run(args: list[str], **kwargs) -> str:
    """执行命令并返回去掉首尾空白的 stdout."""
    result = subprocess.run(args, cwd=ROOT, capture_output=True, text=True, **kwargs)
    if result.returncode != 0:
        raise RuntimeError(f"命令失败: {' '.join(args)}\n{result.stderr.strip()}")
    return result.stdout.strip()


def try_run(args: list[str]) -> str | None:
    """执行命令, 失败时返回 None."""
    try:
        return run(args)
    except (RuntimeError, FileNotFoundError):
        return None


def git(*args: str) -> str | None:
    """执行 git 命令, 失败时返回 None."""
    return try_run(["git", *args])


def build_version() -> tuple[str, str]:
    """返回 (显示版本, 提交 hash)."""
    commit = git("rev-parse", "--short=7", "HEAD") or "unknown"

    tag = git("tag", "--points-at", "HEAD")
    if not tag:
        tag = git("describe", "--tags", "--abbrev=0")

    if not tag:
        # 仓库还没有任何 tag, 用 0.0.0 兜底.
        base = "v0.0.0"
        exact = False
    else:
        first = tag.splitlines()[0].strip()
        base = first if first.startswith("v") else f"v{first}"
        exact = bool(git("tag", "--points-at", "HEAD"))

    if commit == "unknown":
        return base, commit

    dirty = False
    status = try_run(["git", "status", "--porcelain"])
    if status:
        dirty = True

    if exact and not dirty:
        return base, commit
    separator = "^" if dirty else "-"
    return f"{base}{separator}{commit}", commit


def main() -> int:
    configure_output_encoding()
    parser = argparse.ArgumentParser(description="构建 scherun")
    parser.add_argument("--output", default="dist/scherun", help="输出路径")
    parser.add_argument("--version", default=None, help="直接指定版本号")
    parser.add_argument("--fake", action="store_true", help="构建 v0.0.0 的测试版本")
    parser.add_argument("--tags", default="", help="传给 go build 的构建标签")
    parser.add_argument("--debug", action="store_true", help="调试构建, 保留控制台")
    args = parser.parse_args()

    if args.fake:
        display_version, commit = "v0.0.0", "fake"
    elif args.version:
        display_version, commit = args.version, "unknown"
    else:
        display_version, commit = build_version()

    output = ROOT / args.output
    output.parent.mkdir(parents=True, exist_ok=True)

    ldflags = " ".join(
        [
            "-s",
            "-w",
            f"-X {MODULE}/internal/version.Version={display_version}",
            f"-X {MODULE}/internal/version.Commit={commit}",
        ]
    )

    env = os.environ.copy()
    # 托盘实现需要 cgo: macOS 的 systray 只有 Objective-C 实现.
    env.setdefault("CGO_ENABLED", "1")

    flags = ldflags
    # Windows 的 release 构建声明为 GUI 子系统, 避免闪出控制台窗口;
    # debug 构建 (带构建标签或显式 --debug) 保留控制台, 保证终端里 Ctrl+C 可用.
    if env.get("GOOS") == "windows" and not args.tags and not args.debug:
        flags = "-H=windowsgui " + flags

    command = [
        "go",
        "build",
        "-trimpath",
        "-buildvcs=false",
        "-ldflags",
        flags,
    ]
    if args.tags:
        command.extend(["-tags", args.tags])
    command.extend(["-o", str(output), PACKAGE])

    print(f"构建 {display_version} (commit {commit}) -> {output.relative_to(ROOT)}")
    subprocess.run(command, cwd=ROOT, env=env, check=True)

    # 校验注入结果, 挡住 -X 路径写错导致的静默失效.
    reported = try_run([str(output), "--version"])
    if reported and display_version not in reported:
        print(f"版本校验失败: 期望 {display_version}, 实际 {reported}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
