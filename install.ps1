# Installs the latest tern-kata release for Windows: irm https://raw.githubusercontent.com/Dangerboi/tern-kata/main/install.ps1 | iex
# $env:TERN_KATA_BASE_URL replaces the download location (default: the latest GitHub release), $env:TERN_KATA_INSTALL_DIR the install folder.
# $env:TERN_KATA_PLUGIN_ONLY = '1' installs only the Tern window plugin from the release, for the machine that shows the Tern window.

# Installs the plugin folder $Path with tern; a linked copy, which --force leaves alone, gets a message. Returns $false on any other failure.
function Install-TernKataPlugin([string]$Path) {
    # Continue, not the caller's Stop: under Windows PowerShell 5.1 a native command's stderr line is an error record, which Stop throws.
    $ErrorActionPreference = 'Continue'
    $out = (& tern plugin install $Path --force 2>&1 | ForEach-Object { "$_" } | Out-String).Trim()
    if ($LASTEXITCODE -eq 0) {
        Write-Host $out
    } elseif ($out -like '*is linked*') {
        Write-Host "The tern-kata window plugin is linked, so it is left as it is: $out"
    } else {
        Write-Host $out
        return $false
    }
    return $true
}

# One function called last, so a download cut short runs nothing; errors throw (not exit) so `| iex` leaves the shell open.
function Install-TernKata {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    if ($PSVersionTable.PSEdition -eq 'Core' -and -not $IsWindows) {
        throw 'install.ps1: this script supports Windows (on Linux and macOS use install.sh)'
    }
    # Windows PowerShell 5.1 has RuntimeInformation but its OSArchitecture is null there; the environment says it in both.
    # PROCESSOR_ARCHITEW6432 is the machine's CPU when a 32-bit PowerShell runs on 64-bit Windows.
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($cpu) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "install.ps1: unsupported CPU ${_}: tern-kata is released for amd64 and arm64" }
    }

    $base = if ($env:TERN_KATA_BASE_URL) { $env:TERN_KATA_BASE_URL } else { 'https://github.com/Dangerboi/tern-kata/releases/latest/download' }
    $dir = if ($env:TERN_KATA_INSTALL_DIR) { $env:TERN_KATA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\tern-kata' }
    $asset = "tern-kata_windows_$arch.zip"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $pluginOnly = $env:TERN_KATA_PLUGIN_ONLY -eq '1'
        if ($pluginOnly -and -not (Get-Command tern -ErrorAction SilentlyContinue)) {
            throw "install.ps1: tern is not on PATH: install tern-kata's window plugin on the machine that runs Tern"
        }
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
        # Releases before the window plugin was shipped have no plugin folder.
        $plugin = Join-Path $tmp 'plugin'
        $hasPlugin = Test-Path -LiteralPath $plugin -PathType Container
        if ($pluginOnly) {
            if (-not $hasPlugin) { throw 'install.ps1: this release carries no window plugin: nothing was installed' }
            Write-Host 'Installing the Tern window plugin (kata links, the Install tern-kata palette command, startup toasts):'
            if (-not (Install-TernKataPlugin (Join-Path $tmp 'plugin'))) { throw 'install.ps1: could not install the tern-kata window plugin' }
            return
        }
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $exe = Join-Path $dir 'tern-kata.exe'
        # A running tern-kata.exe cannot be overwritten or deleted but can be renamed away, so upgrading it works.
        # An .old still held by a process from an earlier install is renamed aside the same way.
        Get-ChildItem -LiteralPath $dir -Filter 'tern-kata.exe.old-*' -ErrorAction SilentlyContinue |
            ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force -ErrorAction SilentlyContinue }
        if (Test-Path $exe) {
            Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
            if (Test-Path "$exe.old") {
                Move-Item "$exe.old" "$exe.old-$([DateTime]::UtcNow.Ticks)" -Force
            }
            Move-Item $exe "$exe.old" -Force
        }
        Move-Item (Join-Path $tmp 'tern-kata.exe') $exe
        Write-Host "Installed $(& $exe --version) to $exe"
        # The installed binary checks this machine's kata daemon and worker.toml as `tern-kata update` does before a swap; here
        # what it finds is only printed. A release before `--preflight --text` prints nothing on stdout; stdin is piped because
        # releases before 0.5 open a kata Pane for unknown arguments when run inside Tern from a terminal. Continue, not Stop:
        # under Windows PowerShell 5.1 a native command's stderr line is an error record, which Stop throws.
        & {
            $ErrorActionPreference = 'Continue'
            $null | & $exe update --preflight --text 2>$null | ForEach-Object { Write-Warning ("$_" -replace '^warning: ', '') }
        }
        if (-not $hasPlugin) {
            Write-Host 'This release carries no Tern window plugin, so the plugin was left as it is.'
        } elseif (Get-Command tern -ErrorAction SilentlyContinue) {
            Write-Host 'Installing the Tern window plugin (kata links, the Install tern-kata palette command, startup toasts):'
            if (-not (Install-TernKataPlugin (Join-Path $tmp 'plugin'))) { Write-Warning 'Could not install the tern-kata window plugin; rerun this script with $env:TERN_KATA_PLUGIN_ONLY = ''1'' set' }
        } else {
            Write-Host "tern is not on this machine's PATH: on the machine that shows the Tern window, install the window plugin with the plugin-only one-liner in the README (`$env:TERN_KATA_PLUGIN_ONLY)"
        }
    } finally {
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    # Read the raw value so %VAR% entries stay unexpanded, and write it back as REG_EXPAND_SZ.
    $envKey = Get-Item 'HKCU:\Environment'
    $userPath = $envKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    $entries = @(if ($userPath) { $userPath -split ';' | Where-Object { $_ } })
    if (-not ($entries | Where-Object { $_.TrimEnd('\') -eq $dir.TrimEnd('\') })) {
        Set-ItemProperty -Path 'HKCU:\Environment' -Name Path -Value (($entries + $dir) -join ';') -Type ExpandString
        $env:Path = "$env:Path;$dir"
        Write-Host "Added $dir to your user PATH. New Tern tabs and Panes see it at once; shells already open and other programs already running need restarting to see it."
    }
    # Explorer, which starts Tern from the Start menu or taskbar, rereads the user PATH only when told it changed.
    # Sent on every run, so a rerun also fixes a PATH written before this broadcast was added.
    if (-not ('TernKata.Environment' -as [type])) {
        Add-Type -Namespace TernKata -Name Environment -MemberDefinition '[DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint msg, UIntPtr wParam, string lParam, uint flags, uint timeout, out UIntPtr result);'
    }
    $result = [UIntPtr]::Zero
    # HWND_BROADCAST, WM_SETTINGCHANGE "Environment", SMTO_ABORTIFHUNG, 5 s.
    [void][TernKata.Environment]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)

    # npx is the skills CLI's documented runner; bunx runs it the same way where only Bun is installed. npx.cmd, not npx:
    # the npx.ps1 beside it is refused under the Restricted execution policy Windows starts with.
    $run = 'npx.cmd', 'npx.exe', 'bunx.exe' | ForEach-Object { Get-Command $_ -CommandType Application -ErrorAction SilentlyContinue } | Select-Object -First 1
    $skills = if ($run) { "$([IO.Path]::GetFileNameWithoutExtension($run.Name)) skills add Dangerboi/tern-kata -g" } else { 'npx skills add Dangerboi/tern-kata -g (or bunx)' }
    $yes = @(if ($run -and $run.Name -like 'npx*') { '--yes' })
    # Asked only when input comes from the console, so a scripted install never waits on an answer. UserInteractive would
    # also rule out SSH sessions, whose consoles can still answer.
    if ($run -and -not [Console]::IsInputRedirected) {
        $answer = Read-Host "Install the kata and tern-kata agent skills ($skills)? [y/N]"
        if ($answer -match '^[yY]') {
            & $run.Source @yes skills add Dangerboi/tern-kata -g
            if ($LASTEXITCODE -ne 0) { Write-Warning "Could not install the agent skills; run: $skills" }
        } else {
            Write-Host "Skipped the agent skills; to install them later: $skills"
        }
    } else {
        Write-Host "Agent skills for kata and tern-kata (needs Node.js or Bun): $skills"
    }
}

Install-TernKata
