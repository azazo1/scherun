#!/usr/bin/env python3

"""从设计源图生成各平台的应用图标资源.

源图是 assets/icon-design/native-stack-1024.png (设计稿与生成脚本也在那个目录).
产物落在 assets/ 下, 是提交进仓库的静态资源, 打包脚本只读不生成:

    assets/icon.icns        macOS 应用图标 (dmg 与 .app 使用)
    assets/icon.ico         Windows 应用图标 (exe 与安装器使用)
    assets/icon-1024.png    当前图标的 1024 位图副本, 供其他平台取用

只用标准库: 读 PNG 自己做缩放与编码, 因此任何机器上都能重新生成.
缩放用盒式滤波做降采样, 小尺寸图标不会因为跳过像素而抖动.
"""

import struct
import sys
import zlib
from pathlib import Path

from output_encoding import configure_output_encoding

ROOT = Path(__file__).resolve().parent.parent
ASSETS = ROOT / "assets"
SOURCE = ASSETS / "icon-design" / "native-stack-1024.png"

# icns 与 ico 需要的小图标尺寸.
ICNS_SIZES = [16, 32, 64, 128, 256, 512, 1024]
ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]


def read_png(path):
    """读取 PNG, 返回 (像素字节 RGBA, 宽, 高).

    只支持 8 位 RGBA 与 RGB, 且必须无隔行, 够覆盖设计稿导出的图.
    """
    data = Path(path).read_bytes()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError(f"不是 PNG: {path}")

    pos = 8
    width = height = None
    bit_depth = color_type = None
    idat = bytearray()
    while pos < len(data):
        length = struct.unpack(">I", data[pos : pos + 4])[0]
        tag = data[pos + 4 : pos + 8]
        payload = data[pos + 8 : pos + 8 + length]
        pos += 12 + length
        if tag == b"IHDR":
            width, height, bit_depth, color_type, _, _, interlace = struct.unpack(
                ">IIBBBBB", payload
            )
            if interlace:
                raise ValueError("不支持隔行 PNG")
        elif tag == b"IDAT":
            idat.extend(payload)
        elif tag == b"IEND":
            break

    if bit_depth != 8 or color_type not in (2, 6):
        raise ValueError(f"只支持 8 位 RGB/RGBA PNG, 实际 bit_depth={bit_depth} color_type={color_type}")

    channels = 4 if color_type == 6 else 3
    raw = zlib.decompress(bytes(idat))
    stride = width * channels
    out = bytearray(width * height * 4)
    previous = bytearray(stride)

    for row in range(height):
        filter_type = raw[row * (stride + 1)]
        line = bytearray(raw[row * (stride + 1) + 1 : (row + 1) * (stride + 1)])
        unfilter(line, previous, filter_type, channels)
        previous = line
        for col in range(width):
            src = col * channels
            dst = (row * width + col) * 4
            out[dst] = line[src]
            out[dst + 1] = line[src + 1]
            out[dst + 2] = line[src + 2]
            out[dst + 3] = line[src + 3] if channels == 4 else 255
    return bytes(out), width, height


def unfilter(line, previous, filter_type, channels):
    """按 PNG 规范还原一行像素 (就地修改)."""
    if filter_type == 0:
        return
    for i in range(len(line)):
        left = line[i - channels] if i >= channels else 0
        up = previous[i]
        up_left = previous[i - channels] if i >= channels else 0
        if filter_type == 1:
            line[i] = (line[i] + left) & 0xFF
        elif filter_type == 2:
            line[i] = (line[i] + up) & 0xFF
        elif filter_type == 3:
            line[i] = (line[i] + ((left + up) >> 1)) & 0xFF
        elif filter_type == 4:
            line[i] = (line[i] + paeth(left, up, up_left)) & 0xFF
        else:
            raise ValueError(f"未知的行过滤类型: {filter_type}")


def paeth(a, b, c):
    p = a + b - c
    pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
    if pa <= pb and pa <= pc:
        return a
    if pb <= pc:
        return b
    return c


