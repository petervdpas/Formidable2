Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them.
####
## Override the auto-generated default so the installer asset is
## "Formidable-amd64-installer.exe" rather than "Formidable2-amd64-installer.exe".
## The "2" is only a repo/module disambiguator and must never leak into
## any user-visible filename, registry key, or shortcut.
####
!define INFO_PROJECTNAME "Formidable"
####
## Include the wails tools (provides INFO_COMPANYNAME, INFO_PRODUCTNAME,
## INFO_PRODUCTVERSION, INFO_COPYRIGHT, PRODUCT_EXECUTABLE, UNINST_KEY,
## REQUEST_EXECUTION_LEVEL and the wails.* install macros).
####
!include "wails_tools.nsh"

# Compression: solid LZMA gives a smaller installer and is less common
# in malware boilerplate (those tend to use plain zlib).
SetCompressor /SOLID lzma

# Explicit CRC verification of the installer payload on launch.
CRCCheck on

# Replaces the default "Nullsoft Install System v3.x" footer with a
# product-specific branding line. One of the cheaper signals that
# nudges a binary out of "default NSIS template" heuristics.
BrandingText "Formidable - Build and Extend Your Workflow"

# Required 4-part version for VIProductVersion / VIFileVersion.
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
VIAddVersionKey "Comments"         "Open-source editor for structured templates and Markdown records. https://formidable.tools"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
# Wait on the INSTFILES page so the user can take a look into the details of the installation steps.
!define MUI_FINISHPAGE_NOAUTOCLOSE
# This will warn the user if they exit from the installer.
!define MUI_ABORTWARNING

# Custom Welcome page copy. Default Wails text is generic NSIS boilerplate;
# replacing it grounds the installer in the product's voice.
!define MUI_WELCOMEPAGE_TITLE "Welcome to the ${INFO_PRODUCTNAME} setup"
!define MUI_WELCOMEPAGE_TEXT "This wizard installs ${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION}, a desktop editor for YAML templates, Markdown forms, and PDF export.$\r$\n$\r$\nClose any running copy of ${INFO_PRODUCTNAME} before continuing.$\r$\n$\r$\nClick Next to proceed."

# Custom Finish page copy + run + link to the project site.
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

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_PRODUCTNAME}"
ShowInstDetails show

Function .onInit
   !insertmacro wails.checkArchitecture

   # If a previous install registered an InstallLocation, propose it
   # as the default $INSTDIR so upgrades land in the same folder
   # instead of leaving an orphan copy at the prior path.
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

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller

    # Record install location so future installers auto-detect it
    # and offer the same folder by default.
    SetRegView 64
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"

    # Enrich the Programs-and-Features entry so Windows shows the
    # publisher's site, a help link, and disables the Modify/Repair
    # buttons (this installer only supports clean upgrade-or-uninstall).
    WriteRegStr  HKLM "${UNINST_KEY}" "URLInfoAbout"    "https://formidable.tools/"
    WriteRegStr  HKLM "${UNINST_KEY}" "HelpLink"        "https://formidable.tools/"
    WriteRegStr  HKLM "${UNINST_KEY}" "URLUpdateInfo"   "https://formidable.tools/download/"
    WriteRegStr  HKLM "${UNINST_KEY}" "Comments"        "Editor for templates and Markdown forms."
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoModify" 0x00000001
    WriteRegDWORD HKLM "${UNINST_KEY}" "NoRepair" 0x00000001
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd
