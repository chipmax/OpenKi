# Local release helper: build with version injection, then create a GitHub release.
# Usage:  powershell -File tools\release.ps1 -Version 1.2.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Version
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

$tag = "v$Version"
# No -s -w: Huorong deletes a stripped Go exe within about a minute of it
# appearing on this machine, which is what the freshly built loimo.exe is.
go build -trimpath -ldflags "-H=windowsgui -X github.com/chipmax/loimo/internal/gateway.Version=$Version -X github.com/chipmax/loimo/internal/update.Current=$Version" -o dist/loimo.exe ./cmd/loimo
if ($LASTEXITCODE -ne 0) { throw "build failed" }
go build -trimpath -o dist/loimostats.exe ./cmd/loimostats
if ($LASTEXITCODE -ne 0) { throw "build failed" }

Copy-Item README.md dist/ -Force
Compress-Archive -Path dist/loimo.exe, dist/README.md -DestinationPath "dist/loimo-$Version.zip" -Force

$assets = @("dist/loimo.exe", "dist/loimostats.exe", "dist/loimo-$Version.zip")
# Re-running a version that already went out used to die in gh release create;
# upload over the existing assets instead.
if (gh release view $tag 2>$null) {
    gh release upload $tag $assets --clobber
    gh release edit $tag --latest
} else {
    gh release create $tag $assets --title $tag --generate-notes --latest
}
Write-Host "release $tag published"
