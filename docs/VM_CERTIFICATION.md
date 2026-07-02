# VM Certification Runbook

How to reproduce the audit environment and run the profiler against Òfin on
a fresh VM. The official audit runs in cloud VMs on the standard laptop
profile (**4 vCPU, 8 GB RAM, integrated/no GPU**) — provision exactly that
(ADR-007).

## 0. Provision (Ola)

Any x86-64 4 vCPU / 8 GB Ubuntu 24.04 VM. Cheap options: Hetzner CPX31,
DigitalOcean 8 GB/4 vCPU droplet, AWS c5.xlarge (hourly). Do NOT oversize —
the RAM cap is the test.

## 1. Base tooling

```bash
sudo apt-get update && sudo apt-get install -y git curl unzip python3-venv python3-pip build-essential cmake
```

## 2. llama.cpp (the profiler needs `llama-bench` on PATH)

Prebuilt (fastest):

```bash
# pick the latest ubuntu-x64 asset from https://github.com/ggml-org/llama.cpp/releases
curl -LO https://github.com/ggml-org/llama.cpp/releases/download/b9859/llama-b9859-bin-ubuntu-x64.tar.gz

# extract and add to environment
tar -xzf llama-b9859-bin-ubuntu-x64.tar.gz
sudo mv llama-b9859 /opt/llama.cpp
echo 'export PATH="/opt/llama.cpp:$PATH"' >> ~/.bashrc
echo 'export LD_LIBRARY_PATH="/opt/llama.cpp:$LD_LIBRARY_PATH"' >> ~/.bashrc
source ~/.bashrc
llama-bench --help >/dev/null && echo OK
```

From source (fallback, ~5 min):

```bash
git clone --depth 1 https://github.com/ggml-org/llama.cpp && cd llama.cpp
cmake -B build -DCMAKE_BUILD_TYPE=Release && cmake --build build -j4
sudo cp build/bin/llama-{bench,cli,server,embedding} /usr/local/bin/ && cd ..
```

## 3. Clone the repo (private — authenticate first)

```bash
# install gh if not already present
sudo apt-get install -y gh

# authenticate (device code flow — works on headless VMs)
gh auth login --hostname github.com --git-protocol https --web
# opens https://github.com/login/device — enter the code shown

# clone
gh repo clone olamide226/ofin && cd ofin

python3 -m venv .venv && source .venv/bin/activate
```

## 4. Tier A — profiler certification (the numbers that matter)

```bash
bash download_model.sh                       # ~1.9 GB Llama GGUF -> model/
pip install "git+https://github.com/Africa-Deep-Tech-Foundation/adtc-profiler.git"
adtc-profiler run --submission . --mode participant --output submission.json --skip-accuracy
python3 -m json.tool submission.json | sed -n '/throughput/,/cpu_thermal/p'
```

Record into `docs/benchmarks/` as `<date>-vm-<provider>-4vcpu8gb.json` and
compare against the dev baseline with:

```bash
adtc-profiler compare docs/benchmarks/<dev-baseline>.json submission.json --output verdict.json
```

**What to look for:** `tokens_per_second_generation` vs the 15-TPS S_perf
reference (at/above 15 = full marks, stop optimising); `peak_rss_mb` (S_eff;
expect ~2.1 GB); `throttled` must be false over the run.

### Sustained-load variant (Week-3/5 thermal checkbox)

Run the profiler three times back-to-back and watch for TPS decay across
runs — VMs don't expose core temps, so sustained-TPS stability is our
throttle proxy:

```bash
for i in 1 2 3; do adtc-profiler run --submission . --mode participant \
  --output run$i.json --skip-accuracy; done
grep tokens_per_second run*.json
```

## 5. Tier B — full application test (optional but recommended)

```bash
# embedding model (gitignored)
mkdir -p models-dev && curl -L -o models-dev/bge-small-en-v1.5-f16.gguf \
  "https://huggingface.co/CompendiumLabs/bge-small-en-v1.5-gguf/resolve/main/bge-small-en-v1.5-f16.gguf"
cp model/ofin-model.gguf models-dev/Llama-3.2-3B-Instruct-Q4_K_M.gguf

# pipeline env (Ubuntu python3 supports sqlite extensions out of the box)
python3 -m venv .venv && .venv/bin/pip install -r pipeline/requirements.txt
make ingest        # rebuilds data/ofin.db from committed chunks-sac (no API key needed)

# Go engine
sudo snap install go --classic   # or apt golang-go if >=1.22
make build
engine/bin/ofin ask "I have worked 3 years, how much notice must my employer give me?"
```

Watch peak RSS of the whole stack while asking:
`smem -k -c "name rss" | grep -E "llama|ofin"` (target: < 3.5 GB total).

## Notes

- `download_model.sh` needs no credentials (public HF repo) — this is also
  the audit's first step, so a failure here is a submission failure.
- Zero network calls after model download: verify with
  `sudo ss -tupn | grep -v 127.0.0.1` while asking questions.
- VM results are the certification numbers for REPORT.md; Mac numbers stay
  dev-baseline (ADR-002/007).
