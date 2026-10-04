@echo off
setlocal

set "BINARY=%~dp0bin\google-multi-auth-windows-amd64.exe"

if not exist "%BINARY%" (
	echo Error: expected binary not found at %BINARY% 1>&2
	echo Download a release zip, or run build.sh to build from source. 1>&2
	exit /b 1
)

"%BINARY%" setup %*
