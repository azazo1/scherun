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

// iconSize 是托盘图标边长, 32 在 Retina 上足够清晰.
const iconSize = 32

var (
	iconMu    sync.Mutex
	iconCache = map[string][]byte{}
)

// iconPNG 生成状态图标.
//
// 图标是一只圆角方块加一条横向进度条, 右上角叠加状态色点:
// 运行中为绿色, 有挂起为橙色, 空闲为灰色.
func iconPNG(kind string) []byte {
	iconMu.Lock()
	defer iconMu.Unlock()
	if cached, ok := iconCache[kind]; ok {
		return cached
	}

	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))

	// 底板: 深色圆角方块.
	base := color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff}
	fillRoundedRect(img, image.Rect(3, 5, iconSize-3, iconSize-5), 6, base)

	// 中间的 "运行条": 两条横线, 模拟任务列表.
	line := color.NRGBA{R: 0xe8, G: 0xec, B: 0xf2, A: 0xff}
	fillRoundedRect(img, image.Rect(9, 12, 23, 14), 1, line)
	fillRoundedRect(img, image.Rect(9, 17, 19, 19), 1, line)

	// 状态色点.
	drawStatusDot(img, statusColor(kind))

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	data := buf.Bytes()
	iconCache[kind] = data
	return data
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
				// 先描一圈底色让圆点与底板分离.
				img.Set(x, y, c)
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
