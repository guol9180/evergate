#ifndef AppVersion
  #define AppVersion "0.0.0-dev"
#endif
#ifndef SourceExe
  #define SourceExe "..\bin\evergate-windows-amd64.exe"
#endif
#ifndef OutputDir
  #define OutputDir "..\release-installer"
#endif
#ifndef ProgramRoot
  #define ProgramRoot "{localappdata}\Programs\EverGate"
#endif
#ifndef AppDataRoot
  #define AppDataRoot "{localappdata}\EverGate"
#endif
#ifndef SetupAppId
  #define SetupAppId "{{67AE737E-F141-45CB-A535-52DA6953F1F4}"
#endif

[Setup]
AppId={#SetupAppId}
AppName=EverGate
AppVersion={#AppVersion}
AppVerName=EverGate {#AppVersion}
AppPublisher=EverGate
AppPublisherURL=https://github.com/guol9180/evergate
AppSupportURL=https://github.com/guol9180/evergate/issues
AppUpdatesURL=https://github.com/guol9180/evergate/releases
DefaultDirName={#ProgramRoot}
DefaultGroupName=EverGate
DisableDirPage=yes
DisableProgramGroupPage=yes
UsePreviousAppDir=no
UsePreviousTasks=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir={#OutputDir}
OutputBaseFilename=evergate_{#AppVersion}_windows_amd64_setup
SetupIconFile=..\web\public\favicon.ico
UninstallDisplayIcon={app}\evergate.exe
LicenseFile=..\LICENSE
Compression=lzma2
SolidCompression=yes
WizardStyle=modern dynamic
CloseApplications=yes
RestartApplications=no
VersionInfoProductName=EverGate

[Languages]
#ifdef UseChineseLanguage
Name: "chinesesimp"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
#else
Name: "english"; MessagesFile: "compiler:Default.isl"
#endif

[Tasks]
Name: "autostart"; Description: "登录 Windows 时自动启动 EverGate"; GroupDescription: "启动选项："; Flags: unchecked

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "evergate.exe"; Flags: ignoreversion

[Icons]
Name: "{group}\EverGate"; Filename: "{app}\evergate.exe"

[Registry]
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "InstallDir"; ValueData: "{app}"; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "ConfigPath"; ValueData: "{#AppDataRoot}\evergate.yml"; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\EverGate"; ValueType: string; ValueName: "DataDir"; ValueData: "{#AppDataRoot}\data"; Flags: uninsdeletevalue
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "EverGate"; ValueData: """{app}\evergate.exe"" --autostart"; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\evergate.exe"; Description: "立即启动 EverGate"; Flags: nowait postinstall

[Code]
var
  InstallStatePage: TOutputMsgWizardPage;

function ConfigPath: String;
begin
  Result := ExpandConstant('{#AppDataRoot}\evergate.yml');
end;

function DataDir: String;
begin
  Result := ExpandConstant('{#AppDataRoot}\data');
end;

function IsUpgrade: Boolean;
begin
  Result := FileExists(ExpandConstant('{#ProgramRoot}\evergate.exe'));
end;

procedure InitializeWizard;
begin
  if IsUpgrade then
    InstallStatePage := CreateOutputMsgPage(wpLicense,
      '安装状态', '检测到现有 EverGate',
      '将升级 ' + ExpandConstant('{#ProgramRoot}') + ' 中的程序。' + #13#10#13#10 +
      '配置与数据库固定在 ' + ExpandConstant('{#AppDataRoot}') + '，升级不会覆盖或删除已有数据。')
  else
    InstallStatePage := CreateOutputMsgPage(wpLicense,
      '安装状态', '准备首次安装',
      'EverGate 将安装到 ' + ExpandConstant('{#ProgramRoot}') + '。' + #13#10#13#10 +
      '配置与数据库会创建在 ' + ExpandConstant('{#AppDataRoot}') + '。');
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    ForceDirectories(ExpandConstant('{#AppDataRoot}'));
    ForceDirectories(DataDir);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DeleteUserData: Boolean;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    DeleteUserData := False;
    if not UninstallSilent then
      DeleteUserData := MsgBox(
        '是否同时删除 EverGate 配置和数据库？' + #13#10#13#10 +
        '配置文件：' + ConfigPath + #13#10 +
        '数据目录：' + DataDir + #13#10#13#10 +
        '选择“否”将保留这些数据。',
        mbConfirmation, MB_YESNO) = IDYES;
    if DeleteUserData then
    begin
      DeleteFile(ConfigPath);
      DelTree(DataDir, True, True, True);
    end;
  end;
end;
