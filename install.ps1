# Installs the latest tern-kata release for Windows: irm https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.ps1 | iex
# $env:TERN_KATA_BASE_URL replaces the download location (default: the latest GitHub release), $env:TERN_KATA_INSTALL_DIR the install folder.

# One function called last, so a download cut short runs nothing; errors throw (not exit) so `| iex` leaves the shell open.
function Install-TernKata {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
        throw 'install.ps1: this script supports Windows (on Linux and macOS use install.sh)'
    }
    $arch = switch ([Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
        'X64' { 'amd64' }
        'Arm64' { 'arm64' }
        default { throw "install.ps1: unsupported CPU $_: tern-kata is released for amd64 and arm64" }
    }

    $base = if ($env:TERN_KATA_BASE_URL) { $env:TERN_KATA_BASE_URL } else { 'https://github.com/Dangerboi/tern-kata/releases/latest/download' }
    $dir = if ($env:TERN_KATA_INSTALL_DIR) { $env:TERN_KATA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\tern-kata' }
    $asset = "tern-kata_windows_$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $asset"
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

        $want = $null
        foreach ($line in Get-Content (Join-Path $tmp 'checksums.txt')) {
            $fields = $line.Trim() -split '\s+'
            if ($fields.Count -eq 2 -and $fields[1] -eq $asset) { $want = $fields[0] }
        }
        if (-not $want) { throw "install.ps1: checksums.txt has no entry for $asset" }
        $got = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $tmp $asset)).Hash
        if ($got -ne $want) { throw "install.ps1: checksum mismatch for $asset (expected $want, got $got); nothing was installed" }

        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $exe = Join-Path $dir 'tern-kata.exe'
        # A running tern-kata.exe cannot be overwritten but can be renamed away, so upgrading it works.
        if (Test-Path $exe) {
            Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
            Move-Item $exe "$exe.old" -Force
        }
        Move-Item (Join-Path $tmp 'tern-kata.exe') $exe
        Write-Host "Installed $(& $exe --version) to $exe"
    } finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @(if ($userPath) { $userPath -split ';' | Where-Object { $_ } })
    if (-not ($entries | Where-Object { $_.TrimEnd('\') -eq $dir.TrimEnd('\') })) {
        [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
        $env:Path = "$env:Path;$dir"
        Write-Host "Added $dir to your user PATH; shells that are already open need restarting to see it."
    }

    if (Get-Command tern -ErrorAction SilentlyContinue) {
        tern plugin install github.com/Dangerboi/tern-kata --force
        if ($LASTEXITCODE -ne 0) { Write-Warning 'Could not install the tern-kata window plugin; run: tern plugin install github.com/Dangerboi/tern-kata --force' }
    } else {
        Write-Host "tern is not on this machine's PATH: install the window plugin on the machine that shows the Tern window: tern plugin install github.com/Dangerboi/tern-kata"
    }
}

Install-TernKata
