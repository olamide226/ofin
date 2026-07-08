; Òfin Windows Installer — NSIS script
; Build with: makensis ofin.nsi
; Requires NSIS 3.x (brew install makensis or choco install nsis)

!define PRODUCT "Òfin"
; Override from the command line: makensis /DVERSION=0.3.0 ofin.nsi
!ifndef VERSION
  !define VERSION "0.2.0"
!endif
!define PUBLISHER "Ruach Tech"

Name "${PRODUCT} ${VERSION}"
OutFile "Ofin-Setup-${VERSION}.exe"
InstallDir "$LOCALAPPDATA\Ofin"
RequestExecutionLevel user
SetCompressor /SOLID lzma

; ---------- Modern UI ----------
!include "MUI2.nsh"

!define MUI_ICON "..\icons\ofin.ico"
!define MUI_UNICON "..\icons\ofin.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"

  ; Application files
  File "bin\ofin.exe"
  File "..\..\metadata.json"
  File "..\icons\ofin.ico"
  File "launcher.bat"

  ; Bundled llama.cpp (official Windows build): llama-server.exe + all DLLs.
  ; ofin.exe resolves it from this llama\ subfolder automatically.
  SetOutPath "$INSTDIR\llama"
  File "bin\llama\*.*"

  ; Corpus DB + embedding model — staged here, copied to the data dir by
  ; launcher.bat on first run.
  SetOutPath "$INSTDIR\data"
  File "bin\ofin.db"
  SetOutPath "$INSTDIR\models-dev"
  File "bin\bge-small-en-v1.5-f16.gguf"

  SetOutPath "$INSTDIR"

  ; Start Menu shortcut
  CreateDirectory "$SMPROGRAMS\${PRODUCT}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT}\Òfin.lnk" "$INSTDIR\launcher.bat" "" "$INSTDIR\ofin.ico"
  CreateShortCut "$SMPROGRAMS\${PRODUCT}\Uninstall.lnk" "$INSTDIR\uninstall.exe"

  ; Desktop shortcut
  CreateShortCut "$DESKTOP\Òfin.lnk" "$INSTDIR\launcher.bat" "" "$INSTDIR\ofin.ico"

  ; Write uninstaller
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; Registry for Add/Remove Programs
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "DisplayName" "${PRODUCT}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "Publisher" "${PUBLISHER}"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin" "NoRepair" 1
SectionEnd

Section "Uninstall"
  ; Stop running instance
  ExecWait "taskkill /F /IM ofin.exe"
  ExecWait "taskkill /F /IM llama-server.exe"

  ; Remove application files
  Delete "$INSTDIR\ofin.exe"
  Delete "$INSTDIR\ofin.ico"
  Delete "$INSTDIR\launcher.bat"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$INSTDIR\metadata.json"
  RMDir /r "$INSTDIR\llama"
  Delete "$INSTDIR\data\ofin.db"
  Delete "$INSTDIR\models-dev\bge-small-en-v1.5-f16.gguf"
  RMDir "$INSTDIR\data"
  RMDir "$INSTDIR\model"
  RMDir "$INSTDIR\models-dev"
  RMDir "$INSTDIR"

  ; Note: the downloaded 1.9 GB model in %LOCALAPPDATA%\Ofin is left in place.
  ; Users can delete that folder manually to reclaim the space.

  ; Remove shortcuts
  Delete "$SMPROGRAMS\${PRODUCT}\*"
  RMDir "$SMPROGRAMS\${PRODUCT}"
  Delete "$DESKTOP\Òfin.lnk"

  ; Remove registry
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\Ofin"
SectionEnd
