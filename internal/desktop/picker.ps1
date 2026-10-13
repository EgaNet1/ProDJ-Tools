# Shows a folder picker and prints the chosen path (nothing if cancelled).
# Prefers the modern Explorer-style dialog; falls back to the classic one.
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

try {
    Add-Type -TypeDefinition $env:PRODJ_PICKER_SOURCE
    $path = [ProDJ.FolderPicker]::Pick($env:PRODJ_PICKER_TITLE, $env:PRODJ_PICKER_START)
} catch {
    Add-Type -AssemblyName System.Windows.Forms
    $owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true }
    $dialog = New-Object System.Windows.Forms.FolderBrowserDialog
    $dialog.Description = $env:PRODJ_PICKER_TITLE
    $dialog.SelectedPath = $env:PRODJ_PICKER_START
    $path = ''
    if ($dialog.ShowDialog($owner) -eq 'OK') { $path = $dialog.SelectedPath }
}
[Console]::Out.Write($path)
