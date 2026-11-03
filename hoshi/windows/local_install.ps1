# local_preview.py 本機包的安裝入口（複製成 build\local-preview\install.ps1，讀旁邊的 install.json）；-Rollback 退回上一版。
param([switch]$Rollback)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) {
    # 以系統管理員重跑自己（跳 UAC），輸出經 cmd 寫到記錄檔再印出來：PowerShell 5.1 的 transcript 收不到外部程式的輸出。
    $log = Join-Path $PSScriptRoot 'install.log'
    Remove-Item $log -ErrorAction SilentlyContinue
    $command = "/c `"powershell -NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`"$(if ($Rollback) { ' -Rollback' }) > `"$log`" 2>&1`""
    $p = Start-Process cmd.exe -ArgumentList $command -Verb RunAs -Wait -PassThru -WindowStyle Hidden
    if (Test-Path $log) { Get-Content $log -Encoding UTF8 }
    exit $p.ExitCode
}

$config = Get-Content (Join-Path $PSScriptRoot 'install.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$updater = Join-Path $PSScriptRoot 'updater\0c-update.exe'
function Invoke-Updater { & $updater @args; if ($LASTEXITCODE -ne 0) { throw "0c-update $($args -join ' ') 失敗（$LASTEXITCODE）" } }
if ($Rollback) {
    Invoke-Updater rollback
    return
}
Invoke-Updater apply -archive $config.archive -sha256 $config.sha256 -channel internal-preview
if ($config.shell_changed) { Write-Host '這版改到 TSF DLL 或候選窗，請登出再登入。' }
else { Write-Host '這版只改引擎，不用登出（引擎沒換成新版時，上面 0c-update 會說明）。' }
