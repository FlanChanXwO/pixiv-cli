param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('amd64', 'arm64')]
    [string]$GoArch
)

$ErrorActionPreference = 'Stop'

# GitHub Windows runner 当前不自带 sqlite3 CLI；browser evidence 的真实数据库 contract
# 依赖该命令，因此固定使用 SQLite 官方 3.53.4 工具包并校验下载字节的 SHA-256。
# 这些 SHA-256 pin 来自已用 SQLite 下载页公布的 SHA3-256 交叉核对过的官方包：
# x64=88b4659fe747896b853af10157316b4ade143553efb89c1c8ca7423a278dcc8b，
# arm64=0c99da3702b2517c1d738207db7e945e5c55be7748141a192a1c8f3b4455c44b。
$packages = @{
    amd64 = @{
        Url = 'https://www.sqlite.org/2026/sqlite-tools-win-x64-3530400.zip'
        Sha256 = 'f46ee2475de4cbe287e6e5f7d43c838796b14e7379cd216bdbb28d391429f9fc'
    }
    arm64 = @{
        Url = 'https://www.sqlite.org/2026/sqlite-tools-win-arm64-3530400.zip'
        Sha256 = '8a7c30165f6e9b054fbbe5ba6048acf23c967fd76955f7a5d66dc519542d3393'
    }
}

if ([string]::IsNullOrWhiteSpace($env:RUNNER_TEMP) -or [string]::IsNullOrWhiteSpace($env:GITHUB_PATH)) {
    throw 'RUNNER_TEMP and GITHUB_PATH are required in GitHub Actions'
}

$package = $packages[$GoArch]
$archive = Join-Path $env:RUNNER_TEMP "sqlite-tools-$GoArch.zip"
$install = Join-Path $env:RUNNER_TEMP "sqlite-tools-$GoArch"

curl.exe --fail --location $package.Url --output $archive
if ($LASTEXITCODE -ne 0) {
    throw "SQLite package download failed with exit code $LASTEXITCODE"
}

$actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archive).Hash.ToLowerInvariant()
if ($actual -cne $package.Sha256) {
    throw "SQLite package SHA-256 mismatch for $GoArch"
}

if (Test-Path -LiteralPath $install) {
    Remove-Item -LiteralPath $install -Recurse -Force
}
Expand-Archive -LiteralPath $archive -DestinationPath $install -Force
Remove-Item -LiteralPath $archive -Force

$sqlite = Join-Path $install 'sqlite3.exe'
if (-not (Test-Path -LiteralPath $sqlite -PathType Leaf)) {
    throw 'sqlite3.exe was not found after package extraction'
}

& $sqlite --version
if ($LASTEXITCODE -ne 0) {
    throw "sqlite3 --version failed with exit code $LASTEXITCODE"
}

$install | Out-File -FilePath $env:GITHUB_PATH -Encoding utf8 -Append
