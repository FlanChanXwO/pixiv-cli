@echo off
setlocal EnableExtensions DisableDelayedExpansion

rem 该脚本只使用 Windows 10/11 自带的命令行工具，不调用其他脚本宿主。
set "REPOSITORY=FlanChanXwO/pixiv-cli"
set "RELEASE_ROOT=https://github.com/%REPOSITORY%/releases/latest/download"
set "INSTALL_DIR=%LOCALAPPDATA%\Programs\pixiv"
set "PATH_MODE=report"
set "WORK_DIR="
set "STAGED="

:parse_arguments
if "%~1"=="" goto arguments_done
if /I "%~1"=="--install-dir" (
  if "%~2"=="" (set "ERROR_MESSAGE=--install-dir requires a directory" & goto fatal)
  set "INSTALL_DIR=%~2"
  shift
  shift
  goto parse_arguments
)
if /I "%~1"=="--add-to-path" (
  set "PATH_MODE=add"
  shift
  goto parse_arguments
)
if /I "%~1"=="--no-path" (
  set "PATH_MODE=skip"
  shift
  goto parse_arguments
)
if /I "%~1"=="--help" goto show_help
if /I "%~1"=="-h" goto show_help
set "ERROR_MESSAGE=unknown argument: %~1"
goto fatal

:arguments_done
if not defined LOCALAPPDATA (set "ERROR_MESSAGE=LOCALAPPDATA is required" & goto fatal)
if not defined TEMP (set "ERROR_MESSAGE=TEMP is required" & goto fatal)
if not defined INSTALL_DIR (set "ERROR_MESSAGE=install directory cannot be empty" & goto fatal)

where curl.exe >nul 2>&1 || (set "ERROR_MESSAGE=curl.exe is required" & goto fatal)
where certutil.exe >nul 2>&1 || (set "ERROR_MESSAGE=certutil.exe is required" & goto fatal)
where tar.exe >nul 2>&1 || (set "ERROR_MESSAGE=tar.exe is required" & goto fatal)

set "MACHINE_ARCH=%PROCESSOR_ARCHITECTURE%"
if defined PROCESSOR_ARCHITEW6432 set "MACHINE_ARCH=%PROCESSOR_ARCHITEW6432%"
if /I "%MACHINE_ARCH%"=="AMD64" set "TARGET_ARCH=amd64"
if /I "%MACHINE_ARCH%"=="ARM64" set "TARGET_ARCH=arm64"
if not defined TARGET_ARCH (set "ERROR_MESSAGE=only Windows AMD64 and ARM64 are supported" & goto fatal)

set "WORK_DIR=%TEMP%\pixiv-install-%RANDOM%-%RANDOM%-%RANDOM%"
mkdir "%WORK_DIR%" >nul 2>&1 || (set "ERROR_MESSAGE=cannot create a private temporary directory" & goto fatal)
set "CHECKSUMS=%WORK_DIR%\checksums.txt"

echo Downloading the latest stable release metadata...
curl.exe -fsSL "%RELEASE_ROOT%/checksums.txt" -o "%CHECKSUMS%" || (set "ERROR_MESSAGE=cannot download checksums.txt from the official release" & goto fatal)

set "ASSET="
set "EXPECTED="
set "DUPLICATE_ASSET="
for /f "tokens=1,2" %%A in ('findstr.exe /R /X /C:"[0-9a-f][0-9a-f]*  pixiv-cli_[0-9A-Za-z.+-][0-9A-Za-z.+-]*_windows_%TARGET_ARCH%[.]zip" "%CHECKSUMS%"') do (
  if defined ASSET set "DUPLICATE_ASSET=1"
  set "EXPECTED=%%A"
  set "ASSET=%%B"
)
if not defined ASSET (set "ERROR_MESSAGE=checksums.txt does not contain the Windows archive" & goto fatal)
if defined DUPLICATE_ASSET (set "ERROR_MESSAGE=checksums.txt contains more than one Windows archive" & goto fatal)
if "%EXPECTED:~63,1%"=="" (set "ERROR_MESSAGE=release checksum is not a SHA-256 digest" & goto fatal)
if not "%EXPECTED:~64,1%"=="" (set "ERROR_MESSAGE=release checksum is not a SHA-256 digest" & goto fatal)
for %%F in ("%ASSET%") do if /I not "%%~nxF"=="%ASSET%" (set "ERROR_MESSAGE=release archive name must be a basename" & goto fatal)

set "ARCHIVE=%WORK_DIR%\%ASSET%"
echo Downloading %ASSET%...
curl.exe -fsSL "%RELEASE_ROOT%/%ASSET%" -o "%ARCHIVE%" || (set "ERROR_MESSAGE=cannot download the platform archive from the official release" & goto fatal)

