#!/usr/bin/env python3
"""Clean statutory-text markdown for the Òfin corpus.

Fixes the systematic PDF-extraction artifact where two words are run
together (e.g. "doesnot", "theofficer", "shallascertain"). Conservative by
construction: a token is only split when

  1. the token itself is NOT a dictionary word, and
  2. it splits into <function word> + <dictionary word>.

Function words lead the split because the artifact overwhelmingly glues a
short grammatical word to the following content word. Anything ambiguous is
left untouched — for a legal corpus, a missed fix beats a corruption.

Usage: clean_corpus.py IN.md OUT.md   (prints each fix to stderr for review)
"""

import re
import sys

FUNCTION_WORDS = [
    "that", "the", "this", "these", "those", "is", "are", "was", "were",
    "be", "been", "does", "do", "did", "not", "any", "in", "of", "to",
    "shall", "may", "and", "or", "his", "her", "its", "their", "an", "a",
    "if", "as", "at", "by", "for", "he", "she", "it", "with", "under",
    "such", "no", "on", "so", "who", "which", "there", "where", "when",
    "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
    "ten", "twelve",
]
# Longest first so "that" wins over "the"+"at" style overlaps.
FUNCTION_WORDS.sort(key=len, reverse=True)


def load_dictionary() -> set:
    words = set()
    with open("/usr/share/dict/words", encoding="utf-8") as fh:
        for line in fh:
            words.add(line.strip().lower())
    # Statutory vocabulary and British spellings the system dictionary lacks.
    words.update({
        "subsection", "subsections", "paragraph", "paragraphs", "gazette",
        "minister", "employer", "employers", "employee", "employees",
        "worker", "workers", "wages", "recruiter", "recruiting",
        "arbitration", "conciliation", "tribunal", "nigeria", "nigerian",
        "naira", "apprentice", "apprenticeship", "particulars",
        "offence", "offences", "defence", "defences", "labour", "favour",
        "favours", "honour", "honours", "neighbour", "neighbours",
        "organisation", "organisations", "authorised", "authorise",
        "recognised", "recognise", "practise", "licence", "licences",
        "judgement", "judgements", "endeavour", "endeavours",
        # Irregular forms the system word list omits.
        "became", "become", "becomes", "arose", "arisen", "began", "begun",
        "chose", "chosen", "forbade", "forbidden", "paid", "unpaid",
        "said", "made", "held", "kept", "left", "met", "sent", "spent",
    })
    return words


def is_known_word(token: str, dictionary: set) -> bool:
    """Dictionary check with light morphology: the system word list omits
    many inflected forms (e.g. "attempting", "undertaken"), so strip common
    suffixes and retry before declaring a token unknown."""
    if token in dictionary:
        return True
    for suffix, replacements in (
        ("ing", ("", "e")),   # attempting -> attempt, arising -> arise
        ("ed", ("", "e")),    # employed -> employ, required -> require
        ("en", ("", "e")),    # undertaken -> undertake
        ("s", ("",)),         # workers -> worker
        ("es", ("", "e")),    # wages -> wage
        ("ies", ("y",)),      # penalties -> penalty
    ):
        if token.endswith(suffix):
            stem = token[: -len(suffix)]
            for extra in replacements:
                if len(stem) >= 3 and stem + extra in dictionary:
                    return True
            # doubled final consonant: permitting -> permit
            if len(stem) >= 4 and stem[-1] == stem[-2] and stem[:-1] in dictionary:
                return True
    return False


def split_token(token: str, dictionary: set) -> str | None:
    low = token.lower()
    if len(low) < 5 or is_known_word(low, dictionary):
        return None
    for fw in FUNCTION_WORDS:
        rest = low[len(fw):]
        if low.startswith(fw) and len(rest) >= 3 and is_known_word(rest, dictionary):
            return f"{token[:len(fw)]} {token[len(fw):]}"
    return None


def main() -> None:
    src, dst = sys.argv[1], sys.argv[2]
    dictionary = load_dictionary()
    text = open(src, encoding="utf-8").read()
    fixes: list[tuple[str, str]] = []

    def repl(match: re.Match) -> str:
        token = match.group(0)
        fixed = split_token(token, dictionary)
        if fixed is not None:
            fixes.append((token, fixed))
            return fixed
        return token

    cleaned = re.sub(r"\b[a-z]{5,}\b", repl, text)
    with open(dst, "w", encoding="utf-8") as fh:
        fh.write(cleaned)

    for old, new in fixes:
        print(f"  {old!r} -> {new!r}", file=sys.stderr)
    print(f"{len(fixes)} fixes: {src} -> {dst}", file=sys.stderr)


if __name__ == "__main__":
    main()
