@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo ==== Current status ====
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0manage.ps1" -Action status

echo ==== Stopping RAG service ====
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0manage.ps1" -Action stop

echo.
echo Dependencies (Ollama / Docker) are left running so restart is fast.
echo Use "stop-all.bat" to shut everything down.
echo.
pause
