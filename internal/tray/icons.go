//go:build !notray

package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"
)

// 托盘图标尺寸.
//
// macOS 菜单栏图标用 22pt 高, 这里按 2x 出 44 像素, 系统会自己缩放;
// Windows 与 Linux 的托盘区域偏小, 用 32 像素.
const (
	iconSizeMac = 44
	iconSize    = 32
)

var (
	iconMu    sync.Mutex
	iconCache = map[string][]byte{}
)

// iconPNG 生成彩色状态图标, 供 Windows 与 Linux 使用.
//
// 造型取自应用图标的卡片层叠语义: 深色圆角底板上三张叠放的卡片,
// 右上角叠加状态色点. 运行中为绿色, 有挂起为橙色, 空闲为灰色.
func iconPNG(kind string) []byte {
	return cachedIcon("color/"+kind, func() []byte {
		img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))

		// 底板: 深色圆角方块.
		base := color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff}
		fillRoundedRect(img, image.Rect(2, 2, iconSize-2, iconSize-2), 7, base)

		// 三张叠放的卡片, 用深浅区分层次.
		back := color.NRGBA{R: 0x3f, G: 0x76, B: 0xc4, A: 0xff}
		mid := color.NRGBA{R: 0x6f, G: 0xa8, B: 0xf2, A: 0xff}
		front := color.NRGBA{R: 0xdc, G: 0xe7, B: 0xf5, A: 0xff}
		fillRoundedRect(img, image.Rect(9, 6, 23, 12), 2, back)
		fillRoundedRect(img, image.Rect(7, 11, 25, 18), 2, mid)
		fillRoundedRect(img, image.Rect(5, 16, 27, 26), 3, front)

		// 前景卡片上的三个点: 表示"还有更多条目"的列表修饰, 不用三角,
		// 否则小尺寸下容易被看成播放器.
		drawDots(img, 16, 21, 3, 1, color.NRGBA{R: 0x24, G: 0x55, B: 0xb4, A: 0xff})

		// 状态色点.
		drawStatusDot(img, statusColor(kind))
		return encodePNG(img)
	})
}

// templateIconPNG 生成 macOS 模板图标.
//
// 模板图标必须是纯黑加 alpha 的单色图, 系统按菜单栏明暗自动反色,
// 因此这里不能带任何彩色: 卡片层叠用不透明度区分层次, 状态信息交给菜单文本.
func templateIconPNG() []byte {
	return cachedIcon("template", func() []byte {
		img := image.NewNRGBA(image.Rect(0, 0, iconSizeMac, iconSizeMac))

		// 单色前景, alpha 表达层次: 后面的卡片淡, 前面的卡片实.
		back := color.NRGBA{A: 0x8c}
		mid := color.NRGBA{A: 0xc4}
		front := color.NRGBA{A: 0xff}

		// 三张卡片, 位置与彩色版同比例放大到 44 像素.
		fillRoundedRect(img, image.Rect(13, 9, 31, 19), 3, back)
		fillRoundedRect(img, image.Rect(10, 16, 34, 27), 3, mid)
		fillRoundedRect(img, image.Rect(8, 23, 36, 37), 4, front)

		// 前景卡片上挖出三个点, 用透明表示镂空.
		// 前景卡片范围 x 8..36, 中心约 22, 三点围绕中心排布.
		drawDotsTransparent(img, 17, 30, 3, 1)
		return encodePNG(img)
	})
}

// cachedIcon 缓存生成的 PNG, 避免每次状态刷新都重新绘制.
func cachedIcon(key string, draw func() []byte) []byte {
	iconMu.Lock()
	defer iconMu.Unlock()
	if cached, ok := iconCache[key]; ok {
		return cached
	}
	data := draw()
	iconCache[key] = data
	return data
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// statusColor 把状态类别映射成颜色.
func statusColor(kind string) color.NRGBA {
	switch kind {
	case "active":
		return color.NRGBA{R: 0x34, G: 0xd3, B: 0x99, A: 0xff}
	case "warning":
		return color.NRGBA{R: 0xf5, G: 0xa5, B: 0x24, A: 0xff}
	case "idle":
		return color.NRGBA{R: 0x94, G: 0xa3, B: 0xb8, A: 0xff}
	default:
		return color.NRGBA{R: 0x64, G: 0x74, B: 0x8b, A: 0xff}
	}
}

// drawStatusDot 在右上角画状态圆点.
func drawStatusDot(img *image.NRGBA, c color.NRGBA) {
	const radius = 4
	centerX, centerY := iconSize-5, 5
	for y := centerY - radius; y <= centerY+radius; y++ {
		for x := centerX - radius; x <= centerX+radius; x++ {
			dx := x - centerX
			dy := y - centerY
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}
}

// drawDots 画一排实心圆点, 用作列表"更多条目"的修饰.
//
// (x, y) 是第一个点的圆心, count 是点数, radius 是半径, gap 是点间距.
func drawDots(img *image.NRGBA, x, y, count, radius int, c color.NRGBA) {
	for i := 0; i < count; i++ {
		centerX := x + i*(radius*2+2)
		fillCircle(img, centerX, y, radius, c)
	}
}

// drawDotsTransparent 在已绘制区域上挖出一排圆点.
func drawDotsTransparent(img *image.NRGBA, x, y, count, radius int) {
	for i := 0; i < count; i++ {
		centerX := x + i*(radius*2+3)
		fillCircle(img, centerX, y, radius, color.NRGBA{})
	}
}

// fillCircle 画一个实心圆.
func fillCircle(img *image.NRGBA, centerX, centerY, radius int, c color.NRGBA) {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy <= radius*radius {
				img.Set(centerX+dx, centerY+dy, c)
			}
		}
	}
}

// fillRoundedRect 画一个圆角矩形.
func fillRoundedRect(img *image.NRGBA, rect image.Rectangle, radius int, c color.NRGBA) {
	draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Src)
	if radius <= 0 {
		return
	}
	// 把四个角切圆: 逐角清除圆外的像素.
	for _, corner := range []struct{ cx, cy, sx, sy int }{
		{rect.Min.X + radius, rect.Min.Y + radius, -1, -1},
		{rect.Max.X - radius - 1, rect.Min.Y + radius, 1, -1},
		{rect.Min.X + radius, rect.Max.Y - radius - 1, -1, 1},
		{rect.Max.X - radius - 1, rect.Max.Y - radius - 1, 1, 1},
	} {
		for dy := 0; dy <= radius; dy++ {
			for dx := 0; dx <= radius; dx++ {
				if dx*dx+dy*dy > radius*radius {
					img.Set(corner.cx+corner.sx*dx, corner.cy+corner.sy*dy, color.NRGBA{})
				}
			}
		}
	}
}
