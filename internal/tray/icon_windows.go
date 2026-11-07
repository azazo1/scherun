//go:build !notray && windows

package tray

import (
	"encoding/binary"

	"fyne.io/systray"
)

// setIcon 在 Windows 上使用彩色图标.
//
// Windows 通知区域不按模板图标处理, 传模板图会得到纯黑图标, 因此直接走 SetIcon.
// 彩色 PNG 要先包成单图 ICO, 否则通知区域不认.
func setIcon(template, regular []byte) {
	systray.SetIcon(platformIcon(regular))
}

// platformIcon 把 PNG 包装成单图 ICO, 供 Windows 通知区域使用.
func platformIcon(pngData []byte) []byte {
	if len(pngData) == 0 {
		return nil
	}
	const headerSize = 6
	const entrySize = 16
	buf := make([]byte, headerSize+entrySize+len(pngData))
	binary.LittleEndian.PutUint16(buf[0:2], 0)                      // reserved
	binary.LittleEndian.PutUint16(buf[2:4], 1)                      // type: icon
	binary.LittleEndian.PutUint16(buf[4:6], 1)                      // count
	buf[6] = iconSize                                               // width
	buf[7] = iconSize                                               // height
	binary.LittleEndian.PutUint16(buf[10:12], 1)                    // planes
	binary.LittleEndian.PutUint16(buf[12:14], 32)                   // bit count
	binary.LittleEndian.PutUint32(buf[14:18], uint32(len(pngData))) // bytes in resource
	binary.LittleEndian.PutUint32(buf[18:22], headerSize+entrySize) // image offset
	copy(buf[headerSize+entrySize:], pngData)
	return buf
}
