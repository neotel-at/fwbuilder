; Qt 6 installer generated from the staged Windows release.
Unicode true
!include "MUI2.nsh"
!ifndef ARCH
  !error "ARCH must be defined"
!endif
!ifndef STAGE
  !error "STAGE must be defined"
!endif
!define APPNAME "Firewall Builder 6 (${ARCH})"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\FWBuilder6-${ARCH}"
!define STARTMENU_DIR "Firewall Builder 6 (${ARCH})"
Name "Firewall Builder 6 (${ARCH})"
OutFile "fwbuilder-6.0-win-${ARCH}-setup.exe"
InstallDir "$PROGRAMFILES64\Firewall Builder 6 (${ARCH})"
InstallDirRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\FWBuilder6-${ARCH}" "InstallLocation"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
SetRegView 64
ShowInstDetails show
ShowUninstDetails show
!define MUI_ABORTWARNING
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${STAGE}\doc\COPYING"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
Section "Firewall Builder" SecMain
  SectionIn RO
  SetShellVarContext all
  SetOutPath "$INSTDIR"
  ; Bundle the whole deployed Qt6 release, including resources and compilers.
  File /r "${STAGE}\*.*"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayName" "Firewall Builder 6 (${ARCH})"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayVersion" "6.0"
  WriteRegStr HKLM "${UNINST_KEY}" "Publisher" "Firewall Builder"
  WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\fwbuilder.exe"
  WriteRegStr HKLM "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 1
  CreateDirectory "$SMPROGRAMS\${STARTMENU_DIR}"
  CreateShortcut "$SMPROGRAMS\${STARTMENU_DIR}\Firewall Builder.lnk" "$INSTDIR\fwbuilder.exe"
  CreateShortcut "$SMPROGRAMS\${STARTMENU_DIR}\Uninstall.lnk" "$INSTDIR\uninstall.exe"
SectionEnd
Section "Uninstall"
  SetShellVarContext all
  Delete "$SMPROGRAMS\${STARTMENU_DIR}\Firewall Builder.lnk"
  Delete "$SMPROGRAMS\${STARTMENU_DIR}\Uninstall.lnk"
  RMDir "$SMPROGRAMS\${STARTMENU_DIR}"
  DeleteRegKey HKLM "${UNINST_KEY}"
  RMDir /r "$INSTDIR"
SectionEnd
