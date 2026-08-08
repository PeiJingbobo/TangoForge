# 停止 TangoForge 守护进程（Windows）
# 用法：scripts\stop-daemon.bat（或 powershell -ExecutionPolicy Bypass -File scripts\stop-daemon.ps1）
#
# 守护进程为 detached 常驻进程（dev-run.ps1 / App 启动拉起），退出 App 后仍在；
# 本脚本按进程名 + 端口 19810 双保险结束其进程。
$ErrorActionPreference = 'Stop'

$Port = 19810
$Url = "http://127.0.0.1:$Port"

# 探活：未运行则直接提示退出。
$alive = $false
try { Invoke-RestMethod -Uri "$Url/ping" -TimeoutSec 1 | Out-Null; $alive = $true } catch { }

if (-not $alive) {
    Write-Host "守护进程未在运行（$Url）"
    exit 0
}

Write-Host "==> 停止守护进程（端口 $Port）"
$stopped = @()

# ① 按进程名。
$procs = Get-Process -Name 'tangoforge-daemon' -ErrorAction SilentlyContinue
foreach ($p in $procs) {
    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    $stopped += $p.Id
}

# ② 按端口兜底（进程名不同/残留监听）。
$conns = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
foreach ($c in $conns) {
    Stop-Process -Id $c.OwningProcess -Force -ErrorAction SilentlyContinue
    $stopped += $c.OwningProcess
}

Start-Sleep -Milliseconds 800

# 确认停止。
$still = $false
try { Invoke-RestMethod -Uri "$Url/ping" -TimeoutSec 1 | Out-Null; $still = $true } catch { }
if ($still) {
    Write-Host "    !! 停止失败，端口 $Port 仍响应"
    exit 1
}
if ($stopped.Count -eq 0) {
    Write-Host "    !! 端口 $Port 无进程可停止（探活成功但未找到进程）"
    exit 1
}
Write-Host "    已停止（PID: $($stopped -join ', ')）"