def resize(pixels, width, height, size):
    """用盒式滤波把图像缩到 size x size, 带 alpha 加权避免边缘发黑."""
    out = bytearray(size * size * 4)
    scale_x = width / size
    scale_y = height / size

    for dst_y in range(size):
        y0 = int(dst_y * scale_y)
        y1 = max(y0 + 1, int((dst_y + 1) * scale_y))
        for dst_x in range(size):
            x0 = int(dst_x * scale_x)
            x1 = max(x0 + 1, int((dst_x + 1) * scale_x))

            total_alpha = 0.0
            accum_r = accum_g = accum_b = 0.0
            count = 0
            for sy in range(y0, min(y1, height)):
                for sx in range(x0, min(x1, width)):
                    idx = (sy * width + sx) * 4
                    alpha = pixels[idx + 3]
                    # 预乘 alpha 再平均, 否则透明像素的黑色会渗进边缘.
                    accum_r += pixels[idx] * alpha
                    accum_g += pixels[idx + 1] * alpha
                    accum_b += pixels[idx + 2] * alpha
                    total_alpha += alpha
                    count += 1

            dst = (dst_y * size + dst_x) * 4
            if total_alpha > 0 and count > 0:
                out[dst] = int(accum_r / total_alpha + 0.5)
                out[dst + 1] = int(accum_g / total_alpha + 0.5)
                out[dst + 2] = int(accum_b / total_alpha + 0.5)
                out[dst + 3] = int(total_alpha / count + 0.5)
            # total_alpha 为 0 时保持全透明.
    return bytes(out)


def png_chunk(tag, data):
    """构造一个 PNG chunk."""
    return (
        struct.pack(">I", len(data))
        + tag
        + data
        + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    )


def encode_png(pixels, size):
    """把 RGBA 像素编码成 PNG 字节."""
    raw = bytearray()
    stride = size * 4
    for y in range(size):
        raw.append(0)  # 每行用 filter type 0.
        raw.extend(pixels[y * stride : (y + 1) * stride])

    header = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    return (
        b"\x89PNG\r\n\x1a\n"
        + png_chunk(b"IHDR", header)
        + png_chunk(b"IDAT", zlib.compress(bytes(raw), 9))
        + png_chunk(b"IEND", b"")
    )


def write_icns(pngs):
    """把各尺寸 PNG 组装成 icns.

    icns 是 4 字节类型 + 4 字节长度 (含头部) 的条目序列, 各尺寸对应固定类型码.
    """
    type_by_size = {
        16: b"icp4",
        32: b"icp5",
        64: b"icp6",
        128: b"ic07",
        256: b"ic08",
        512: b"ic09",
        1024: b"ic10",
    }
    body = bytearray()
    for size, data in pngs.items():
        body.extend(type_by_size[size])
        body.extend(struct.pack(">I", len(data) + 8))
        body.extend(data)
    return b"icns" + struct.pack(">I", len(body) + 8) + bytes(body)


def write_ico(pngs):
    """把各尺寸 PNG 组装成 ico, 条目全用 PNG 压缩形式."""
    entries = sorted(pngs.items())
    header = struct.pack("<HHH", 0, 1, len(entries))

    offset = len(header) + 16 * len(entries)
    directory = bytearray()
    payload = bytearray()
    for size, data in entries:
        # 256 尺寸在目录里记作 0.
        dimension = 0 if size >= 256 else size
        directory.extend(
            struct.pack("<BBBBHHII", dimension, dimension, 0, 0, 1, 32, len(data), offset)
        )
        payload.extend(data)
        offset += len(data)
    return header + bytes(directory) + bytes(payload)


def main() -> int:
    configure_output_encoding()
    if not SOURCE.is_file():
        print(f"找不到设计源图: {SOURCE}", file=sys.stderr)
        return 1

    pixels, width, height = read_png(SOURCE)
    print(f"读取源图 {width}x{height}")
    if width != height:
        print(f"源图必须是正方形, 实际 {width}x{height}", file=sys.stderr)
        return 1

    ASSETS.mkdir(parents=True, exist_ok=True)

    # 1024 直接复用源图, 不缩放, 避免一次无谓的重采样.
    icns_pngs = {}
    for size in ICNS_SIZES:
        if size == width:
            icns_pngs[size] = encode_png(pixels, size)
        else:
            icns_pngs[size] = encode_png(resize(pixels, width, height, size), size)
        print(f"  缩放 {size}x{size}")
    (ASSETS / "icon.icns").write_bytes(write_icns(icns_pngs))
    print(f"已生成 assets/icon.icns ({len(ICNS_SIZES)} 个尺寸)")

    ico_pngs = {size: icns_pngs[size] for size in ICO_SIZES if size in icns_pngs}
    for size in ICO_SIZES:
        if size not in ico_pngs:
            ico_pngs[size] = encode_png(resize(pixels, width, height, size), size)
    (ASSETS / "icon.ico").write_bytes(write_ico(ico_pngs))
    print(f"已生成 assets/icon.ico ({len(ICO_SIZES)} 个尺寸)")

    (ASSETS / "icon-1024.png").write_bytes(icns_pngs[1024])
    print("已生成 assets/icon-1024.png")
    return 0


if __name__ == "__main__":
    sys.exit(main())
