param([switch]$Integration)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Push-Location -LiteralPath $PSScriptRoot
try {
    $localDirectory = Join-Path $PSScriptRoot '.local'
    $env:GOTMPDIR = Join-Path $localDirectory 'tmp'
    New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
    $env:TEMP = $env:GOTMPDIR
    $env:TMP = $env:GOTMPDIR
    $env:GOENV = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:GOCACHE = Join-Path $localDirectory 'go-cache'
    $env:GOPATH = Join-Path $localDirectory 'gopath'
    $env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
    $env:TEST_TELEMETRY_DIR = Join-Path $localDirectory 'telemetry'
    $goExecutable = Join-Path $localDirectory 'go\bin\go.exe'
    & $goExecutable test -count=1 -v ./...
    if ($LASTEXITCODE -ne 0) { throw "Go 测试失败，退出码 $LASTEXITCODE" }
    & $goExecutable test -tags shipcheck_gui -run TestGUICommand -count=1 -v ./...
    if ($LASTEXITCODE -ne 0) { throw "窗口命令测试失败，退出码 $LASTEXITCODE" }
    & $goExecutable vet ./...
    if ($LASTEXITCODE -ne 0) { throw "Go 检查失败，退出码 $LASTEXITCODE" }
    & $goExecutable vet -tags shipcheck_gui ./...
    if ($LASTEXITCODE -ne 0) { throw "窗口代码检查失败，退出码 $LASTEXITCODE" }
    if ($Integration) {
        & .\build.ps1
        & .\tests\accept-extended.ps1
    }
} finally {
    Pop-Location
}