set "ACTUAL="
for /f "skip=1 tokens=* delims=" %%H in ('certutil.exe -hashfile "%ARCHIVE%" SHA256') do if not defined ACTUAL set "ACTUAL=%%H"
set "ACTUAL=%ACTUAL: =%"
if not defined ACTUAL (set "ERROR_MESSAGE=certutil.exe did not return a SHA-256 digest" & goto fatal)
if /I not "%ACTUAL%"=="%EXPECTED%" (set "ERROR_MESSAGE=SHA-256 mismatch; the existing installation was not changed" & goto fatal)
echo SHA-256 verified.

set "EXTRACT_DIR=%WORK_DIR%\extract"
mkdir "%EXTRACT_DIR%" >nul 2>&1 || (set "ERROR_MESSAGE=cannot create the extraction directory" & goto fatal)
tar.exe -xf "%ARCHIVE%" -C "%EXTRACT_DIR%" pixiv.exe || (set "ERROR_MESSAGE=the verified archive does not contain pixiv.exe" & goto fatal)
if not exist "%EXTRACT_DIR%\pixiv.exe" (set "ERROR_MESSAGE=the archive does not contain pixiv.exe" & goto fatal)

if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%" >nul 2>&1
if not exist "%INSTALL_DIR%" (set "ERROR_MESSAGE=cannot create the install directory" & goto fatal)
set "STAGED=%INSTALL_DIR%\pixiv.new-%RANDOM%-%RANDOM%.exe"
copy /Y "%EXTRACT_DIR%\pixiv.exe" "%STAGED%" >nul || (set "ERROR_MESSAGE=cannot stage the verified binary" & goto fatal)
"%STAGED%" version --json >nul 2>&1 || (set "ERROR_MESSAGE=the staged binary failed its version preflight" & goto fatal)
move /Y "%STAGED%" "%INSTALL_DIR%\pixiv.exe" >nul || (set "ERROR_MESSAGE=cannot replace the installed binary" & goto fatal)
set "STAGED="

if /I "%PATH_MODE%"=="add" call :add_user_path || goto fatal
if /I "%PATH_MODE%"=="report" call :report_path

echo Installed pixiv to %INSTALL_DIR%\pixiv.exe
"%INSTALL_DIR%\pixiv.exe" version
call :cleanup
exit /b 0

:add_user_path
where reg.exe >nul 2>&1 || (set "ERROR_MESSAGE=the binary was installed but reg.exe is required to update PATH" & exit /b 1)
set "USER_PATH="
set "USER_PATH_TYPE=REG_EXPAND_SZ"
for /f "tokens=1,2,*" %%A in ('reg.exe query HKCU\Environment /v Path 2^>nul') do (
  if /I "%%A"=="Path" set "USER_PATH_TYPE=%%B"
  if /I "%%A"=="Path" set "USER_PATH=%%C"
)
set "PATH_PRESENT="
for %%P in ("%USER_PATH:;=" "%") do if /I "%%~P"=="%INSTALL_DIR%" set "PATH_PRESENT=1"
if defined PATH_PRESENT exit /b 0
if defined USER_PATH (
  set "NEW_USER_PATH=%USER_PATH%;%INSTALL_DIR%"
) else (
  set "NEW_USER_PATH=%INSTALL_DIR%"
)
if /I not "%USER_PATH_TYPE%"=="REG_SZ" if /I not "%USER_PATH_TYPE%"=="REG_EXPAND_SZ" set "USER_PATH_TYPE=REG_EXPAND_SZ"
reg.exe add HKCU\Environment /v Path /t %USER_PATH_TYPE% /d "%NEW_USER_PATH%" /f >nul || (set "ERROR_MESSAGE=the binary was installed but the user PATH could not be updated" & exit /b 1)
echo Added %INSTALL_DIR% to the user PATH; sign out or restart the terminal host before using it.
exit /b 0

:report_path
set "PATH_PRESENT="
for %%P in ("%PATH:;=" "%") do if /I "%%~P"=="%INSTALL_DIR%" set "PATH_PRESENT=1"
if defined PATH_PRESENT exit /b 0
echo Add %INSTALL_DIR% to the user PATH, or rerun with --add-to-path.
exit /b 0

:show_help
echo Install the latest stable pixiv-cli release for Windows AMD64 or ARM64.
echo.
echo Usage:
echo   install.cmd [--install-dir DIR] [--add-to-path^|--no-path]
echo.
echo Options:
echo   --install-dir DIR  Install pixiv.exe into DIR.
echo   --add-to-path      Add the install directory to the user PATH.
echo   --no-path          Do not modify the user PATH.
echo   -h, --help         Show this help.
exit /b 0

:fatal
echo pixiv installer: %ERROR_MESSAGE% 1>&2
call :cleanup
exit /b 1

:cleanup
if defined STAGED del /f /q "%STAGED%" >nul 2>&1
if defined WORK_DIR rmdir /s /q "%WORK_DIR%" >nul 2>&1
exit /b 0
