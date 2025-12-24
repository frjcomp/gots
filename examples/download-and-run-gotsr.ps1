#=== 1) Determine latest release via GitHub REST API ===
$owner = "frjcomp"
$repo  = "gots"

Write-Host "Fetching latest release info..."
$apiUrl = "https://api.github.com/repos/$owner/$repo/releases/latest"
$releaseJson = Invoke-WebRequest -Uri $apiUrl -UseBasicParsing `
    -Headers @{ "User-Agent" = "PowerShell" } | ConvertFrom-Json

$tagName = $releaseJson.tag_name
if (-not $tagName) {
    Write-Error "Could not determine latest release tag!"
    exit 1
}

Write-Host "Latest tag is: $tagName"

#=== 2) Construct download URL for Windows AMD64 zip ===
$assetName = "gots_gotsr_windows_amd64.zip"
$downloadUrl = "https://github.com/$owner/$repo/releases/download/$tagName/$assetName"

Write-Host "Downloading $assetName from $downloadUrl ..."
$tempZip = Join-Path $env:TEMP $assetName

Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip -UseBasicParsing `
    -Headers @{ "User-Agent" = "PowerShell" }

#=== 3) Extract the ZIP ===
$extractDir = Join-Path $env:TEMP ("gotsr-$tagName")
Write-Host "Extracting to $extractDir ..."
Remove-Item -Path $extractDir -Recurse -Force -ErrorAction SilentlyContinue
Expand-Archive -Path $tempZip -DestinationPath $extractDir -Force

#=== 4) Run the gotsr binary ===
$exePath = Join-Path $extractDir "gotsr.exe"
if (-not (Test-Path $exePath)) {
    Write-Error "gotsr.exe not found after extraction!"
    exit 1
}

Write-Host "Running gotsr ..."
& $exePath --target example.com:9001 --retries 3

#=== 5) Clean up (optional) ===
Remove-Item -Path $tempZip -Force
# Remove-Item -Path $extractDir -Recurse -Force
Write-Host "Done."
