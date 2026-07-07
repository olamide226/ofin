# Running Òfin on Windows

Òfin's toolchain is Linux/macOS-native (bash, Make, Go with CGo, llama.cpp).
The recommended path is **WSL2** (Windows Subsystem for Linux) — it's free,
built into Windows 10/11, and gives you a real Linux environment without a VM.

**Don't try native Windows** — the Go CGo build needs `libsqlite3-dev` and the
Makefile assumes bash. WSL2 takes 10 minutes to set up and everything just works.

## Option A: WSL2 (recommended — 10 min)

### 1. Install WSL2

Open PowerShell as Administrator and run:

```powershell
wsl --install -d Ubuntu-24.04
```

Restart when prompted. After reboot, Ubuntu will open — create a username and password.

### 2. Install tools inside WSL

```bash
sudo apt update && sudo apt install -y git curl unzip libsqlite3-dev

# Download llama.cpp binaries (needed at runtime)
# Get the latest llama.cpp release from:
# https://github.com/ggml-org/llama.cpp/releases
# Or build from source if you prefer.
```

### 3. Clone and get the model

```bash
git clone https://github.com/olamide226/ofin.git
cd ofin
bash download_model.sh    # downloads the GGUF (~1.9 GB)
```

### 4. Download the pre-built binary (no Go needed)

```bash
curl -L -o ofin https://github.com/olamide226/ofin/releases/download/v0.1.0/ofin-linux
chmod +x ofin
```

### 5. Ask a question

```bash
./ofin ask "How much notice after 3 years of service?"
# Web UI:
./ofin serve    # → http://127.0.0.1:8090
```

### 6. Stop background processes

```bash
./ofin stop
```

### Building from source (developers only)

```bash
sudo snap install go --classic
cd engine
go build -tags sqlite_fts5 -o bin/ofin ./cmd/ofin
```

## Option B: Native Windows (PowerShell — not recommended)

If you absolutely can't use WSL, here's the PowerShell path. It's untested
and several steps may need adjustment.

### 1. Download the model

```powershell
.\download_model.ps1
```

Or manually:
```powershell
$url = "https://huggingface.co/olamide226/ofin-model/resolve/main/ofin-model.gguf"
New-Item -ItemType Directory -Force -Path model
Invoke-WebRequest -Uri $url -OutFile model/ofin-model.gguf
```

### 2. Get the binary

Download `ofin-linux` from [GitHub Releases](https://github.com/olamide226/ofin/releases).
There's no native Windows `.exe` yet — use WSL2 instead.

### 3. Run (in WSL)

```bash
./ofin serve    # → http://127.0.0.1:8090
```

Note: `ofin stop` (which runs `pkill` internally) won't work on native Windows.
Stop the background llama-server processes via Task Manager instead.

## Troubleshooting

**"ofin: starting llama-server: exec: 'llama-server': executable file not found"**

You need llama.cpp. The Òfin repo includes pre-built llama.cpp binaries at
`/opt/llama.cpp/` on Linux. On Windows/WSL, download from
https://github.com/ggml-org/llama.cpp/releases and place `llama-server` and
`llama-bench` in your PATH.

**"go build: CGo linker errors"**

```
sudo apt install libsqlite3-dev   # Linux/WSL
```

**"model not found"**

Run `bash download_model.sh` — the model is 1.88 GB from Hugging Face.

**Server starts but curl/Chrome can't connect**

Make sure you're using `http://127.0.0.1:8090` (not `localhost` — WSL2 sometimes
has hostname resolution issues). The server binds to 127.0.0.1 only.
