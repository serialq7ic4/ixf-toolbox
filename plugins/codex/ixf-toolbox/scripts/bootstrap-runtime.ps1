[CmdletBinding()]
param(
    [switch]$DryRun,
    [switch]$Apply,
    [string]$InstallDir
)

$ErrorActionPreference = "Stop"
$ReleaseVersion = "3.27.0"
$Repository = "serialq7ic4/ixf-toolbox"

try {
    if (-not ("IxfToolboxBootstrap.NativePath" -as [type])) {
        Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;

namespace IxfToolboxBootstrap {
    public static class NativePath {
        private const uint FileShareRead = 0x00000001;
        private const uint FileShareWrite = 0x00000002;
        private const uint FileShareDelete = 0x00000004;
        private const uint OpenExisting = 3;
        private const uint FileFlagBackupSemantics = 0x02000000;
        private static readonly IntPtr InvalidHandleValue = new IntPtr(-1);

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern IntPtr CreateFileW(
            string fileName,
            uint desiredAccess,
            uint shareMode,
            IntPtr securityAttributes,
            uint creationDisposition,
            uint flagsAndAttributes,
            IntPtr templateFile);

        [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern uint GetFinalPathNameByHandleW(
            IntPtr file,
            StringBuilder path,
            uint pathLength,
            uint flags);

        [DllImport("kernel32.dll", SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool CloseHandle(IntPtr handle);

        public static string Resolve(string path) {
            IntPtr handle = CreateFileW(
                path,
                0,
                FileShareRead | FileShareWrite | FileShareDelete,
                IntPtr.Zero,
                OpenExisting,
                FileFlagBackupSemantics,
                IntPtr.Zero);
            if (handle == InvalidHandleValue) {
                throw new Win32Exception(Marshal.GetLastWin32Error());
            }

            try {
                StringBuilder buffer = new StringBuilder(512);
                uint length = GetFinalPathNameByHandleW(handle, buffer, (uint)buffer.Capacity, 0);
                if (length == 0) {
                    throw new Win32Exception(Marshal.GetLastWin32Error());
                }
                if (length >= (uint)buffer.Capacity) {
                    buffer = new StringBuilder(checked((int)length));
                    length = GetFinalPathNameByHandleW(handle, buffer, (uint)buffer.Capacity, 0);
                    if (length == 0 || length >= (uint)buffer.Capacity) {
                        throw new Win32Exception(Marshal.GetLastWin32Error());
                    }
                }

                string resolved = buffer.ToString();
                if (resolved.StartsWith(@"\\?\UNC\", StringComparison.OrdinalIgnoreCase)) {
                    return @"\\" + resolved.Substring(8);
                }
                if (resolved.StartsWith(@"\\?\", StringComparison.OrdinalIgnoreCase)) {
                    return resolved.Substring(4);
                }
                return resolved;
            }
            finally {
                CloseHandle(handle);
            }
        }
    }
}
"@
    }

    if ($DryRun -and $Apply) {
        throw "-DryRun and -Apply are mutually exclusive"
    }
    $IsApply = [bool]$Apply
    $IsDryRun = -not $IsApply

    $LocalRoot = [Environment]::GetEnvironmentVariable("LOCALAPPDATA")
    if ([string]::IsNullOrWhiteSpace($LocalRoot) -or -not (Test-Path -LiteralPath $LocalRoot -PathType Container)) {
        throw "LOCALAPPDATA must name an existing current user directory"
    }
    $RootReal = [IxfToolboxBootstrap.NativePath]::Resolve($LocalRoot).TrimEnd('\', '/')
    if ([string]::IsNullOrWhiteSpace($InstallDir)) {
        $InstallDir = Join-Path $LocalRoot "ixf-toolbox\bin"
    }
    elseif (-not [IO.Path]::IsPathRooted($InstallDir)) {
        $InstallDir = Join-Path (Get-Location).Path $InstallDir
    }
    if (($InstallDir -split '[\\/]') -contains '..') {
        throw "install directory cannot contain traversal and must stay inside the current user directory"
    }
    $InstallDir = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\', '/')

    $Probe = $InstallDir
    $Suffix = [Collections.Generic.List[string]]::new()
    while (-not (Test-Path -LiteralPath $Probe)) {
        $Name = [IO.Path]::GetFileName($Probe)
        if ([string]::IsNullOrEmpty($Name)) {
            throw "cannot resolve install directory"
        }
        $Suffix.Insert(0, $Name)
        $Parent = [IO.Path]::GetDirectoryName($Probe)
        if ([string]::IsNullOrEmpty($Parent) -or $Parent -eq $Probe) {
            throw "cannot resolve install directory"
        }
        $Probe = $Parent
    }
    if (-not (Test-Path -LiteralPath $Probe -PathType Container)) {
        throw "install directory ancestor is not a directory"
    }
    $InstallReal = [IxfToolboxBootstrap.NativePath]::Resolve($Probe)
    foreach ($Part in $Suffix) {
        $InstallReal = Join-Path $InstallReal $Part
    }
    $InstallReal = [IO.Path]::GetFullPath($InstallReal).TrimEnd('\', '/')
    $RootPrefix = $RootReal + [IO.Path]::DirectorySeparatorChar
    if ($InstallReal -ne $RootReal -and -not $InstallReal.StartsWith($RootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "install directory must resolve inside the current user directory"
    }

    $Architecture = [Runtime.InteropServices.RuntimeInformation]::ProcessArchitecture.ToString().ToLowerInvariant()
    if ($Architecture -ne "x64") {
        throw "unsupported Windows architecture: $Architecture"
    }
    $Asset = "ixf_${ReleaseVersion}_windows_amd64.exe"
    $ChecksumAsset = "ixf_${ReleaseVersion}_checksums.txt"
    $ReleaseBase = "https://github.com/${Repository}/releases/download/v${ReleaseVersion}"
    if (-not [string]::IsNullOrWhiteSpace($env:IXF_BOOTSTRAP_RELEASE_BASE_URL)) {
        if ($env:IXF_BOOTSTRAP_TESTING -ne "1") {
            if (-not $env:IXF_BOOTSTRAP_RELEASE_BASE_URL.StartsWith("https://", [StringComparison]::OrdinalIgnoreCase)) {
                throw "release URL override must use HTTPS unless IXF_BOOTSTRAP_TESTING=1"
            }
            throw "release URL override is test-only"
        }
        $ReleaseBase = $env:IXF_BOOTSTRAP_RELEASE_BASE_URL.TrimEnd('/')
    }
    $ReleaseURI = [Uri]$ReleaseBase
    if ($ReleaseURI.Scheme -ne "https" -and $env:IXF_BOOTSTRAP_TESTING -ne "1") {
        throw "release URL must use HTTPS"
    }
    $TargetPath = Join-Path $InstallDir "ixf.exe"

    function Write-Result {
        [ordered]@{
            ok = $true
            dryRun = $IsDryRun
            apply = $IsApply
            version = $ReleaseVersion
            asset = $Asset
            releaseHost = $ReleaseURI.Authority
            targetPath = $TargetPath
        } | ConvertTo-Json -Compress
    }

    if (-not $IsApply) {
        Write-Result
        exit 0
    }

    $TempDir = Join-Path ([IO.Path]::GetTempPath()) ("ixf-toolbox-bootstrap-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $TempDir | Out-Null
    try {
        $DownloadedAsset = Join-Path $TempDir $Asset
        $DownloadedChecksums = Join-Path $TempDir $ChecksumAsset
        Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/$Asset" -OutFile $DownloadedAsset
        Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/$ChecksumAsset" -OutFile $DownloadedChecksums

        $Pattern = '^([0-9A-Fa-f]{64})\s+\*?' + [Regex]::Escape($Asset) + '$'
        $ChecksumMatches = @(Get-Content -LiteralPath $DownloadedChecksums | ForEach-Object {
            if ($_ -match $Pattern) { $Matches[1].ToLowerInvariant() }
        })
        if ($ChecksumMatches.Count -ne 1) {
            throw "checksum file does not contain exactly one entry for $Asset"
        }
        $ActualChecksum = (Get-FileHash -Algorithm SHA256 -LiteralPath $DownloadedAsset).Hash.ToLowerInvariant()
        if ($ActualChecksum -ne $ChecksumMatches[0]) {
            throw "checksum mismatch for $Asset"
        }

        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        $TargetTemp = Join-Path $InstallDir (".ixf." + [Guid]::NewGuid().ToString("N") + ".tmp")
        try {
            Copy-Item -LiteralPath $DownloadedAsset -Destination $TargetTemp
            Move-Item -LiteralPath $TargetTemp -Destination $TargetPath -Force
        }
        finally {
            if (Test-Path -LiteralPath $TargetTemp) {
                Remove-Item -LiteralPath $TargetTemp -Force
            }
        }
    }
    finally {
        if (Test-Path -LiteralPath $TempDir) {
            Remove-Item -LiteralPath $TempDir -Recurse -Force
        }
    }

    Write-Result
}
catch {
    [Console]::Error.WriteLine("ERROR " + $_.Exception.Message)
    exit 1
}
