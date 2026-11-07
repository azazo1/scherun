#!/usr/bin/env python3

"""生成当前平台的发布产物.

需要同时存在 scripts/archive.py, scripts/build-version.py 与 scripts/release_common.py.

各平台产物形态不同:

    macOS    scherun.app 应用包, 再封装成 dmg (内含指向 /Applications 的符号链接)
    Windows  Inno Setup 单文件安装器, 默认 per-user 安装, 不请求提权
    Linux    可执行文件的 tar.gz 归档

托盘依赖 cgo (macOS 的 systray 只有 Objective-C 实现), 因此不做交叉编译,
每个平台在自己的 runner 上原生构建. 脚本内部按平台分支, 调用方不需要区分.

用法:
    uv run --no-project python scripts/dist.py
"""

import os
import shutil
import stat
import sys
from pathlib import Path

import release_common as rc

PROJECT_NAME = "scherun"
APP_NAME = "scherun"
MAIN_PACKAGE = "./cmd/scherun"
BINARY_NAME = "scherun"
# Go 没有包版本可读, 仓库还没有版本 tag 时用占位版本兜底.
PACKAGE_VERSION = "0.0.0"
# 冒烟检查用的参数, 要求二进制能报出版本号.
SMOKE_ARGS = ["--version"]

# macOS 应用包标识, 与 appdirs 里使用的目录名保持一致.
BUNDLE_ID = "com.azazo1.scherun"
# 应用图标与安装器脚本.
ICON_ICNS = "assets/icon.icns"
ICON_ICO = "assets/icon.ico"
INNO_SCRIPT = "packaging/windows/scherun.iss"

# 构建前端产物后再编译 Go: 前端走 go:embed, 缺了它界面是空的.
WEB_DIR = "web"
EMBED_SCRIPT = "scripts/embed_web.py"

# 目录布局.
DIST = "dist"
STAGE_ROOT = os.path.join(DIST, "stage")


def host_target():
    """取当前构建机真实的 GOOS 与 GOARCH.

    不能用 go env: 它会读取 GOOS / GOARCH 环境变量, 于是设了交叉目标之后
    "宿主平台" 也会跟着变成目标平台, 冒烟检查就会去执行跑不起来的产物.
    这里从 go env 的默认值文件里取, 并清掉这两个变量后再问一次.
    """
    env = {k: v for k, v in os.environ.items() if k not in ("GOOS", "GOARCH")}
    goos = rc.try_run(["go", "env", "GOOS"], env=env) or ""
    goarch = rc.try_run(["go", "env", "GOARCH"], env=env) or ""
    return goos, goarch


def build_target():
    """返回本次构建的 (GOOS, GOARCH).

    默认就是宿主平台. 设了 GOOS / GOARCH 时按它们构建, 产物命名也跟着走,
    否则会出现 "名字是 linux 内容是 darwin" 的错配产物.
    注意: 托盘依赖 cgo, 交叉出来的产物在目标平台跑不起来, 正式发布请用原生 runner.
    """
    host_goos, host_goarch = host_target()
    goos = os.environ.get("GOOS") or host_goos
    goarch = os.environ.get("GOARCH") or host_goarch
    return goos, goarch, (goos, goarch) == (host_goos, host_goarch)


def goos_to_platform(goos):
    """GOOS 到产物命名平台 token 的映射."""
    mapping = {"darwin": "macos", "linux": "linux", "windows": "windows"}
    if goos not in mapping:
        rc.die(f"无法识别目标平台 {goos}, 请检查 GOOS")
    return mapping[goos]


def goarch_to_arch(goarch):
    """GOARCH 到产物命名架构 token 的映射."""
    mapping = {"amd64": "x86_64", "arm64": "aarch64"}
    if goarch not in mapping:
        rc.die(f"无法识别目标架构 {goarch}, 请检查 GOARCH")
    return mapping[goarch]


