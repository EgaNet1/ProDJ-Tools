# Проверяет и собирает ProDJ Tools в один файл ProDJTools.exe.
# Запуск:  .\build.ps1                  версия берётся из файла VERSION
#          .\build.ps1 -Version 1.2.0
param([string]$Version)
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\activate.ps1"
Push-Location $PSScriptRoot
try {
    if (-not $Version) { $Version = (Get-Content VERSION -Raw).Trim() }
    $Version = $Version.TrimStart('v')
    if ($Version -notmatch '^\d+(\.\d+){1,3}$') { throw "Версия должна быть вида 1.2.3, а не '$Version'" }

    # Иконка, версия и манифест — в ресурсы exe (файл .syso рядом с main.go
    # компилятор подхватывает сам). Иконку перерисовывает assets\genicon.go.
    go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --out cmd\prodj\rsrc --manifest gui `
        --icon assets\icon.ico --file-version $Version --product-version $Version `
        --product-name 'ProDJ Tools' --file-description 'ProDJ Tools' `
        --copyright '© EgaNet1, MIT License' --original-filename ProDJTools.exe
    if ($LASTEXITCODE -ne 0) { throw 'не удалось подготовить ресурсы exe' }

    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet нашёл проблемы' }

    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'тесты не прошли' }

    # -H windowsgui: без консольного окна; -s -w: без отладочной информации (файл меньше).
    go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$Version" -o ProDJTools.exe ./cmd/prodj
    if ($LASTEXITCODE -ne 0) { throw 'сборка не удалась' }

    $size = [math]::Round((Get-Item ProDJTools.exe).Length / 1MB, 1)
    Write-Host "Готово: ProDJTools.exe $Version ($size МБ)"
} finally {
    Pop-Location
}
