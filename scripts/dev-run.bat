@echo off
rem TangoForge 桌面端开发启动（Windows 入口）
rem 用法：scripts\dev-run.bat [debug]
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-run.ps1" %*
