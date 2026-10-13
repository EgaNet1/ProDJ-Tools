; Установщик ProDJ Tools (Inno Setup 6).
; Собирается скриптом build-installer.ps1 из корня проекта; он же передаёт
; версию и путь к exe:  ISCC /DAppVersion=1.2.3 /DSourceExe=..\ProDJTools.exe

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceExe
  #define SourceExe "..\ProDJTools.exe"
#endif

#define AppName "ProDJ Tools"
#define AppExe  "ProDJTools.exe"
#define AppURL  "https://github.com/EgaNet1/ProDJ-Tools"

[Setup]
; AppId связывает версии между собой: не менять, иначе обновление
; установится рядом со старой версией, а не поверх неё.
AppId={{6F3C2B9E-4D1A-4E7B-9C25-8A1F0D3E7B42}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=EgaNet1
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
VersionInfoVersion={#AppVersion}

; Без прав администратора программа ставится только для текущего
; пользователя в %LOCALAPPDATA%\Programs; в диалоге можно выбрать всех.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
UsePreviousAppDir=yes

ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0

OutputDir=..\dist
OutputBaseFilename=ProDJTools-Setup-{#AppVersion}
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
SetupIconFile=..\assets\icon.ico
UninstallDisplayIcon={app}\{#AppExe}
UninstallDisplayName={#AppName}
; Перед обновлением закрыть запущенную программу.
CloseApplications=yes

; Inno Setup picks the language of Windows. Chinese, Hindi and Indonesian
; have no official Inno Setup translation, so the wizard falls back to
; English there; the program itself speaks all of its languages.
[Languages]
Name: "en"; MessagesFile: "compiler:Default.isl"
Name: "ru"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "es"; MessagesFile: "compiler:Languages\Spanish.isl"
Name: "pt"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"
Name: "fr"; MessagesFile: "compiler:Languages\French.isl"
Name: "de"; MessagesFile: "compiler:Languages\German.isl"
Name: "it"; MessagesFile: "compiler:Languages\Italian.isl"
Name: "pl"; MessagesFile: "compiler:Languages\Polish.isl"
Name: "tr"; MessagesFile: "compiler:Languages\Turkish.isl"
Name: "ar"; MessagesFile: "compiler:Languages\Arabic.isl"
Name: "ja"; MessagesFile: "compiler:Languages\Japanese.isl"
Name: "ko"; MessagesFile: "compiler:Languages\Korean.isl"

[CustomMessages]
en.RemoveUserData=Also remove the ProDJ Tools analysis cache, settings and log?%n%nYour music folders, playlists and .prodj-backup.json backups are not touched.
ru.RemoveUserData=Удалить также кэш анализа, настройки и журнал ProDJ Tools?%n%nПапки с музыкой, плейлисты и резервные копии .prodj-backup.json не затрагиваются.
es.RemoveUserData=¿Eliminar también la caché de análisis, los ajustes y el registro de ProDJ Tools?%n%nTus carpetas de música, listas y copias .prodj-backup.json no se tocan.
pt.RemoveUserData=Remover também o cache de análise, as configurações e o registro do ProDJ Tools?%n%nSuas pastas de música, playlists e backups .prodj-backup.json não são alterados.
fr.RemoveUserData=Supprimer aussi le cache d’analyse, les réglages et le journal de ProDJ Tools ?%n%nVos dossiers de musique, playlists et sauvegardes .prodj-backup.json ne sont pas touchés.
de.RemoveUserData=Auch Analyse-Cache, Einstellungen und Protokoll von ProDJ Tools entfernen?%n%nMusikordner, Playlists und .prodj-backup.json-Sicherungen bleiben unberührt.
it.RemoveUserData=Rimuovere anche la cache di analisi, le impostazioni e il registro di ProDJ Tools?%n%nCartelle musicali, playlist e backup .prodj-backup.json non vengono toccati.
pl.RemoveUserData=Usunąć także pamięć podręczną analizy, ustawienia i dziennik ProDJ Tools?%n%nFoldery z muzyką, playlisty i kopie .prodj-backup.json pozostaną nietknięte.
tr.RemoveUserData=ProDJ Tools analiz önbelleği, ayarları ve günlüğü de kaldırılsın mı?%n%nMüzik klasörleriniz, çalma listeleriniz ve .prodj-backup.json yedekleri etkilenmez.
ar.RemoveUserData=هل تريد أيضًا إزالة ذاكرة التحليل والإعدادات والسجل الخاصة بـ ProDJ Tools؟%n%nلن تُمس مجلدات الموسيقى وقوائم التشغيل والنسخ الاحتياطية ‎.prodj-backup.json.
ja.RemoveUserData=ProDJ Tools の解析キャッシュ、設定、ログも削除しますか？%n%n音楽フォルダー、プレイリスト、.prodj-backup.json のバックアップには影響しません。
ko.RemoveUserData=ProDJ Tools의 분석 캐시, 설정, 로그도 삭제할까요?%n%n음악 폴더, 재생목록, .prodj-backup.json 백업은 그대로 유지됩니다.

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "{#AppExe}"; Flags: ignoreversion
Source: "..\LICENSE"; DestDir: "{app}"; DestName: "LICENSE.txt"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#AppExe}"; Description: "{cm:LaunchProgram,{#AppName}}"; Flags: nowait postinstall skipifsilent

[Code]
// Программа хранит кэш, настройки, журнал и профиль окна в
// %LOCALAPPDATA%\ProDJTools (см. cmd/prodj/main.go). При удалении
// спрашиваем, убрать ли и их: кэш анализа большой библиотеки жалко терять,
// если программу просто переустанавливают.
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: String;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  DataDir := ExpandConstant('{localappdata}\ProDJTools');
  if DirExists(DataDir) and not UninstallSilent then
    if MsgBox(CustomMessage('RemoveUserData'), mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
      DelTree(DataDir, True, True, True);
end;
