# Òfin build targets. See CLAUDE.md / README.md for context.

.PHONY: build chunk sac ingest ask stop

build:            ## build the ofin CLI (FTS5 tag is required)
	cd engine && go build -tags sqlite_fts5 -o bin/ofin ./cmd/ofin

chunk:            ## corpus markdown -> data/chunks/
	python3 pipeline/chunk_statutes.py

sac:              ## data/chunks/ -> data/chunks-sac/ (needs GOOGLE_API_KEY, build-time only)
	python3 pipeline/enhance_chunks_with_sac.py

ingest:           ## data/chunks-sac/ -> data/ofin.db (venv has sqlite-vec; pyenv python lacks extension support)
	.venv/bin/python pipeline/ingest.py

ask:              ## e.g. make ask Q="How much notice after 3 years?"
	engine/bin/ofin ask "$(Q)"

stop:             ## stop background llama-server processes
	engine/bin/ofin stop
