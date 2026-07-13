[CmdletBinding()]
param(
    [string]$Version = "0.1.0",
    [string]$Commit = "",
    [string]$OutputDirectory = "dist"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if (-not [IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $root $OutputDirectory
}
if ($Version -notmatch '^[0-9A-Za-z.+-]+$') {
    throw "Version contains unsupported characters."
}
if ([string]::IsNullOrWhiteSpace($Commit)) {
    $Commit = (& git -C $root rev-parse --short=12 HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($Commit)) {
        $Commit = "unknown"
    }
    $Commit = $Commit.Trim()
}
if ($Commit -notmatch '^[0-9A-Za-z._-]+$') {
    throw "Commit contains unsupported characters."
}

$go = (Get-Command go -ErrorAction Stop).Source
$resource = Join-Path $root "cmd\lensa-proxy\rsrc_windows_amd64.syso"
$manifest = Join-Path $root "assets\lensa-proxy.exe.manifest"
$icon = Join-Path $root "assets\lensa-proxy.ico"
$output = Join-Path $OutputDirectory "LenSA_Proxy_windows_amd64.exe"
$previous = @{
    GOOS = $env:GOOS
    GOARCH = $env:GOARCH
    CGO_ENABLED = $env:CGO_ENABLED
}

try {
    New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
    & $go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -manifest $manifest -ico $icon -o $resource
    if ($LASTEXITCODE -ne 0) {
        throw "Resource generation failed."
    }
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"
    $ldflags = "-s -w -H=windowsgui -X main.version=$Version -X main.commit=$Commit -X main.packaged=true"
    & $go -C $root build -trimpath -buildvcs=false -ldflags $ldflags -o $output ./cmd/lensa-proxy
    if ($LASTEXITCODE -ne 0) {
        throw "Modern build failed."
    }
} finally {
    Remove-Item $resource -Force -ErrorAction SilentlyContinue
    foreach ($name in $previous.Keys) {
        if ($null -eq $previous[$name]) {
            Remove-Item "Env:$name" -ErrorAction SilentlyContinue
        } else {
            Set-Item "Env:$name" $previous[$name]
        }
    }
}

$file = Get-Item $output
$hash = (Get-FileHash $output -Algorithm SHA256).Hash.ToLowerInvariant()
[PSCustomObject]@{
    Path = $file.FullName
    Bytes = $file.Length
    SHA256 = $hash
}
