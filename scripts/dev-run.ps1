# TangoForge 桌面端开发启动（Windows）
# 用法：scripts\dev-run.bat           正常启动
#       scripts\dev-run.bat debug    调试模式（打开渲染进程 DevTools）
# 或直接：powershell -ExecutionPolicy Bypass -File scripts\dev-run.ps1 [-Debug]
#
# 守护进程策略（对应 macOS scripts/dev-run.sh）：
#   1) 未运行 → 拉起 bin\tangoforge-daemon.exe（常驻，退出 App 后仍在）
#   2) 运行中 → 复用（不中断连接；如需重启请手动结束进程或重启系统）
param([switch]$Debug)

$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path

# 工具链与镜像（国内网络）
$env:Path = "$Root\node_modules\.bin;$env:Path"
$env:COREPACK_NPM_REGISTRY = 'https://registry.npmmirror.com'
$env:ELECTRON_MIRROR = 'https://npmmirror.com/mirrors/electron/'

$DaemonBin = Join-Path $Root 'bin\tangoforge-daemon.exe'
$DaemonPort = 19810
$DaemonUrl = "http://127.0.0.1:$DaemonPort"

Write-Host "==> 1/3 守护进程检查 ($DaemonUrl)"
if (-not (Test-Path $DaemonBin)) {
    Write-Host "    !! 未找到 $DaemonBin，请先在 Windows 构建（go build -o bin\tangoforge-daemon.exe ./cmd/daemon）"
    exit 1
}

$daemonUp = $false
try { Invoke-RestMethod -Uri "$DaemonUrl/ping" -TimeoutSec 2 | Out-Null; $daemonUp = $true } catch { }

if ($daemonUp) {
    Write-Host "    daemon 已在运行（复用）"
}
else {
    Write-Host "    daemon 未运行，拉起最新二进制（退出 App 后仍常驻）"
    $log = Join-Path $env:TEMP 'tangoforge-daemon.log'
    $err = Join-Path $env:TEMP 'tangoforge-daemon.err.log'
    Start-Process -FilePath $DaemonBin -WindowStyle Hidden `
        -RedirectStandardOutput $log -RedirectStandardError $err
    $ready = $false
    for ($i = 0; $i -lt 10; $i++) {
        Start-Sleep -Milliseconds 500
        try { Invoke-RestMethod -Uri "$DaemonUrl/ping" -TimeoutSec 1 | Out-Null; $ready = $true; break } catch { }
    }
    if (-not $ready) {
        Write-Host "    !! daemon 启动失败，查看 $log / $err"
        exit 1
    }
    Write-Host "    daemon 已就绪"
}

Write-Host "==> 2/3 UI 凭据 (~\.taskboard-app\config.yaml ui_token)"
$cfg = Join-Path $env:USERPROFILE '.taskboard-app\config.yaml'
if ((Test-Path $cfg) -and (Select-String -Path $cfg -Pattern '^ui_token:' -Quiet)) {
    Write-Host "    ui_token 已配置"
}
else {
    Write-Host "    !! 未找到 ui_token，UI 将以受限身份运行"
}

Write-Host "==> 3/3 启动 Electron（electron-vite dev）"
Push-Location (Join-Path $Root 'app')
try {
    if ($Debug) {
        $env:ELECTRON_DEBUG = '1'
        Write-Host "    调试模式：渲染进程 DevTools 将打开"
    }
    corepack pnpm dev
}
finally {
    Pop-Location
}
