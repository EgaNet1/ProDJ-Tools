# Собирает установщик dist\ProDJTools-Setup-<версия>.exe.
# Запуск:  .\build-installer.ps1                 версия берётся из файла VERSION
#          .\build-installer.ps1 -Version 1.2.0
# Inno Setup, если его нет в системе, скачивается в папку .tools проекта —
# как и Go в .goenv, в систему ничего не устанавливается.
param([string]$Version)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if (-not $Version) { $Version = (Get-Content VERSION -Raw).Trim() }
    $Version = $Version.TrimStart('v')
    if ($Version -notmatch '^\d+(\.\d+){1,3}$') { throw "Версия должна быть вида 1.2.3, а не '$Version'" }

    & "$PSScriptRoot\build.ps1" -Version $Version

    $iscc = @(
        "$PSScriptRoot\.tools\innosetup\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    ) | Where-Object { Test-Path $_ } | Select-Object -First 1

    if (-not $iscc) {
        Write-Host 'Скачиваю Inno Setup в .tools (один раз)...'
        $tools = Join-Path $PSScriptRoot '.tools'
        New-Item -ItemType Directory -Force $tools | Out-Null
        $setup = Join-Path $tools 'innosetup-setup.exe'
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $ProgressPreference = 'SilentlyContinue'
        Invoke-WebRequest 'https://github.com/jrsoftware/issrc/releases/download/is-6_7_3/innosetup-6.7.3.exe' -OutFile $setup -UseBasicParsing
        $signature = Get-AuthenticodeSignature $setup
        if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=Pyrsys B\.V\.') {
            Remove-Item $setup
            throw 'У скачанного установщика Inno Setup нет действительной подписи, файл удалён'
        }
        # /PORTABLE=1: без записей в реестре, меню «Пуск» и деинсталлятора.
        $dir = Join-Path $tools 'innosetup'
        Start-Process $setup -Wait -ArgumentList '/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART',
            '/CURRENTUSER', '/PORTABLE=1', "/DIR=`"$dir`""
        Remove-Item $setup
        $iscc = Join-Path $dir 'ISCC.exe'
        if (-not (Test-Path $iscc)) { throw 'Не удалось установить Inno Setup' }
    }

    # Вторая попытка — на случай, если антивирус ещё держит только что собранный exe.
    foreach ($attempt in 1, 2) {
        & $iscc /Qp "/DAppVersion=$Version" "/DSourceExe=$PSScriptRoot\ProDJTools.exe" installer\ProDJTools.iss
        if ($LASTEXITCODE -eq 0) { break }
        if ($attempt -eq 2) { throw 'сборка установщика не удалась' }
        Start-Sleep 3
    }

    $out = Get-Item "dist\ProDJTools-Setup-$Version.exe"
    Write-Host "Готово: $($out.FullName) ($([math]::Round($out.Length / 1MB, 1)) МБ)"
} finally {
    Pop-Location
}
