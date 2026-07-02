#!/usr/bin/env python3
"""Ingest SAC-enriched chunks into the Òfin retrieval database.

Builds data/ofin.db (single file, ships with the app):
  - chunks      — full metadata + text + summary + cross_refs (the verifier's
                  citation lookup happens here: act_short + section_id)
  - chunks_fts  — FTS5 over title/summary/text (keyword leg)
  - vec_chunks  — sqlite-vec vec0 table, 384-dim embeddings (vector leg)

Embeddings come from bge-small-en-v1.5 F16 through a temporary local
llama-server --embedding process (ADR-008). Documents are embedded WITHOUT
the BGE query prefix; queries at runtime use it.

Usage:
    python3 pipeline/ingest.py [--model models-dev/bge-small-en-v1.5-f16.gguf]
"""

import argparse
import json
import socket
import sqlite3
import struct
import subprocess
import time
import urllib.request
from pathlib import Path

import sqlite_vec

REPO = Path(__file__).parent.parent
EMBED_DIM = 384
# ~450 tokens: keeps header+summary+leading text inside BGE's 512-token window.
EMBED_MAX_CHARS = 1800
EMBED_BATCH = 32


def wait_for_server(port: int, timeout_s: int = 60) -> None:
    deadline = time.time() + timeout_s
    while time.time() < deadline:
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/health", timeout=2) as r:
                if r.status == 200:
                    return
        except Exception:
            time.sleep(0.5)
    raise RuntimeError("llama-server (embedding) did not become healthy in time")


def embed_batch(port: int, texts: list[str]) -> list[list[float]]:
    payload = json.dumps({"input": texts}).encode()
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}/v1/embeddings",
        data=payload, headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=120) as r:
        data = json.loads(r.read())
    by_index = {item["index"]: item["embedding"] for item in data["data"]}
    return [by_index[i] for i in range(len(texts))]


def serialize_f32(vec: list[float]) -> bytes:
    return struct.pack(f"{len(vec)}f", *vec)


def build_db(db_path: Path, chunk_files: list[Path], embed_port: int) -> None:
    if db_path.exists():
        db_path.unlink()
    db = sqlite3.connect(db_path)
    db.enable_load_extension(True)
    sqlite_vec.load(db)
    db.enable_load_extension(False)

    db.executescript("""
        CREATE TABLE chunks (
            id INTEGER PRIMARY KEY,
            act_short TEXT NOT NULL,
            section_id TEXT NOT NULL,
            section_number INTEGER,
            section_title TEXT,
            part TEXT,
            chunk_type TEXT NOT NULL,
            jurisdiction TEXT NOT NULL,
            source TEXT NOT NULL,
            citation TEXT NOT NULL,
            as_at TEXT NOT NULL,
            file TEXT NOT NULL,
            text TEXT NOT NULL,
            summary TEXT NOT NULL,
            cross_refs TEXT NOT NULL DEFAULT '[]'
        );
        CREATE INDEX idx_chunks_citation ON chunks(act_short, section_id);
        CREATE VIRTUAL TABLE chunks_fts USING fts5(
            section_title, summary, body
        );
    """)
    db.execute(f"CREATE VIRTUAL TABLE vec_chunks USING vec0(embedding float[{EMBED_DIM}])")

    rows: list[dict] = []
    for path in chunk_files:
        data = json.loads(path.read_text(encoding="utf-8"))
        for chunk in data["chunks"]:
            m = chunk["metadata"]
            rows.append({
                "act_short": m["act_short"], "section_id": m["section_id"],
                "section_number": m.get("section_number"),
                "section_title": m.get("section_title"), "part": m.get("part"),
                "chunk_type": m["chunk_type"], "jurisdiction": m["jurisdiction"],
                "source": m["source"], "citation": m["citation"],
                "as_at": m["as_at"], "file": m["file"],
                "text": chunk["text"], "summary": chunk.get("summary", ""),
                "cross_refs": json.dumps(chunk.get("cross_refs", [])),
                "augmented_text": chunk.get("augmented_text", chunk["text"]),
            })

    print(f"Embedding {len(rows)} chunks (batch={EMBED_BATCH})…")
    embeddings: list[list[float]] = []
    for i in range(0, len(rows), EMBED_BATCH):
        batch = [r["augmented_text"][:EMBED_MAX_CHARS] for r in rows[i:i + EMBED_BATCH]]
        embeddings.extend(embed_batch(embed_port, batch))
        print(f"  {min(i + EMBED_BATCH, len(rows))}/{len(rows)}", end="\r")
    print()

    for idx, (row, emb) in enumerate(zip(rows, embeddings), start=1):
        db.execute(
            """INSERT INTO chunks (id, act_short, section_id, section_number, section_title,
               part, chunk_type, jurisdiction, source, citation, as_at, file, text, summary, cross_refs)
               VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)""",
            (idx, row["act_short"], row["section_id"], row["section_number"],
             row["section_title"], row["part"], row["chunk_type"], row["jurisdiction"],
             row["source"], row["citation"], row["as_at"], row["file"],
             row["text"], row["summary"], row["cross_refs"]),
        )
        db.execute(
            "INSERT INTO chunks_fts (rowid, section_title, summary, body) VALUES (?,?,?,?)",
            (idx, row["section_title"] or "", row["summary"], row["text"]),
        )
        db.execute(
            "INSERT INTO vec_chunks (rowid, embedding) VALUES (?,?)",
            (idx, serialize_f32(emb)),
        )

    db.commit()
    n = db.execute("SELECT count(*) FROM chunks").fetchone()[0]
    acts = db.execute("SELECT act_short, count(*) FROM chunks GROUP BY act_short").fetchall()
    db.close()
    print(f"✅ {db_path} built: {n} chunks")
    for act, count in acts:
        print(f"   {act}: {count}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Build ofin.db from SAC-enriched chunks")
    parser.add_argument("--chunks-dir", type=Path, default=REPO / "data/chunks-sac")
    parser.add_argument("--db", type=Path, default=REPO / "data/ofin.db")
    parser.add_argument("--model", type=Path, default=REPO / "models-dev/bge-small-en-v1.5-f16.gguf")
    parser.add_argument("--port", type=int, default=0, help="0 = pick a free port")
    args = parser.parse_args()

    chunk_files = sorted(args.chunks_dir.glob("*.json"))
    if not chunk_files:
        raise SystemExit(f"No chunk files in {args.chunks_dir} — run enhance_chunks_with_sac.py first")

    port = args.port
    if port == 0:
        with socket.socket() as s:
            s.bind(("127.0.0.1", 0))
            port = s.getsockname()[1]

    server = subprocess.Popen(
        ["llama-server", "-m", str(args.model), "--embedding",
         "--port", str(port), "--host", "127.0.0.1", "-c", "512", "-ub", "512"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    try:
        wait_for_server(port)
        build_db(args.db, chunk_files, port)
    finally:
        server.terminate()
        server.wait(timeout=10)


if __name__ == "__main__":
    main()
