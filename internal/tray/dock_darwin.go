//go:build darwin && !notray

package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

// hideDockIcon 把应用激活策略设为 Accessory, 从 Dock 隐藏但保留菜单栏图标.
static void hideDockIcon(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
    });
}
*/
import "C"

// HideDockIcon 在 macOS 上隐藏 Dock 图标.
//
// 本应用没有原生窗口, 界面在浏览器里, 因此不该占用 Dock.
func HideDockIcon() {
	C.hideDockIcon()
}
