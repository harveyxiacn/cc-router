; -*- coding: utf-8 -*-
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"
Name "CC Router ${VERSION}"
OutFile "${OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\CC Router"
RequestExecutionLevel user
SetCompressor /SOLID zlib
ShowInstDetails show
ShowUninstDetails show
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\CC Router"
!define MUI_ABORTWARNING
!define MUI_ICON "..\desktop\build\windows\icon.ico"
!define MUI_UNICON "..\desktop\build\windows\icon.ico"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${STAGE}\LICENSE"
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
Var LeaseHandle
Var LeaseOverlapped
Var InstallerMutex
Var RollbackFailed

; The complete owned-file list is shared by preflight, backup, commit and removal.
; There are no recursive deletes and no PATH, ccr alias or account-data changes.
!macro OwnedFiles operation
  !insertmacro ${operation} "cc-router-desktop.exe"
  !insertmacro ${operation} "cc-router.exe"
  !insertmacro ${operation} "README.md"
  !insertmacro ${operation} "LICENSE"
  !insertmacro ${operation} "THIRD_PARTY_NOTICES.txt"
  !insertmacro ${operation} "compatibility.md"
  !insertmacro ${operation} "DESKTOP.md"
  !insertmacro ${operation} "distribution.md"
  !insertmacro ${operation} "Uninstall.exe"
  !insertmacro ${operation} ".cc-router-installed"
!macroend

!macro Init prefix
!if "${prefix}" == ""
Function .onInit
!else
Function un.onInit
!endif
  SetShellVarContext current
  StrCpy $LeaseHandle ""
  StrCpy $LeaseOverlapped ""
  ; A single per-session mutex serializes installer and uninstaller instances.
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Local\CCRouterInstaller") p .r0 ?e'
  Pop $1
  StrCpy $InstallerMutex $0
  ${If} $0 == 0
  ${OrIf} $1 == 183
    MessageBox MB_OK|MB_ICONSTOP "Another CC Router installer or uninstaller is running." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  ${IfNot} ${RunningX64}
    MessageBox MB_OK|MB_ICONSTOP "CC Router requires 64-bit Windows." /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
  !if "${prefix}" == ""
    ; Fixed per-user location, including for /S or /D. Portable users use OTA.
    StrCpy $INSTDIR "$LOCALAPPDATA\Programs\CC Router"
    ${If} ${FileExists} "$INSTDIR\cc-router-desktop.exe"
    ${AndIfNot} ${FileExists} "$INSTDIR\.cc-router-installed"
      MessageBox MB_OK|MB_ICONSTOP "This location contains a portable installation. Use its built-in updater or move it before installing." /SD IDOK
      SetErrorLevel 2
      Abort
    ${EndIf}
  !endif
FunctionEnd

Function ${prefix}AcquireLease
  ClearErrors
  ; Do not permit _?= or /D to redirect known-file operations to another folder.
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\CC Router"
  !if "${prefix}" == "un."
    IfFileExists "$INSTDIR\.cc-router-installed" +2 0
    Goto unsafe
  !endif
  ; Reject the installation leaf if it is a junction/link or a non-directory.
  System::Call 'kernel32::GetFileAttributesW(w "$INSTDIR") i .r0'
  ${If} $0 != -1
    IntOp $1 $0 & 0x400
    IntOp $2 $0 & 0x10
    ${If} $1 != 0
    ${OrIf} $2 == 0
      Goto unsafe
    ${EndIf}
  ${EndIf}
  CreateDirectory "$INSTDIR"
  IfErrors unsafe
  IfFileExists "$INSTDIR\.cc-router-install-new" busy 0
  IfFileExists "$INSTDIR\.cc-router-install-old" busy 0
  System::Call 'kernel32::GetFileAttributesW(w "$INSTDIR\.cc-router-update.lock") i .r0'
  ${If} $0 != -1
    IntOp $1 $0 & 0x410
    ${If} $1 != 0
      Goto unsafe
    ${EndIf}
  ${EndIf}
  ; Share read/write/delete, then lock byte zero exactly as internal/update does.
  System::Call 'kernel32::CreateFileW(w "$INSTDIR\.cc-router-update.lock", i 0xC0000000, i 7, p 0, i 4, i 0x80, p 0) p .r0'
  ${If} $0 == -1
    Goto unsafe
  ${EndIf}
  StrCpy $LeaseHandle $0
  ; NSIS runs as a 32-bit process; OVERLAPPED = two pointers, two DWORDs, event.
  System::Call '*(p 0, p 0, i 0, i 0, p 0) p .r0'
  StrCpy $LeaseOverlapped $0
  System::Call 'kernel32::LockFileEx(p $LeaseHandle, i 3, i 0, i 1, i 0, p $LeaseOverlapped) i .r0'
  ${If} $0 == 0
    Goto busy
  ${EndIf}
  Return
