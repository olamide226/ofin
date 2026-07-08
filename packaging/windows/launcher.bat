@echo off
:: Òfin Windows Launcher
:: Double-click to start Òfin. Opens your browser automatically.

setlocal
cd /d "%~dp0"

set "OFIN_DATA=%LOCALAPPDATA%\Ofin"

:: First run: seed the data directory (corpus DB + embedding model).
if not exist "%OFIN_DATA%\data\ofin.db" (
    echo First run - setting up...
    mkdir "%OFIN_DATA%\data" 2>nul
    mkdir "%OFIN_DATA%\model" 2>nul
    mkdir "%OFIN_DATA%\models-dev" 2>nul
    copy /Y "data\ofin.db" "%OFIN_DATA%\data\ofin.db" >nul
    copy /Y "models-dev\bge-small-en-v1.5-f16.gguf" "%OFIN_DATA%\models-dev\bge-small-en-v1.5-f16.gguf" >nul
)

:: Start Òfin. The web server shows a first-launch download page if the
:: 1.9 GB model isn't present yet, then flips to the app automatically.
:: ofin.exe finds the bundled llama-server.exe in the llama\ subfolder itself.
start "Ofin" /B "ofin.exe" serve --data-dir "%OFIN_DATA%"

:: Give the server a moment, then open the browser.
timeout /t 3 /nobreak >nul
start http://127.0.0.1:8090

echo Ofin is running at http://127.0.0.1:8090
echo Closing this window will stop Ofin.
pause

:: On close, stop the background processes.
taskkill /F /IM ofin.exe >nul 2>&1
taskkill /F /IM llama-server.exe >nul 2>&1
