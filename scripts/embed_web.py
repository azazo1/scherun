#!/usr/bin/env python3

"""把 web/dist 的前端产物同步到 internal/webui/dist, 供 go:embed 打包.

跨平台实现, 不依赖 shell 的 cp -R.

同步时只替换产物, 保留 .gitkeep: 这个占位文件让 go:embed 在没有构建前端时
依然能编译, 干净克隆后直接 go build 才不会因为空目录失败.
"""

import shutil
import sys
from pathlib import Path

from output_encoding import configure_output_encoding

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "web" / "dist"
TARGET = ROOT / "internal" / "webui" / "dist"

# 版本库里保留的占位文件, 清理目标目录时必须留下.
KEEP = {".gitkeep"}


def main() -> int:
    configure_output_encoding()

    if not SOURCE.is_dir():
        print(f"找不到前端构建产物: {SOURCE}", file=sys.stderr)
        print("请先运行 just web", file=sys.stderr)
        return 1

    TARGET.mkdir(parents=True, exist_ok=True)

    # 清掉上一次的产物, 但保留占位文件.
    for entry in TARGET.iterdir():
        if entry.name in KEEP:
            continue
        if entry.is_dir():
            shutil.rmtree(entry)
        else:
            entry.unlink()

    shutil.copytree(SOURCE, TARGET, dirs_exist_ok=True)
    print(f"已同步前端资源: {TARGET}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