busy:
  Call ${prefix}ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "Close CC Router, its companion CLI and managed sessions first. If an update or earlier installer left a recovery directory, resolve it before retrying. Existing users should use the built-in updater." /SD IDOK
  SetErrorLevel 2
  Abort
unsafe:
  Call ${prefix}ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "The installation location is not a writable regular directory. No installed files were replaced." /SD IDOK
  SetErrorLevel 2
  Abort
FunctionEnd

Function ${prefix}ReleaseLease
  ${If} $LeaseHandle != ""
    ${If} $LeaseOverlapped != ""
      System::Call 'kernel32::UnlockFileEx(p $LeaseHandle, i 0, i 1, i 0, p $LeaseOverlapped)'
      System::Free $LeaseOverlapped
      StrCpy $LeaseOverlapped ""
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p $LeaseHandle)'
    StrCpy $LeaseHandle ""
  ${EndIf}
FunctionEnd

!if "${prefix}" == ""
Function .onGUIEnd
!else
Function un.onGUIEnd
!endif
  Call ${prefix}ReleaseLease
  ${If} $InstallerMutex != ""
    System::Call 'kernel32::CloseHandle(p $InstallerMutex)'
  ${EndIf}
FunctionEnd
!macroend
!insertmacro Init ""
!insertmacro Init "un."

!macro Preflight file
  ${If} ${FileExists} "$INSTDIR\${file}"
    System::Call 'kernel32::GetFileAttributesW(w "$INSTDIR\${file}") i .r0'
    IntOp $1 $0 & 0x410
    ${If} $1 != 0
      Goto locked
    ${EndIf}
    ; Exclusive open catches even older binaries that do not hold an OTA lease.
    System::Call 'kernel32::CreateFileW(w "$INSTDIR\${file}", i 0x80000000, i 0, p 0, i 3, i 0x80, p 0) p .r0'
    ${If} $0 == -1
      Goto locked
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p r0)'
  ${EndIf}
!macroend
!macro Backup file
  ${If} ${FileExists} "$INSTDIR\${file}"
    ClearErrors
    Rename "$INSTDIR\${file}" "$INSTDIR\.cc-router-install-old\${file}"
    IfErrors rollback
  ${EndIf}
!macroend
!macro Commit file
  ClearErrors
  Rename "$INSTDIR\.cc-router-install-new\${file}" "$INSTDIR\${file}"
  IfErrors rollback
!macroend
!macro Restore file
  ; A missing staged file means its replacement was committed; remove only it.
  ${IfNot} ${FileExists} "$INSTDIR\.cc-router-install-new\${file}"
    ClearErrors
    Delete "$INSTDIR\${file}"
    ${If} ${Errors}
      StrCpy $RollbackFailed 1
    ${EndIf}
  ${EndIf}
  ${If} ${FileExists} "$INSTDIR\.cc-router-install-old\${file}"
    ClearErrors
    Rename "$INSTDIR\.cc-router-install-old\${file}" "$INSTDIR\${file}"
    ${If} ${Errors}
      StrCpy $RollbackFailed 1
    ${EndIf}
  ${EndIf}
!macroend
!macro Cleanup file
  Delete "$INSTDIR\.cc-router-install-new\${file}"
  Delete "$INSTDIR\.cc-router-install-old\${file}"
!macroend
!macro Remove file
  Delete "$INSTDIR\${file}"
!macroend

!macro PrepareMutation prefix
  Call ${prefix}AcquireLease
  !insertmacro OwnedFiles Preflight
  ${If} ${FileExists} "$INSTDIR\.cc-router-update-work"
    ; The embedded fresh companion validates and retires completed OTA journals.
    ; It takes its own lease: never lend it an unverifiable caller-owned lock.
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File /oname=cc-router-installer-state.exe "${STAGE}\cc-router.exe"
    IfErrors helper_failed
    Call ${prefix}ReleaseLease
    nsExec::ExecToStack /TIMEOUT=30000 '$\"$PLUGINSDIR\cc-router-installer-state.exe$\" internal-installer-state $\"$INSTDIR$\"'
    Pop $0
    Pop $1
    ${If} $0 != 0
      Goto helper_failed
    ${EndIf}
    Call ${prefix}AcquireLease
    ; A new update could have started while the helper released its lease.
    ${If} ${FileExists} "$INSTDIR\.cc-router-update-work"
      Goto helper_failed
    ${EndIf}
    !insertmacro OwnedFiles Preflight
  ${EndIf}
!macroend

