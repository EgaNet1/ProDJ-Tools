# Изолированное Go-окружение проекта (аналог Python venv).
# Использование:  . .\activate.ps1   (точка в начале обязательна)
# После этого в текущем окне PowerShell команда `go` берётся из .goenv,
# а модули и кэш сборки складываются туда же — система не затрагивается.

$env_root = Join-Path $PSScriptRoot ".goenv"

$env:GOROOT      = Join-Path $env_root "go"
$env:GOPATH      = Join-Path $env_root "gopath"
$env:GOMODCACHE  = Join-Path $env_root "gopath\pkg\mod"
$env:GOCACHE     = Join-Path $env_root "cache"
$env:GOTOOLCHAIN = "local"   # не скачивать другие версии Go
$env:CGO_ENABLED = "0"       # проект на чистом Go, C-компилятор не нужен

# При первом запуске (например, сразу после git clone) скачать ту версию Go,
# что указана в go.mod, и проверить её контрольную сумму.
if (-not (Test-Path (Join-Path $env:GOROOT "bin\go.exe"))) {
    $go_version = (Select-String -Path (Join-Path $PSScriptRoot "go.mod") -Pattern '^go\s+(\S+)').Matches[0].Groups[1].Value
    $go_arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    $go_file = "go$go_version.windows-$go_arch.zip"
    Write-Host "Скачиваю $go_file в .goenv (один раз)..."

    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    $saved_progress = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"   # иначе Invoke-WebRequest в PowerShell 5 качает в разы медленнее
    $release = (Invoke-RestMethod "https://go.dev/dl/?mode=json&include=all") |
        ForEach-Object { $_.files } | Where-Object { $_.filename -eq $go_file } | Select-Object -First 1
    if (-not $release) { throw "На go.dev нет $go_file" }

    New-Item -ItemType Directory -Force $env_root | Out-Null
    $zip = Join-Path $env_root $go_file
    Invoke-WebRequest "https://go.dev/dl/$go_file" -OutFile $zip -UseBasicParsing
    if ((Get-FileHash $zip -Algorithm SHA256).Hash -ne $release.sha256) {
        Remove-Item $zip
        throw "Контрольная сумма $go_file не совпала, файл удалён"
    }
    # tar.exe есть в Windows 10+ и распаковывает Go намного быстрее Expand-Archive.
    & "$env:SystemRoot\System32\tar.exe" -xf $zip -C $env_root
    Remove-Item $zip
    $ProgressPreference = $saved_progress
    if (-not (Test-Path (Join-Path $env:GOROOT "bin\go.exe"))) { throw "Не удалось распаковать $go_file" }
}

$go_bin = Join-Path $env:GOROOT "bin"
if (-not ($env:Path.Split(';') -contains $go_bin)) {
    $env:Path = "$go_bin;$env:Path"
}

function global:deactivate {
    $env:Path = ($env:Path.Split(';') | Where-Object { $_ -ne $go_bin }) -join ';'
    Remove-Item Env:GOROOT, Env:GOPATH, Env:GOMODCACHE, Env:GOCACHE, Env:GOTOOLCHAIN, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Function:deactivate
}

Write-Host "ProDJTools: $(go version)"
