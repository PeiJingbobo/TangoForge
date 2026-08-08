@echo off
rem 停止 TangoForge 守护进程（Windows 入口）
rem 用法：scripts\stop-daemon.bat
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0stop-daemon.ps1"
