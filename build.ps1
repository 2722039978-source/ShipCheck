param([string]$Go = '.\.local\go\bin\go.exe')

$ErrorActionPreference = 'Stop'
Push-Location -LiteralPath $PSScriptRoot
try {
    $compiler = (Get-Command -Name $Go -CommandType Application -ErrorAction Stop).Source
    $local = Join-Path $PSScriptRoot '.local'
    $env:GOTMPDIR = Join-Path $local 'tmp'
    New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
    $env:TEMP = $env:GOTMPDIR
    $env:TMP = $env:GOTMPDIR
    $env:GOENV = 'off'
    $env:GOTOOLCHAIN = 'local'
    $env:GOCACHE = Join-Path $local 'go-cache'
    $env:GOPATH = Join-Path $local 'gopath'
    $env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
    $env:TEST_TELEMETRY_DIR = Join-Path $local 'telemetry'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'

    & $compiler tool rsrc -arch amd64 -manifest '.\resources\shipcheck.manifest' -o '.\shipcheck_manifest_windows_amd64.syso'
    if ($LASTEXITCODE -ne 0) { throw "Windows 清单编译失败，退出码 $LASTEXITCODE" }

    & $compiler build -mod=readonly -trimpath -tags shipcheck_gui -ldflags '-H=windowsgui' -o '.\ShipCheck.exe' .
    if ($LASTEXITCODE -ne 0) { throw "图形版构建失败，退出码 $LASTEXITCODE" }

    & $compiler build -mod=readonly -trimpath -o '.\.local\shipcheck.exe' .
    if ($LASTEXITCODE -ne 0) { throw "命令行版构建失败，退出码 $LASTEXITCODE" }

    Get-Item -LiteralPath '.\ShipCheck.exe','.\.local\shipcheck.exe' | Select-Object -ExpandProperty FullName
} finally {
    Pop-Location
}
