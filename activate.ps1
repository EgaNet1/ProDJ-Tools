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
