$ErrorActionPreference = 'Stop'
$configPath = Join-Path $PSScriptRoot '..\dependencies\ReleaseConfig.psd1'
$config = Import-PowerShellDataFile -LiteralPath $configPath
Write-Output "$($config.Name) $($config.Version)"
