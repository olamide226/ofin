#!/usr/bin/env python3
"""Bake the Òfin persona into the GGUF chat template.

Replaces the tools/dates boilerplate between the system header and
{{- system_message }} in the Jinja2 chat template with a same-byte-length
Òfin default persona. After baking, the model defaults to Òfin when no
system prompt is provided (standalone judge testing), while our app's
explicit system prompt still takes effect when the full stack runs.

Uses byte-accurate binary patching of the GGUF file — no offset shifts,
no tensor rewriting.
"""

import os
import struct
import shutil
import sys
from gguf import GGUFReader

MODEL_PATH = os.path.join(os.path.dirname(__file__), "..", "model", "ofin-model.gguf")
BACKUP_PATH = os.path.join(os.path.dirname(__file__), "..", "model", "ofin-model-vanilla-backup.gguf")

# ——— Persona ———————————————————————————————————————————————————————————
# The replacement Jinja2 literal is: \n{{- "PERSONA" }}\n
# Prefix + suffix = 11 bytes; the persona text fills the rest.
PREFIX = '\n{{- "'
SUFFIX = '" }}\n'

# This persona fills exactly the space the old tools+dates boilerplate occupied.
# It explicitly lists what Òfin covers so the model doesn't get confused about
# its own scope (v1 was too vague — the model refused valid labour questions).
_PERSONA_TEXT = (
    "You are Òfin, an offline legal companion for Nigerian law. "
    "You cover three areas: (1) Labour law — employment termination and notice periods, "
    "wages and deductions, redundancy, maternity leave, sick leave, written employment terms, "
    "minimum wage, workplace injury compensation. (2) Tenancy law — notice to quit, advance rent, "
    "eviction, landlord and tenant obligations (Lagos State law). (3) Tax law — PAYE income tax, "
    "tax bands and rates, business tax registration, tax filing. "
    "Cite specific sections from Nigerian statutes in [Act Name, s.X(Y)] format. "
    "Answer in the same language as the question — use Nigerian Pidgin if the question is in Pidgin. "
    "If asked about anything outside labour, tenancy, or tax law, politely refuse and explain your scope. "
    "You give statutory information with citations, not legal advice."
)


def _walk_gguf(data: bytes):
    """Find tokenizer.chat_template offset and byte-length in a GGUF binary."""
    kv_count = struct.unpack_from("<Q", data, 16)[0]
    pos = 24  # after magic(4) + version(4) + tensor_count(8) + kv_count(8)
    scalar_sizes = {0: 1, 1: 1, 2: 2, 3: 2, 4: 4, 5: 4, 6: 4, 7: 1, 10: 8, 11: 8, 12: 8}

    for _ in range(kv_count):
        key_len = struct.unpack_from("<Q", data, pos)[0]; pos += 8
        key = data[pos:pos + key_len].decode("utf-8", errors="replace"); pos += key_len
        val_type = struct.unpack_from("<I", data, pos)[0]; pos += 4

        if val_type == 8:  # STRING
            vlen = struct.unpack_from("<Q", data, pos)[0]; pos += 8
            if key == "tokenizer.chat_template":
                return pos, vlen
            pos += vlen
        elif val_type == 9:  # ARRAY
            etype = struct.unpack_from("<I", data, pos)[0]
            alen = struct.unpack_from("<Q", data, pos + 4)[0]; pos += 12
            if etype == 8:
                for _ in range(alen):
                    sl = struct.unpack_from("<Q", data, pos)[0]; pos += 8 + sl
            elif etype in scalar_sizes:
                pos += alen * scalar_sizes[etype]
            else:
                pos += alen * 4
        elif val_type in scalar_sizes:
            pos += scalar_sizes[val_type]

    return None, None


def main():
    # 1. Find the template in the binary file
    with open(MODEL_PATH, "rb") as f:
        data = f.read()

    template_offset, template_byte_len = _walk_gguf(data)
    if template_offset is None:
        print("ERROR: tokenizer.chat_template not found in GGUF metadata")
        return 1

    # 2. Read the current template
    old_template = data[template_offset:template_offset + template_byte_len].decode("utf-8", errors="replace")
    print(f"Template: {len(old_template)} chars, {template_byte_len} bytes")

    # 3. Locate the section to replace
    sys_hdr = '{{- "<|start_header_id|>system<|end_header_id|>\\n\\n" }}'
    sm_marker = "{{- system_message }}"
    h = old_template.find(sys_hdr)
    s = old_template.find(sm_marker, h)
    if h < 0 or s < 0:
        print(f"ERROR: Cannot find markers (h={h}, s={s})")
        return 1

    old_section = old_template[h + len(sys_hdr):s]
    old_section_bytes = len(old_section.encode("utf-8"))
    print(f"Section to replace: {len(old_section)} chars, {old_section_bytes} bytes")

    # 4. Build replacement of EXACT same byte length
    pfx_b = len(PREFIX.encode("utf-8"))
    sfx_b = len(SUFFIX.encode("utf-8"))
    avail = old_section_bytes - pfx_b - sfx_b

    persona = _PERSONA_TEXT
    pb = len(persona.encode("utf-8"))
    if pb < avail:
        persona = persona + " " * (avail - pb)
    elif pb > avail:
        while len(persona.encode("utf-8")) > avail:
            persona = persona[:-1]

    replacement = PREFIX + persona + SUFFIX
    rep_b = len(replacement.encode("utf-8"))
    assert rep_b == old_section_bytes, f"Replacement bytes {rep_b} != section bytes {old_section_bytes}"

    # 5. Build new template and verify byte-identical length
    new_template = old_template[:h + len(sys_hdr)] + replacement + old_template[s:]
    new_b = new_template.encode("utf-8")
    assert len(new_b) == template_byte_len, f"New template {len(new_b)}B != old {template_byte_len}B"

    # 6. Backup
    if not os.path.exists(BACKUP_PATH):
        shutil.copy2(MODEL_PATH, BACKUP_PATH)
        print(f"Backup: {BACKUP_PATH}")
    else:
        print(f"Backup exists: {BACKUP_PATH}")

    # 7. Binary patch
    patched = data[:template_offset] + new_b + data[template_offset + template_byte_len:]
    assert len(patched) == len(data)

    with open(MODEL_PATH, "wb") as f:
        f.write(patched)
    print("✓ Binary patch applied")

    # 8. Verify
    reader = GGUFReader(MODEL_PATH)
    field = reader.fields["tokenizer.chat_template"]
    raw = field.parts[field.data[0]]
    baked = bytes(raw.tolist()).decode("utf-8", errors="replace")

    if "Òfin" not in baked:
        print("✗ Òfin NOT found — restoring backup")
        shutil.copy2(BACKUP_PATH, MODEL_PATH)
        return 1

    i = baked.find("Òfin")
    print(f"✓ Òfin at template pos {i}")
    print(f"  {baked[i:i+160]}...")

    vanilla_sz = os.path.getsize(BACKUP_PATH)
    baked_sz = os.path.getsize(MODEL_PATH)
    print(f"\n✓ Done. Vanilla: {vanilla_sz:,}B  Baked: {baked_sz:,}B  Same: {vanilla_sz == baked_sz}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
