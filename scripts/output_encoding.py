#!/usr/bin/env python3

"""脚本共用的输出编码配置.

单独成文件是为了让不依赖 release_common 的脚本 (例如 embed_web.py, make_icons.py)
也能用同一份实现, 而不是各自抄一遍.

Windows 控制台默认是本地代码页 (例如 cp1252), 直接 print 中文会抛
UnicodeEncodeError; CI 的 Windows runner 上已经因此失败过一次.
"""

import sys


def configure_output_encoding() -> None:
    """把 stdout 与 stderr 切到 UTF-8, 失败时静默忽略.

    不是所有流都支持 reconfigure (例如被重定向成纯文本句柄时),
    这种场合保持原样即可, 不该因为改不了编码就让脚本退出.
    """
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8")
        except (AttributeError, ValueError, OSError):
            pass
