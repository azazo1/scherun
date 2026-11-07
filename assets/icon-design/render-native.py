#!/usr/bin/env python3
"""绘制带瓷白底座与层叠任务卡片的 macOS 图标候选."""

from pathlib import Path
import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageFont

OUT = Path(__file__).resolve().parent
S = 2
N = 1024 * S
Y, X = np.mgrid[0:N, 0:N].astype(float) / S


def rounded(box, radius):
    mask = Image.new("L", (N, N))
    ImageDraw.Draw(mask).rounded_rectangle(tuple(int(v*S) for v in box), radius=int(radius*S), fill=255)
    return mask


def gradient(mask, top, bottom, box, glow=0):
    t = np.clip((Y-box[1])/(box[3]-box[1]), 0, 1)[..., None]
    pixels = np.array(top)*(1-t) + np.array(bottom)*t
    if glow:
        light = np.exp(-((X-box[0]-90)**2+(Y-box[1])**2)/(2*230**2))[..., None]
        pixels = np.clip(pixels + glow*light, 0, 255)
    layer = Image.fromarray(pixels.astype("uint8"), "RGB").convert("RGBA")
    layer.putalpha(mask)
    return layer


def shadow(canvas, mask, blur, dy, opacity):
    shifted = Image.new("L", (N, N))
    shifted.paste(mask, (0, round(dy*S)))
    shifted = shifted.filter(ImageFilter.GaussianBlur(blur*S))
    shifted = shifted.point(lambda v: round(v*opacity))
    layer = Image.new("RGBA", (N, N), (18, 35, 65, 0))
    layer.putalpha(shifted)
    canvas.alpha_composite(layer)


def stroke(canvas, box, radius, fill, width=2):
    ImageDraw.Draw(canvas).rounded_rectangle(tuple(round(v*S) for v in box), radius=round(radius*S), outline=fill, width=round(width*S))


def pill(canvas, box, color):
    ImageDraw.Draw(canvas).rounded_rectangle(tuple(round(v*S) for v in box), radius=round((box[3]-box[1])*S/2), fill=color)


def main():
    im = Image.new("RGBA", (N, N))
    board = (76, 72, 948, 944)
    mask = rounded(board, 196)
    shadow(im, mask, 13, 13, .20)
    im.alpha_composite(gradient(mask, (255, 255, 255), (214, 225, 241), board))
    stroke(im, (79, 75, 945, 941), 193, (255, 255, 255, 255), 3)
    stroke(im, (86, 82, 938, 934), 187, (237, 243, 251, 255), 2)

    # 三张实体卡片形成递进层次, 阴影在透明图层上合成.
    layers = [
        ((286, 230, 738, 536), 51, (160, 204, 249), (89, 140, 212)),
        ((248, 319, 776, 634), 57, (95, 167, 247), (39, 102, 205)),
        ((208, 417, 816, 766), 66, (47, 133, 240), (26, 65, 160)),
    ]
    for i, (box, r, top, bottom) in enumerate(layers):
        m = rounded(box, r)
        shadow(im, m, 17 if i == 2 else 11, 17, .29)
        im.alpha_composite(gradient(m, top, bottom, box, glow=12))
        # 薄亮边和深色底缘提供厚度, 不使用过重的装饰高光.
        edge = Image.new("RGBA", (N, N))
        stroke(edge, (box[0]+1, box[1]+1, box[2]-1, box[3]-1), r-1, (255, 255, 255, 100), 2)
        im.alpha_composite(edge)
        if i < 2:
            pill(im, (box[0]+45, box[1]+32, box[0]+59, box[1]+46), (239, 249, 255, 255))
            pill(im, (box[0]+78, box[1]+34, box[0]+210, box[1]+44), (221, 237, 255, 255))

    # 前景卡片只保留少量大笔画, 避免缩小后变成密集文字.
    pill(im, (269, 490, 292, 513), (131, 246, 218, 255))
    pill(im, (318, 492, 493, 510), (195, 224, 255, 255))
    pill(im, (269, 562, 623, 601), (247, 251, 255, 255))
    pill(im, (269, 634, 489, 661), (159, 197, 244, 255))

    # 独立运行按钮采用明亮薄荷色与深色凹入符号.
    button = (628, 638, 802, 812)
    bm = rounded(button, 87)
    shadow(im, bm, 14, 10, .30)
    im.alpha_composite(gradient(bm, (173, 255, 229), (63, 203, 165), button, glow=9))
    edge = Image.new("RGBA", (N, N))
    stroke(edge, (630, 640, 800, 810), 85, (231, 255, 246, 205), 3)
    im.alpha_composite(edge)
    symbol = Image.new("RGBA", (N, N))
    d = ImageDraw.Draw(symbol)
    triangle = [(697, 690), (697, 760), (750, 725)]
    d.polygon([(x*S,y*S) for x,y in triangle], fill=(18, 100, 88, 255))
    d.line([(x*S,y*S) for x,y in triangle+[triangle[0]]], fill=(18, 100, 88, 255), width=8*S, joint="curve")
    for x,y in triangle:
        d.ellipse(((x-4)*S,(y-4)*S,(x+4)*S,(y+4)*S), fill=(18,100,88,255))
    im.alpha_composite(symbol)
    icon = im.resize((1024, 1024), Image.Resampling.LANCZOS)
    icon.save(OUT / 'native-stack-1024.png')

    # 浅色与深色环境, 并附实际像素尺寸展示.
    sheet = Image.new("RGB", (1400, 920), "#f1f3f7")
    d = ImageDraw.Draw(sheet)
    font = lambda n: ImageFont.load_default(size=n)
    d.text((52, 35), "scherun / NATIVE STACK", font=font(31), fill="#24344a")
    d.text((52, 80), "Porcelain base. Layered tasks. One active run.", font=font(20), fill="#718096")
    d.rounded_rectangle((40, 132, 680, 752), radius=28, fill="#e4e9f0")
    d.rounded_rectangle((700, 132, 1360, 752), radius=28, fill="#172332")
    for x in (105, 775):
        small = icon.resize((510, 510), Image.Resampling.LANCZOS)
        sheet.paste(small, (x, 180), small)
    for size, x in [(16, 80), (32, 180), (64, 300), (128, 470)]:
        small = icon.resize((size, size), Image.Resampling.LANCZOS)
        sheet.paste(small, (x, 780+(128-size)//2), small)
        d.text((x+size+12, 830), str(size), font=font(16), fill="#718096")
    d.text((750, 813), "1024 px / transparent PNG", font=font(23), fill="#24344a")
    d.text((750, 850), "Optical size aligned with macOS icon grid", font=font(18), fill="#718096")
    sheet.save(OUT / 'native-preview.png')
    assert icon.mode == 'RGBA' and icon.size == (1024, 1024)
    assert icon.getpixel((0, 0))[3] == 0
    print("已生成原生质感候选, 透明边缘和图像尺寸验证通过.")


if __name__ == '__main__':
    main()