def main():
    rc.configure_output_encoding()
    rc.repo_root()

    display_version, archive_version = rc.resolve_versions(
        rc.script_path("build-version.py"), PACKAGE_VERSION
    )
    goos, goarch, is_host = build_target()
    target_platform = goos_to_platform(goos)
    target_arch = goarch_to_arch(goarch)

    print(f"构建 {PROJECT_NAME} {display_version} ({target_platform}-{target_arch})")

    build_web()
    staging = rc.stage_dir(os.path.join(STAGE_ROOT, f"{target_platform}-{target_arch}"))
    binary = build_binary(staging, display_version, goos, goarch, is_host)

    if target_platform == "macos":
        package_macos(staging, binary, display_version, archive_version, target_arch)
    elif target_platform == "windows":
        package_windows(staging, binary, display_version, archive_version, target_arch)
    elif target_platform == "linux":
        package_linux(staging, binary, display_version, archive_version, target_arch)
    else:
        rc.die(f"暂不支持的平台: {target_platform}")

    print("完成")


def build_web():
    """构建前端产物并同步进 go:embed 目录.

    前端走 go:embed, 缺了它界面是空的, 所以发布构建必须自己重建一次,
    不能依赖工作区里可能过期的 web/dist.
    """
    print("构建前端产物")
    rc.run(["bun", "install", "--frozen-lockfile"], cwd=WEB_DIR, capture=False)
    rc.run(["bun", "run", "build"], cwd=WEB_DIR, capture=False)
    rc.run([sys.executable, EMBED_SCRIPT])


def build_binary(staging, display_version, goos, goarch, is_host):
    """编译 Go 二进制, 注入版本号.

    注入路径用 go list -m 取模块名, 不要手写, 改模块名时会静默失效.
    托盘需要 cgo, 因此这里不设 CGO_ENABLED=0: 正式发布必须在目标平台原生构建,
    设了 GOOS / GOARCH 只会得到跑不起来的产物.
    """
    module = rc.run(["go", "list", "-m"])
    binary_name = BINARY_NAME + (".exe" if goos == "windows" else "")
    binary = os.path.join(staging, binary_name)

    ldflags = [
        "-s",
        "-w",
        f"-X {module}/internal/version.Version={display_version}",
    ]
    # Windows release 构建声明为 GUI 子系统, 避免双击时闪出控制台窗口;
    # 日志仍写入数据目录下的文件, 不受影响.
    if goos == "windows":
        ldflags.insert(0, "-H=windowsgui")

    env = os.environ.copy()
    env["GOOS"] = goos
    env["GOARCH"] = goarch

    rc.run(
        [
            "go",
            "build",
            "-trimpath",
            "-buildvcs=false",
            "-ldflags",
            " ".join(ldflags),
            "-o",
            binary,
            MAIN_PACKAGE,
        ],
        env=env,
        capture=False,
    )

    # 注入失效时 -X 不报错, 必须校验一次.
    if not is_host:
        # 非宿主平台的产物跑不起来, 退化为文件格式, 架构与版本串检查.
        target_platform = goos_to_platform(goos)
        rc.check_binary_format(binary, target_platform)
        rc.check_binary_arch(binary, target_platform, goarch_to_arch(goarch))
        rc.check_version_string(binary, display_version)
    elif goos == "windows":
        # GUI 子系统程序没有可读的标准输出, 直接运行既拿不到版本号, 又可能让 CI 等不到退出,
        # 改为在产物字节里核对注入的版本串.
        rc.check_version_string(binary, display_version)
    else:
        rc.smoke_check([binary], SMOKE_ARGS, display_version)
    return binary


