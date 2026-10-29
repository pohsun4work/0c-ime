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
function Get-ActiveProfile { $p = (& $updater settings -json | ConvertFrom-Json).profiles; $p.items | Where-Object id -eq $p.active }

# 舊版讀不懂新配方加的規則欄位，讀到會把整份風格設定當成唯讀；整合風格要用要換上的那一版的配方重建，
# 而且在換版之前做，換上的引擎一啟動就讀到。
$old = Get-ActiveProfile
$blend = $old -and $old.source -eq 'blend' -and -not $old.builtin
if ($Rollback) {
    $zuin = Join-Path $env:LOCALAPPDATA 'Zuin'
    $receipt = Get-Content "$zuin\install-receipt.json" -Raw -Encoding UTF8 | ConvertFrom-Json
    $target = Get-Content "$zuin\$($receipt.previous)\previous.json" -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($blend) {
        Invoke-Updater profile select legacy
        Invoke-Updater profile delete $old.id
        & "$zuin\$($target.directory)\0c-update.exe" profile copy blend --name $old.name
        if ($LASTEXITCODE -ne 0) { throw "用 $($target.version) 的配方重建整合風格失敗（$LASTEXITCODE）" }
    }
    Invoke-Updater rollback
    return
}

# 規則沒變就換回原本那份。
$new = $null
if ($blend) {
    Invoke-Updater profile copy blend --name $old.name
    $new = Get-ActiveProfile
    if (($old.rules | ConvertTo-Json -Compress) -eq ($new.rules | ConvertTo-Json -Compress)) {
        Invoke-Updater profile select $old.id
        Invoke-Updater profile delete $new.id
        $new = $null
    }
}
try {
    Invoke-Updater apply -archive $config.archive -sha256 $config.sha256 -channel internal-preview
} catch {
    if ($new) { Invoke-Updater profile select $old.id; Invoke-Updater profile delete $new.id }
    throw
}
if ($new) {
    Invoke-Updater profile delete $old.id
    Write-Host "整合風格的配方有變，已重建（$($new.id)）。"
    foreach ($v in $old.values.PSObject.Properties) {
        $now = $new.values.($v.Name)
        if ($now -ne $v.Value) { Write-Host "  $($v.Name) 原本是 $($v.Value)，回到配方的 $now；要的話到設定改回來。" }
    }
}
if ($config.shell_changed) { Write-Host '這版改到 TSF DLL 或候選窗，請登出再登入。' }
else { Write-Host '這版只改引擎，不用登出（引擎沒換成新版時，上面 0c-update 會說明）。' }
