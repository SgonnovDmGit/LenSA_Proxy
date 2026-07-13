[CmdletBinding()]
param(
    [string]$Version = "0.1.0",
    [string]$Commit = "",
    [string]$OutputDirectory = "dist",
    [string]$ToolchainDirectory = "build\toolchains\go-legacy-win7-1.26.4-1"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$toolchainVersion = "1.26.4-1"
$toolchainURL = "https://github.com/thongtech/go-legacy-win7/releases/download/v1.26.4-1/go-legacy-win7-1.26.4-1.windows_amd64.zip"
$toolchainSHA256 = "f9944a4cace7e72a9f1c96800714a02777e7311f81b6d96da6319cd46d0916be"
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if (-not [IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $root $OutputDirectory
}
if (-not [IO.Path]::IsPathRooted($ToolchainDirectory)) {
    $ToolchainDirectory = Join-Path $root $ToolchainDirectory
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

$resourceGo = (Get-Command go -ErrorAction Stop).Source
$goRoot = Join-Path $ToolchainDirectory "go-legacy-win7"
$legacyGo = Join-Path $goRoot "bin\go.exe"
$archive = "$ToolchainDirectory.zip"
if (-not (Test-Path $legacyGo)) {
    if (Test-Path $ToolchainDirectory) {
        throw "Toolchain directory exists but does not contain go-legacy-win7\bin\go.exe: $ToolchainDirectory"
    }
    New-Item -ItemType Directory -Path (Split-Path $ToolchainDirectory -Parent) -Force | Out-Null
    if (-not (Test-Path $archive)) {
        Invoke-WebRequest -Uri $toolchainURL -OutFile $archive
    }
    $archiveHash = (Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($archiveHash -ne $toolchainSHA256) {
        throw "Legacy toolchain checksum mismatch."
    }
    $temporaryExtraction = "$ToolchainDirectory.extract-$PID"
    try {
        Expand-Archive -Path $archive -DestinationPath $temporaryExtraction
        Move-Item -Path $temporaryExtraction -Destination $ToolchainDirectory
    } finally {
        if (Test-Path $temporaryExtraction) {
            Remove-Item $temporaryExtraction -Recurse -Force
        }
    }
}

$reportedVersion = (& $legacyGo version)
if ($LASTEXITCODE -ne 0 -or $reportedVersion -notmatch "go1\.26\.4") {
    throw "Unexpected legacy toolchain version: $reportedVersion"
}

$resource = Join-Path $root "cmd\lensa-proxy\rsrc_windows_386.syso"
$manifest = Join-Path $root "assets\lensa-proxy.exe.manifest"
$icon = Join-Path $root "assets\lensa-proxy.ico"
$output = Join-Path $OutputDirectory "LenSA_Proxy_windows_legacy_386.exe"
$previous = @{
    GOOS = $env:GOOS
    GOARCH = $env:GOARCH
    CGO_ENABLED = $env:CGO_ENABLED
    GOROOT = $env:GOROOT
    GOTOOLCHAIN = $env:GOTOOLCHAIN
}

try {
    New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
    & $resourceGo run github.com/akavel/rsrc@v0.10.2 -arch 386 -manifest $manifest -ico $icon -o $resource
    if ($LASTEXITCODE -ne 0) {
        throw "Resource generation failed."
    }
    $env:GOOS = "windows"
    $env:GOARCH = "386"
    $env:CGO_ENABLED = "0"
    $env:GOROOT = $goRoot
    $env:GOTOOLCHAIN = "local"
    $ldflags = "-s -w -H=windowsgui -X main.version=$Version -X main.commit=$Commit -X main.packaged=true"
    & $legacyGo -C $root build -trimpath -buildvcs=false -ldflags $ldflags -o $output ./cmd/lensa-proxy
    if ($LASTEXITCODE -ne 0) {
        throw "Legacy build failed."
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
    Toolchain = "go-legacy-win7 $toolchainVersion"
}
