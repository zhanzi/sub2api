param(
    [string]$Registry = "devtest.pointlife365.net:5180",
    [string]$RegistryNamespace = "slzr",
    [string]$ImageName = "sub2api",
    [string]$RegistryUser = "slzr",
    [string]$PasswordFile = "",
    [string]$VersionTag = "",
    [string]$GoProxy = "https://goproxy.cn,direct",
    [string]$GoSumDb = "sum.golang.google.cn"
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..")

if ([string]::IsNullOrWhiteSpace($PasswordFile)) {
    $PasswordFile = Join-Path $scriptDir "registry-password.txt"
}

if ([string]::IsNullOrWhiteSpace($VersionTag)) {
    $gitSha = git -C $repoRoot rev-parse --short=8 HEAD
    $buildDate = Get-Date -Format "yyyyMMdd"
    $VersionTag = "product-zhanzi-$buildDate-$gitSha"
}

$imageRepo = "$Registry/$RegistryNamespace/$ImageName"
$versionImage = "${imageRepo}:$VersionTag"
$latestImage = "${imageRepo}:latest"

if (-not (Test-Path -LiteralPath $PasswordFile)) {
    throw "Missing registry password file: $PasswordFile. Create it locally and do not commit it."
}

Write-Host "Logging in to $Registry as $RegistryUser"
Get-Content -LiteralPath $PasswordFile -Raw | docker login $Registry --username $RegistryUser --password-stdin

Write-Host "Building $versionImage"
docker build `
    -t $versionImage `
    -t $latestImage `
    --build-arg "GOPROXY=$GoProxy" `
    --build-arg "GOSUMDB=$GoSumDb" `
    -f (Join-Path $repoRoot "Dockerfile") `
    $repoRoot

Write-Host "Pushing $versionImage"
docker push $versionImage

Write-Host "Pushing $latestImage"
docker push $latestImage

Write-Host ""
Write-Host "Done."
Write-Host ""
Write-Host "Immutable image:"
Write-Host "  $versionImage"
Write-Host ""
Write-Host "Latest image:"
Write-Host "  $latestImage"
Write-Host ""
Write-Host "Production .env example:"
Write-Host "  SUB2API_IMAGE=$versionImage"
Write-Host ""
Write-Host "Local/test .env example:"
Write-Host "  SUB2API_IMAGE=$latestImage"
