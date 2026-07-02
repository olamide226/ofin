# Òfin build targets. See CLAUDE.md / README.md for context.

.PHONY: build chunk sac ingest ask stop

build:            ## build the ofin CLI (FTS5 tag is required)
	cd engine && go build -tags sqlite_fts5 -o bin/ofin ./cmd/ofin

PY := .venv/bin/python

setup:            ## one-time: create the pipeline venv (needs Homebrew python3)
	/opt/homebrew/bin/python3 -m venv .venv
	.venv/bin/pip install -r pipeline/requirements.txt

chunk:            ## corpus markdown -> data/chunks/
	$(PY) pipeline/chunk_statutes.py

sac:              ## data/chunks/ -> data/chunks-sac/ (needs GOOGLE_API_KEY, build-time only)
	$(PY) pipeline/enhance_chunks_with_sac.py

ingest:           ## data/chunks-sac/ -> data/ofin.db
	$(PY) pipeline/ingest.py

test:             ## run pipeline unit tests + corpus QA gate + Go tests
	$(PY) -m pytest pipeline/tests -q
	$(PY) pipeline/qa_corpus.py --strict
	cd engine && go test -tags sqlite_fts5 ./...

ask:              ## e.g. make ask Q="How much notice after 3 years?"
	engine/bin/ofin ask "$(Q)"

stop:             ## stop background llama-server processes
	engine/bin/ofin stop