Section "CC Router"
  !insertmacro PrepareMutation ""
  CreateDirectory "$INSTDIR\.cc-router-install-old"
  SetOutPath "$INSTDIR\.cc-router-install-new"
  SetOverwrite off
  File "${STAGE}\cc-router-desktop.exe"
  File "${STAGE}\cc-router.exe"
  File "${STAGE}\README.md"
  File "${STAGE}\LICENSE"
  File "${STAGE}\THIRD_PARTY_NOTICES.txt"
  File "${STAGE}\compatibility.md"
  File "${STAGE}\DESKTOP.md"
  File /oname=distribution.md "${DISTRIBUTION_DOC}"
  IfErrors stage_failed
  WriteUninstaller "$INSTDIR\.cc-router-install-new\Uninstall.exe"
  IfErrors stage_failed
  FileOpen $0 "$INSTDIR\.cc-router-install-new\.cc-router-installed" w
  IfErrors stage_failed
  FileWrite $0 "CC Router ${VERSION}$\r$\n"
  IfErrors marker_failed
  FileClose $0
  IfErrors stage_failed
  ; Change working directory so staging directories can be removed.
  SetOutPath "$TEMP"
  !insertmacro OwnedFiles Backup
  !insertmacro OwnedFiles Commit
  !insertmacro OwnedFiles Cleanup
  RMDir "$INSTDIR\.cc-router-install-new"
  RMDir "$INSTDIR\.cc-router-install-old"
  CreateDirectory "$SMPROGRAMS\CC Router"
  CreateShortcut "$SMPROGRAMS\CC Router\CC Router.lnk" "$INSTDIR\cc-router-desktop.exe"
  CreateShortcut "$SMPROGRAMS\CC Router\Uninstall CC Router.lnk" "$INSTDIR\Uninstall.exe"
  CreateDirectory "$DESKTOP"
  CreateShortcut "$DESKTOP\CC Router.lnk" "$INSTDIR\cc-router-desktop.exe"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "CC Router"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "Harvey Xia"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\cc-router-desktop.exe"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\Uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
  Call ReleaseLease
  Goto done
locked:
  Call ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "An installed file is running, locked, or is not a regular file. Close the application and CLI, then retry. The previous installation has not been changed." /SD IDOK
  SetErrorLevel 2
  Abort
helper_failed:
  Call ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "The existing OTA recovery state is unfinished, damaged, busy, or could not be retired safely. Open CC Router to recover the update before installing. Installed program files and recovery evidence were preserved." /SD IDOK
  SetErrorLevel 2
  Abort
marker_failed:
  FileClose $0
  Goto stage_failed
stage_failed:
  SetOutPath "$TEMP"
  !insertmacro OwnedFiles Cleanup
  RMDir "$INSTDIR\.cc-router-install-new"
  RMDir "$INSTDIR\.cc-router-install-old"
  Call ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "Could not stage the new installation. The previous installation has not been changed." /SD IDOK
  SetErrorLevel 2
  Abort
rollback:
  StrCpy $RollbackFailed 0
  !insertmacro OwnedFiles Restore
  ${If} $RollbackFailed == 0
    !insertmacro OwnedFiles Cleanup
    RMDir "$INSTDIR\.cc-router-install-new"
    RMDir "$INSTDIR\.cc-router-install-old"
    MessageBox MB_OK|MB_ICONSTOP "Installation failed. The previous installed files were restored." /SD IDOK
  ${Else}
    MessageBox MB_OK|MB_ICONSTOP "Installation failed and a file could not be restored. Previous files are preserved in .cc-router-install-old. Keep both installer recovery directories for manual recovery." /SD IDOK
  ${EndIf}
  Call ReleaseLease
  SetErrorLevel 2
  Abort
done:
SectionEnd

Section "Uninstall"
  ; Running uninstaller executes from a temporary copy; check all other files.
  !insertmacro PrepareMutation "un."
  ClearErrors
  !insertmacro OwnedFiles Remove
  IfErrors removal_failed
  Delete "$SMPROGRAMS\CC Router\CC Router.lnk"
  Delete "$SMPROGRAMS\CC Router\Uninstall CC Router.lnk"
  RMDir "$SMPROGRAMS\CC Router"
  Delete "$DESKTOP\CC Router.lnk"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  Call un.ReleaseLease
  ; Only the owned lock file and an empty directory may be removed.
  Delete "$INSTDIR\.cc-router-update.lock"
  RMDir "$INSTDIR"
  Goto done
locked:
  Call un.ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "An installed file is running, locked, or is not a regular file. Close CC Router and its companion CLI, then retry. No account data was changed." /SD IDOK
  SetErrorLevel 2
  Abort
helper_failed:
  Call un.ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "The existing OTA recovery state is unfinished, damaged, busy, or could not be retired safely. Open CC Router to recover the update before uninstalling. Installed program files, account data and recovery evidence were preserved." /SD IDOK
  SetErrorLevel 2
  Abort
removal_failed:
  Call un.ReleaseLease
  MessageBox MB_OK|MB_ICONSTOP "Some program files could not be removed. Close programs using this folder and retry. Account profiles, metadata and backups are preserved." /SD IDOK
  SetErrorLevel 2
  Abort
done:
SectionEnd
