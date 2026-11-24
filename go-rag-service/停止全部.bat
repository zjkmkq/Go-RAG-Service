@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo ==== Stopping everything ====
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0manage.ps1" -Action stopall

echo.
pause
