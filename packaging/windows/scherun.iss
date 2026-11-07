; scherun 的 Windows 安装器脚本 (Inno Setup 6).
;
; 由 scripts/dist.py 调用, 版本号与源文件路径通过 /D 宏传入, 不要在这里写死.
;
; 安装策略:
;   - 默认 per-user 安装到 %LOCALAPPDATA%\Programs\scherun, 全程不请求提权.
;   - AppId 一经发布不得更改, 它决定卸载项与升级是否落到同一份安装记录上.
;   - 安装目录只放程序自有文件; 配置, 数据与日志都在 %LOCALAPPDATA%\scherun, 不在这里.
;   - 升级就是同一安装器覆盖安装, 用户数据目录不动.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef AppBuildVersion
  #define AppBuildVersion "0.0.0"
#endif
#ifndef SourceBinary
  #error 缺少 SourceBinary 宏
#endif
#ifndef SourceIcon
  #error 缺少 SourceIcon 宏
#endif
#ifndef OutputDir
  #define OutputDir "dist"
#endif
#ifndef OutputBaseFilename
  #define OutputBaseFilename "scherun-setup"
#endif

[Setup]
AppId={{9F3C1B42-7E5A-4C8D-9B21-4D6A8E2F1C33}
AppName=scherun
AppVersion={#AppVersion}
AppVerName=scherun {#AppBuildVersion}
AppPublisher=azazo1
AppPublisherURL=https://github.com/azazo1/scherun
AppSupportURL=https://github.com/azazo1/scherun/issues
DefaultDirName={userpf}\scherun
DefaultGroupName=scherun
DisableProgramGroupPage=yes
; 已有安装时沿用上次的目录, 升级不会装出第二份.
UsePreviousAppDir=yes
; 默认 per-user 安装, 不请求提权.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=
OutputDir={#OutputDir}
OutputBaseFilename={#OutputBaseFilename}
SetupIconFile={#SourceIcon}
UninstallDisplayIcon={app}\scherun.exe
UninstallDisplayName=scherun
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
; 程序是 GUI 应用, 不需要控制台.
ArchitecturesAllowed=x64compatible arm64
ArchitecturesInstallIn64BitMode=x64compatible arm64

; Inno Setup 官方发行版不含简体中文语言文件 (ChineseSimplified.isl 属于第三方翻译),
; 直接引用会让编译失败. 这里条件包含: 有就用中文界面, 没有就回退到默认英文界面.
#define ChineseIsl AddBackslash(CompilerPath) + "Languages\ChineseSimplified.isl"

#if FileExists(ChineseIsl)
[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
#endif

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加任务:"; Flags: unchecked
Name: "autostart"; Description: "开机自动启动 scherun"; GroupDescription: "附加任务:"; Flags: unchecked

[Files]
; 主程序随版本更新, 用 ignoreversion 保证覆盖.
Source: "{#SourceBinary}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceIcon}"; DestDir: "{app}"; DestName: "scherun.ico"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\scherun"; Filename: "{app}\scherun.exe"
Name: "{autodesktop}\scherun"; Filename: "{app}\scherun.exe"; Tasks: desktopicon

[Registry]
; 开机自启用 HKCU 的 Run 项, per-user 安装不该写 HKLM.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; \
    ValueName: "scherun"; ValueData: """{app}\scherun.exe"""; Flags: uninsdeletevalue; \
    Tasks: autostart

[InstallDelete]
; 清掉上一版本可能残留而本版本不再提供的私有文件.
Type: files; Name: "{app}\scherun.ico"

[UninstallDelete]
; 运行期可能在安装目录生成的文件.
Type: filesandordirs; Name: "{app}"

[Run]
Filename: "{app}\scherun.exe"; Description: "启动 scherun"; Flags: nowait postinstall skipifsilent
