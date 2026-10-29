# 安裝 local_preview.py 打好的本機包（未簽章的 internal-preview）；整合風格的配方有變時重建風格。
# local_preview.py 把這支複製成 build\local-preview\install.ps1，要裝的套件在旁邊的 bundle\。
# 不是系統管理員時自己跳 UAC：powershell -ExecutionPolicy Bypass -File build\local-preview\install.ps1
param([switch]$Pause)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$shellChanged = $null  # local_preview.py 填入：TSF DLL、候選窗跟上次的 shell 發布比有沒有變

$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) {
    Start-Process powershell -Verb RunAs -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"", '-Pause'
    return
}

$bundle = Join-Path $PSScriptRoot 'bundle'
$updater = Join-Path $bundle '0c-update.exe'
$settings = Join-Path $env:LOCALAPPDATA 'Zuin\settings.json'
function Invoke-Updater { & $updater @args; if ($LASTEXITCODE -ne 0) { throw "0c-update $($args -join ' ') 失敗（$LASTEXITCODE）" } }
function Get-ActiveProfile { $p = (Get-Content $settings -Raw -Encoding UTF8 | ConvertFrom-Json).input_profile; $p.items | Where-Object id -eq $p.active }
function Get-EngineVersion { (& $updater status -json | ConvertFrom-Json).runtime.version }

try {
    Invoke-Updater install -bundle $bundle -channel internal-preview
    # 自訂風格存的是複製當下的規則，配方加了規則要重新複製才會生效。先複製再比，規則沒變就換回原本那份。
    $old = Get-ActiveProfile
    if ($old.source -ne 'blend') {
        Write-Host "目前用的不是整合風格（$($old.name)），風格沒動。"
    } else {
        Invoke-Updater profile copy blend --name $old.name
        $new = Get-ActiveProfile
        if (($old.rules | ConvertTo-Json -Compress) -eq ($new.rules | ConvertTo-Json -Compress)) {
            Invoke-Updater profile select $old.id
            Invoke-Updater profile delete $new.id
            Write-Host '整合風格的配方沒變，沿用原本的風格。'
        } else {
            Invoke-Updater profile delete $old.id
            Write-Host "整合風格的配方有變，已重建（$($new.id)），舊的那份刪掉了。"
            foreach ($v in $old.values.PSObject.Properties) {
                if ($new.values.($v.Name) -ne $v.Value) {
                    Write-Host "  $($v.Name) 原本是 $($v.Value)，回到配方的 $($new.values.($v.Name))；要的話到設定改回來。"
                }
            }
        }
    }
    if ($shellChanged) {
        Write-Host '這版改到 TSF DLL 或候選窗，請登出再登入。'
        return
    }
    # 新引擎由登入工作在背景叫起，可能在風格寫好之前就讀了設定：等它回報新版本，再把風格套用一次。
    $version = (Get-Content (Join-Path $bundle 'release.json') -Raw -Encoding UTF8 | ConvertFrom-Json).version
    for ($i = 0; $i -lt 20 -and (Get-EngineVersion) -ne $version; $i++) { Start-Sleep -Milliseconds 500 }
    if ((Get-EngineVersion) -ne $version) {
        Write-Host "引擎還沒換成 $version（交接時可能正在組字），登出再登入後生效。"
        return
    }
    Invoke-Updater profile select (Get-ActiveProfile).id -apply
    Write-Host "引擎已換成 $version，風格也套用了，不用登出。"
} finally {
    if ($Pause) { Read-Host '按 Enter 關閉' | Out-Null }
}
