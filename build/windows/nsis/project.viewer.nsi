Unicode true

####
## Standalone Formidable Viewer installer. Mirrors the main app's NSIS
## script but with the viewer's own identity, icon, and a leaner section:
## the viewer is read-only (it opens external .zip exports), so it needs
## no writable install dir, no file/protocol associations.
####
!define INFO_PROJECTNAME "FormidableViewer"
!define INFO_PRODUCTNAME "Formidable Viewer"

!include "wails_tools.nsh"

SetCompressor /SOLID lzma
CRCCheck on
BrandingText "Formidable Viewer - Open exports offline"

VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"      "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription"  "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"   "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"      "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"   "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"      "${INFO_PRODUCTNAME}"
VIAddVersionKey "InternalName"     "${INFO_PRODUCTNAME}-installer"
VIAddVersionKey "OriginalFilename" "${INFO_PROJECTNAME}-installer.exe"
VIAddVersionKey "Comments"         "Offline viewer for Formidable exports. https://formidable.tools"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.viewer.ico"
!define MUI_UNICON "..\icon.viewer.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

!define MUI_WELCOMEPAGE_TITLE "Welcome to the ${INFO_PRODUCTNAME} setup"
!define MUI_WELCOMEPAGE_TEXT "This wizard installs ${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION}, a standalone viewer for Formidable offline exports (.zip).$\r$\n$\r$\nClick Next to proceed."

!define MUI_FINISHPAGE_TITLE "${INFO_PRODUCTNAME} is installed"
!define MUI_FINISHPAGE_TEXT "${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION} is ready to use. A shortcut has been added to the Start menu and the Desktop."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${INFO_PRODUCTNAME}"
!define MUI_FINISHPAGE_LINK "Visit formidable.tools for documentation and updates"
!define MUI_FINISHPAGE_LINK_LOCATION "https://formidable.tools/"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "..\..\..\LICENSE"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture

   SetRegView 64
   ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
   ${If} $0 != ""
       StrCpy $INSTDIR "$0"
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller

    SetRegView 64
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"

    WriteRegStr  HKLM "${UNINST_KEY}" "URLInfoAbout"    "https://formidable.tools/"
    WriteRegStr  HKLM "${UNINST_KEY}" "HelpLink"        "https://formidable.tools/"
    WriteRegStr  HKLM "${UNINST_KEY}" "Comments"        "Offline viewer for Formidable exports."
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 0x00000001
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 0x00000001
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.deleteUninstaller
SectionEnd
