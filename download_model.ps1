# Òfin model downloader — PowerShell equivalent of download_model.sh
# Downloads the baked Òfin GGUF (Llama 3.2 3B Q4_K_M with legal persona).
# Idempotent: skips if the file is already complete.
param()

$ErrorActionPreference = "Stop"
$url = "https://huggingface.co/olamide226/ofin-model/resolve/main/ofin-model.gguf"
$out = "model/ofin-model.gguf"
$minBytes = 1500000000

New-Item -ItemType Directory -Force -Path model | Out-Null

if (Test-Path $out) {
    $size = (Get-Item $out).Length
    if ($size -ge $minBytes) {
        Write-Host "Model already present at $out ($size bytes) — skipping download."
        exit 0
    }
    Write-Host "Partial file found ($size bytes) — resuming download."
}

Write-Host "Downloading Òfin model from Hugging Face (1.88 GB)..."
Invoke-WebRequest -Uri $url -OutFile $out

$size = (Get-Item $out).Length
if ($size -lt $minBytes) {
    Write-Error "Downloaded file is smaller than expected ($size bytes)."
    exit 1
}

Write-Host "Model downloaded to $out ($size bytes)."
