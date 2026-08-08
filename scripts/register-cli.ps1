# 将 TangoForge CLI 注册到当前用户 PATH（幂等）
# 用法：powershell -ExecutionPolicy Bypass -File register-cli.ps1
# 随 App 分发于 resources\bin\；注册后新开终端即可全局使用 tangoforge。
$ErrorActionPreference = 'Stop'

$Bin = $PSScriptRoot
$Cli = Join-Path $Bin 'tangoforge.exe'
if (-not (Test-Path $Cli)) {
    # 仓库场景：脚本在 scripts/，CLI 在仓库根 bin/。
    $repoBin = Join-Path $Bin '..\bin'
    if (Test-Path (Join-Path $repoBin 'tangoforge.exe')) {
        $Bin = (Resolve-Path $repoBin).Path
        $Cli = Join-Path $Bin 'tangoforge.exe'
    }
}
if (-not (Test-Path $Cli)) {
    Write-Host "!! 未找到 CLI：$Cli"
    exit 1
}

# 仅修改 User 级 PATH（不动系统级，无需管理员）。
$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$Entries = @($UserPath -split ';' | Where-Object { $_ -ne '' })
if ($Entries -contains $Bin) {
    Write-Host "已注册（跳过）：$Bin"
    exit 0
}

$NewPath = (($Entries + $Bin) -join ';')
[Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
# 当前进程同步生效（新终端也自动生效）。
$env:Path = "$Bin;$env:Path"

Write-Host "已注册 CLI 到用户 PATH：$Bin"
Write-Host "验证：tangoforge --help （需守护进程运行；App 启动或 scripts\dev-run.bat 会自动拉起）"
