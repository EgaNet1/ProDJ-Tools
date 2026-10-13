# Проверяет и собирает ProDJ Tools в один файл ProDJTools.exe.
# Запуск:  .\build.ps1
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\activate.ps1"
Push-Location $PSScriptRoot
try {
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet нашёл проблемы' }

    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'тесты не прошли' }

    # -H windowsgui: без консольного окна; -s -w: без отладочной информации (файл меньше).
    go build -trimpath -ldflags '-s -w -H windowsgui' -o ProDJTools.exe ./cmd/prodj
    if ($LASTEXITCODE -ne 0) { throw 'сборка не удалась' }

    $size = [math]::Round((Get-Item ProDJTools.exe).Length / 1MB, 1)
    Write-Host "Готово: ProDJTools.exe ($size МБ)"
} finally {
    Pop-Location
}
