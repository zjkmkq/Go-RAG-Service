@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo ==== Starting dependencies ====
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-server.ps1"
if errorlevel 1 (
    echo.
    echo Something failed above. Fix it, then run this again.
    pause
    exit /b 1
)
echo.
echo ==== Entering chat ====
echo Type a question, or drag a txt/md file into this window to upload it.
echo Type "help" for commands, "exit" to quit.
echo.
ragcli.exe
pause
