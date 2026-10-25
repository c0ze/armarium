; Armarium for Windows: a per-user install (no administrator rights) of the
; tray app and the command line. Build with
;   iscc /DMyAppVersion=X.Y.Z /DSourceDir=<folder with Armarium.exe, armarium.exe> packaging\windows\armarium.iss
; The AppId must never change: a newer installer finds the old install by it.

#ifndef MyAppVersion
  #define MyAppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir "..\..\dist\windows"
#endif

[Setup]
AppId={{8B4E1C52-6F3A-4D0B-9E7A-2C5F1A9D3E61}
AppName=Armarium
AppVersion={#MyAppVersion}
AppVerName=Armarium {#MyAppVersion}
AppPublisher=Gand
AppPublisherURL=https://armarium.skriv.ist/
AppSupportURL=https://github.com/c0ze/armarium/issues
AppUpdatesURL=https://github.com/c0ze/armarium/releases
DefaultDirName={localappdata}\Programs\Armarium
DisableDirPage=yes
DisableProgramGroupPage=yes
DisableReadyPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.17763
AppMutex=ArmariumDesktopRunning
CloseApplications=yes
RestartApplications=no
SetupIconFile=..\icons\armarium.ico
UninstallDisplayIcon={app}\Armarium.exe
UninstallDisplayName=Armarium
OutputDir=..\..\dist
OutputBaseFilename=Armarium-Setup-{#MyAppVersion}-x64
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Files]
Source: "{#SourceDir}\Armarium.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\armarium.exe"; DestDir: "{app}\cli"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{userprograms}\Armarium"; Filename: "{app}\Armarium.exe"; Comment: "Your books and comics, served from this PC"

[Run]
Filename: "{app}\Armarium.exe"; Description: "Start Armarium"; Flags: postinstall nowait skipifsilent

[Code]
const
  RunKey = 'Software\Microsoft\Windows\CurrentVersion\Run';

// The tray app holds a named mutex; ask it to close before files are removed.
function InitializeUninstall(): Boolean;
begin
  Result := True;
  while CheckForMutexes('ArmariumDesktopRunning') do
  begin
    if MsgBox('Armarium is still running. Quit it from its icon in the taskbar notification area, then click OK.',
      mbInformation, MB_OKCANCEL) = IDCANCEL then
    begin
      Result := False;
      Exit;
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Data: String;
begin
  if CurUninstallStep = usUninstall then
    RegDeleteValue(HKEY_CURRENT_USER, RunKey, 'Armarium');
  if CurUninstallStep = usPostUninstall then
  begin
    Data := ExpandConstant('{localappdata}\Armarium');
    if DirExists(Data) and not UninstallSilent() then
      if MsgBox('Also delete your Armarium settings and reading progress?' + #13#10#13#10 +
        'Your book and comic files are not touched either way. Keep them if you might install Armarium again.',
        mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
        DelTree(Data, True, True, True);
  end;
end;
