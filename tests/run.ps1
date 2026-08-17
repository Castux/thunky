# Regression harness for the Thunky front-end and the G-machine backend (PowerShell).
#
# Thunky now has a single execution engine, so this is a GOLDEN harness: for
# every tests/cases/<category>/<name>.þ it runs the engine and checks that the
# combined stdout+stderr and the exit code match the recorded <name>.expected
# (and <name>.exit, when non-zero). The golden files were produced by the
# pre-rewrite tree-walking interpreter (the frozen oracle).
#
# A sibling <name>.in, if present, is fed as standard input (raw bytes, so the
# invalid-UTF-8 case works). Output is captured as raw bytes (via the process
# BaseStream) so no console encoding mangles it. Paths are passed forward-slash,
# repo-root-relative, matching the portable golden files.
#
# Usage:
#   pwsh tests/run.ps1 [category]
#   pwsh tests/run.ps1 -Bless [category]   # (re)generate .expected / .exit
#
# These are distinct from the stdlib unit tests in examples/core_tests.þ.

param(
    [switch]$Bless,
    [string]$Category = ''
)

$ErrorActionPreference = 'Stop'

# Process.Start gives a redirected child's stdin a StreamWriter built from
# Console.InputEncoding, with AutoFlush set — and setting AutoFlush flushes that
# encoding's preamble immediately. On a UTF-8 console (chcp 65001) that is a BOM
# written into every test program's standard input before it has read a byte,
# which the stdin cases duly reported as a leading code point 65279. Swap in a
# preamble-free UTF-8 for the run; ProcessStartInfo.StandardInputEncoding would
# be the direct fix but does not exist in .NET Framework.
$savedInputEncoding = $null
try {
    $savedInputEncoding = [Console]::InputEncoding
    [Console]::InputEncoding = New-Object System.Text.UTF8Encoding($false)
} catch {
    # No console attached (a CI host, say): nothing to correct.
}

$root = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $root
$bin = Join-Path $root 'thunky.test.exe'

Write-Host "building $bin ..."
& go build -o $bin .
if ($LASTEXITCODE -ne 0) { Write-Host 'build failed'; exit 1 }

# Invoke-MF runs the binary, feeding inFile as raw stdin, and returns the combined
# stdout+stderr as a byte array plus the exit code. Reading the raw BaseStream
# avoids any console-encoding transformation.
function Invoke-MF([string]$relPath, [string]$inFile) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $bin
    $psi.Arguments = "`"$relPath`""
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false
    $p = [System.Diagnostics.Process]::Start($psi)
    # Write and close the raw BaseStream, never the StreamWriter wrapping it.
    # Its encoding comes from the console's, and closing it flushes that
    # encoding's preamble — on a UTF-8 console that fed every program a BOM it
    # never sent, which the stdin cases then read as a first code point.
    # ProcessStartInfo.StandardInputEncoding would fix it, but it does not exist
    # in the .NET Framework that Windows PowerShell 5.1 runs on.
    $stdin = $p.StandardInput.BaseStream
    if ($inFile -and (Test-Path $inFile)) {
        $bytes = [System.IO.File]::ReadAllBytes($inFile)
        $stdin.Write($bytes, 0, $bytes.Length)
    }
    $stdin.Flush()
    $stdin.Close()
    $ms = New-Object System.IO.MemoryStream
    $p.StandardOutput.BaseStream.CopyTo($ms)
    $p.StandardError.BaseStream.CopyTo($ms)
    $p.WaitForExit()
    return @{ Bytes = $ms.ToArray(); Code = $p.ExitCode }
}

function Bytes-Equal($a, $b) {
    if ($a.Length -ne $b.Length) { return $false }
    return [System.Linq.Enumerable]::SequenceEqual([byte[]]$a, [byte[]]$b)
}

$pass = 0
$fail = 0
$cur = ''

# The extension is built from its code point rather than written literally.
# Windows PowerShell 5.1 decodes a BOM-less script as the system ANSI codepage,
# which turns a literal 'þ' into a byte that matches no file — the harness then
# found zero cases and cheerfully reported success. This form cannot be
# corrupted by however the script itself is decoded.
$ext = '.' + [char]0x00FE

$cases = @(Get-ChildItem -Path 'tests/cases' -Recurse -File |
    Where-Object { $_.Name.EndsWith($ext, [System.StringComparison]::Ordinal) } |
    Sort-Object FullName)

if ($cases.Count -eq 0) {
    Write-Host "no test cases found under tests/cases (looked for *$ext)"
    Remove-Item $bin -ErrorAction SilentlyContinue
    exit 1
}

$cases | ForEach-Object {
    $catd = Split-Path (Split-Path $_.FullName -Parent) -Leaf
    if ($Category -ne '' -and $catd -ne $Category) { return }
    if ($catd -ne $cur) { $cur = $catd; Write-Host ''; Write-Host "[$cur]" }

    $rel = $_.FullName.Substring($root.Path.Length + 1).Replace('\', '/')
    $base = [System.IO.Path]::ChangeExtension($_.FullName, $null).TrimEnd('.')
    $inFile = "$base.in"
    $expectedFile = "$base.expected"
    $exitFile = "$base.exit"

    $r = Invoke-MF $rel $inFile

    if ($Bless) {
        [System.IO.File]::WriteAllBytes($expectedFile, $r.Bytes)
        if ($r.Code -ne 0) { Set-Content -Path $exitFile -Value $r.Code -NoNewline }
        elseif (Test-Path $exitFile) { Remove-Item $exitFile }
        Write-Host ("  BLESS " + $_.BaseName + " (exit $($r.Code))")
        return
    }

    $expectedExit = 0
    if (Test-Path $exitFile) { $expectedExit = [int](Get-Content -Raw $exitFile) }

    $reasons = @()
    if (-not (Test-Path $expectedFile)) {
        $reasons += "golden(no .expected -- run -Bless)"
    } else {
        $expected = [System.IO.File]::ReadAllBytes($expectedFile)
        if (-not (Bytes-Equal $r.Bytes $expected) -or ($r.Code -ne $expectedExit)) {
            $reasons += "golden(exit $($r.Code), expected $expectedExit)"
        }
    }

    if ($reasons.Count -eq 0) {
        $pass++
        Write-Host ("  PASS  " + $_.BaseName)
    } else {
        $fail++
        Write-Host ("  FAIL  " + $_.BaseName + " -- " + ($reasons -join '; '))
    }
}

Remove-Item $bin -ErrorAction SilentlyContinue
if ($null -ne $savedInputEncoding) { try { [Console]::InputEncoding = $savedInputEncoding } catch { } }

if ($Bless) { Write-Host ''; Write-Host '=== blessed expectations ==='; exit 0 }

Write-Host ''
Write-Host "=== $pass passed, $fail failed ==="
# A run that checked nothing is a failure, not a success: without this, a
# mistyped category reports "0 passed, 0 failed" and exits 0.
if ($pass + $fail -eq 0) { Write-Host "no cases ran (category '$Category' matched nothing)"; exit 1 }
if ($fail -ne 0) { exit 1 }