def package_macos(staging, binary, display_version, archive_version, target_arch):
    """组装 .app 应用包并封装成 dmg.

    布局遵循 macOS 惯例: 可执行文件放 Contents/MacOS, Info.plist 与图标放 Contents/Resources.
    """
    app_name = f"{APP_NAME}.app"
    app_dir = os.path.join(staging, app_name)
    macos_dir = os.path.join(app_dir, "Contents", "MacOS")
    resources_dir = os.path.join(app_dir, "Contents", "Resources")
    os.makedirs(macos_dir, exist_ok=True)
    os.makedirs(resources_dir, exist_ok=True)

    installed_binary = os.path.join(macos_dir, BINARY_NAME)
    shutil.move(binary, installed_binary)
    os.chmod(installed_binary, 0o755)

    shutil.copy(ICON_ICNS, os.path.join(resources_dir, "icon.icns"))
    write_info_plist(
        os.path.join(app_dir, "Contents", "Info.plist"), display_version, archive_version
    )

    # 应用包结构自检: 主程序可执行, Info.plist 与图标都在位.
    if not os.access(installed_binary, os.X_OK):
        rc.die(f"应用包主程序不可执行: {installed_binary}")
    for required in ("Contents/Info.plist", "Contents/Resources/icon.icns"):
        if not os.path.isfile(os.path.join(app_dir, required)):
            rc.die(f"应用包缺少 {required}")

    make_dmg(staging, app_name, archive_version)


