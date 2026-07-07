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
sudo apt update && sudo apt install -y build-essential cmake git curl unzip libsqlite3-dev

# Go 1.21+ (if not already installed)
sudo snap install go --classic
# or: wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz && sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
# Add to ~/.bashrc: export PATH=/usr/local/go/bin:$PATH

# Verify
go version  # should print go1.21.x or later
```

### 3. Clone and build Òfin

```bash
git clone https://github.com/olamide226/ofin.git
cd ofin
bash download_model.sh    # downloads the GGUF (~1.9 GB)

# Build the CLI
cd engine
go build -tags sqlite_fts5 -o bin/ofin ./cmd/ofin
cd ..
```

### 4. The corpus DB is pre-built

The repo includes `data/ofin.db` (the pre-built SQLite-vec + FTS5 database).
You don't need to run the Python pipeline unless you're modifying the corpus.
If you DO need to rebuild it:

```bash
sudo apt install -y python3-venv python3-pip
python3 -m venv .venv
.venv/bin/pip install -r pipeline/requirements.txt
make chunk sac ingest   # 'make sac' needs GOOGLE_API_KEY
```

### 5. Ask a question

```bash
engine/bin/ofin ask "How much notice after 3 years of service?"
# Web UI:
engine/bin/ofin serve    # → http://127.0.0.1:8090
```

### 6. Stop background processes

```bash
engine/bin/ofin stop
```

## Option B: Native Windows (PowerShell — not recommended)

If you absolutely can't use WSL, here's the PowerShell path. It's untested
and several steps may need adjustment.

### Prerequisites

```powershell
# Install Go from https://go.dev/dl/
# Install CMake from https://cmake.org/download/
# Install Git from https://git-scm.com/download/win
# Install sqlite3.dll and dev headers (MSYS2 recommended):
#   https://www.msys2.org/ → pacman -S mingw-w64-x86_64-sqlite3
```

### Download the model

```powershell
# PowerShell equivalent of download_model.sh
$url = "https://huggingface.co/olamide226/ofin-model/resolve/main/ofin-model.gguf"
$out = "model/ofin-model.gguf"
New-Item -ItemType Directory -Force -Path model
Invoke-WebRequest -Uri $url -OutFile $out
```

### Build

```powershell
cd engine
$env:CGO_ENABLED = "1"
go build -tags sqlite_fts5 -o bin/ofin.exe ./cmd/ofin
```

### Run

```powershell
bin/ofin.exe serve    # → http://127.0.0.1:8090
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