def write_info_plist(path, display_version, archive_version):
    """写 Info.plist.

    CFBundleShortVersionString 只接受纯数字三段式, 所以用归档版本号 (已剥掉 v 前缀);
    构建版本 (可能带短 hash) 放进 CFBundleVersion 之外的字段, 供 About 与排查使用.
    """
    short_version = numeric_version(archive_version)
    plist = f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>zh_CN</string>
    <key>CFBundleDisplayName</key>
    <string>{APP_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>{BINARY_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>{BUNDLE_ID}</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>{APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>{short_version}</string>
    <key>CFBundleVersion</key>
    <string>{short_version}</string>
    <key>ScherunBuildVersion</key>
    <string>{display_version}</string>
    <key>CFBundleIconFile</key>
    <string>icon</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
"""
    Path(path).write_text(plist, encoding="utf-8")


def numeric_version(version):
    """把版本号规整成纯数字三段式, 供 CFBundleShortVersionString 使用.

    1.2.3 / 1.2.3-a1b2c3d / 1.2.3^a1b2c3d 都取 1.2.3; 位数不足时补 0.
    """
    base = version.split("-")[0].split("^")[0]
    parts = [part for part in base.split(".") if part.isdigit()]
    while len(parts) < 3:
        parts.append("0")
    return ".".join(parts[:3])


def make_dmg(staging, app_name, archive_version):
    """用 hdiutil 制作 dmg, 内含应用与指向 /Applications 的符号链接.

    hdiutil 只在 macOS 上有, 而这一步本来就只在 macOS runner 上执行.
    """
    if not shutil.which("hdiutil"):
        rc.die("找不到 hdiutil, 无法制作 dmg")

    # dmg 内容目录: 应用 + /Applications 快捷方式, 用户拖过去即可安装.
    dmg_root = os.path.join(staging, "dmg-root")
    os.makedirs(dmg_root, exist_ok=True)
    shutil.move(os.path.join(staging, app_name), os.path.join(dmg_root, app_name))
    link = os.path.join(dmg_root, "Applications")
    if not os.path.islink(link):
        os.symlink("/Applications", link)

    dmg_path = os.path.join(DIST, f"{PROJECT_NAME}-{archive_version}-macos-{rc.target_arch()}.dmg")
    if os.path.exists(dmg_path):
        os.remove(dmg_path)

    rc.run(
        [
            "hdiutil",
            "create",
            "-volname",
            APP_NAME,
            "-srcfolder",
            dmg_root,
            "-ov",
            "-format",
            "UDZO",
            dmg_path,
        ],
        capture=False,
    )

    # 校验 dmg 结构与内容: 挂载后确认应用与符号链接都在.
    verify_dmg(dmg_path, app_name)
    print(f"已生成 {dmg_path}")


def verify_dmg(dmg_path, app_name):
    """挂载 dmg 检查内容, 再卸载."""
    mount_point = rc.run(
        ["hdiutil", "attach", "-nobrowse", "-readonly", "-plist", dmg_path]
    )
    # 用 plist 输出里最后一个 mount-point 作为挂载点.
    mount = None
    for line in mount_point.splitlines():
        line = line.strip()
        if line.startswith("<string>/Volumes/"):
            mount = line[len("<string>") : -len("</string>")]
    if not mount:
        rc.die(f"无法确定 dmg 挂载点: {dmg_path}")

    try:
        entries = set(os.listdir(mount))
        if app_name not in entries:
            rc.die(f"dmg 里没有 {app_name}: {sorted(entries)}")
        if not os.path.islink(os.path.join(mount, "Applications")):
            rc.die("dmg 里缺少指向 /Applications 的符号链接")
        executable = os.path.join(mount, app_name, "Contents", "MacOS", BINARY_NAME)
        if not os.access(executable, os.X_OK):
            rc.die(f"dmg 里的主程序不可执行: {executable}")
        print("dmg 结构检查通过")
    finally:
        rc.try_run(["hdiutil", "detach", mount, "-quiet"])


def package_windows(staging, binary, display_version, archive_version, target_arch):
    """用 Inno Setup 打包安装器.

    Inno Setup 只在 Windows 上有 (或通过 wine), 这一步只在 windows runner 上执行.
    """
    compiler = find_inno_setup()
    setup_path = os.path.join(DIST, f"{PROJECT_NAME}-{archive_version}-windows-{target_arch}-setup.exe")
    if os.path.exists(setup_path):
        os.remove(setup_path)

    define_version = numeric_version(archive_version)
    rc.run(
        [
            compiler,
            "/Qp",
            f"/DAppVersion={define_version}",
            f"/DAppBuildVersion={archive_version}",
            f"/DSourceBinary={os.path.abspath(binary)}",
            f"/DSourceIcon={os.path.abspath(ICON_ICO)}",
            f"/DOutputDir={os.path.abspath(DIST)}",
            f"/DOutputBaseFilename={os.path.splitext(os.path.basename(setup_path))[0]}",
            os.path.abspath(INNO_SCRIPT),
        ],
        capture=False,
    )

    if not os.path.isfile(setup_path):
        rc.die(f"安装器没有生成: {setup_path}")

    # 校验: PE 文件头 + 架构, 与 Linux/macOS 的产物检查对齐.
    rc.check_binary_format(setup_path, "windows")
    rc.check_binary_arch(setup_path, "windows", target_arch)
    print(f"已生成 {setup_path}")


def find_inno_setup():
    """定位 ISCC.exe.

    优先用环境变量, 其次查 PATH, 最后查常见安装位置.
    """
    override = os.environ.get("INNO_SETUP_ISCC", "").strip()
    if override:
        if not os.path.isfile(override):
            rc.die(f"INNO_SETUP_ISCC 指向的文件不存在: {override}")
        return override

    found = shutil.which("ISCC.exe") or shutil.which("iscc")
    if found:
        return found

    candidates = [
        os.path.join(os.environ.get("PROGRAMFILES(X86)", ""), "Inno Setup 6", "ISCC.exe"),
        os.path.join(os.environ.get("PROGRAMFILES", ""), "Inno Setup 6", "ISCC.exe"),
        os.path.join(os.environ.get("LOCALAPPDATA", ""), "Programs", "Inno Setup 6", "ISCC.exe"),
    ]
    for candidate in candidates:
        if candidate and os.path.isfile(candidate):
            return candidate

    rc.die("找不到 Inno Setup 的 ISCC.exe, 请安装 Inno Setup 6 或设置 INNO_SETUP_ISCC 指向它")


def package_linux(staging, binary, display_version, archive_version, target_arch):
    """Linux 出可执行文件的 tar.gz 归档, 附带许可证与说明."""
    os.chmod(binary, os.stat(binary).st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    rc.check_binary_format(binary, "linux")
    rc.check_binary_arch(binary, "linux", target_arch)

    entries = [os.path.basename(binary)]
    for extra in ("LICENSE", "README.md"):
        if os.path.isfile(extra):
            shutil.copy(extra, os.path.join(staging, extra))
            entries.append(extra)

    rc.archive(
        staging,
        entries,
        PROJECT_NAME,
        archive_version,
        target=("linux", target_arch),
    )


if __name__ == "__main__":
    main()
