"""Regression tests for the corpus cleaner's word-splitting heuristics.

Pins the Week-1 false positives: British spellings and irregular verb forms
missing from the macOS word list must NOT be split.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "scripts"))

import pytest
from clean_corpus import load_dictionary, split_token

DICT = load_dictionary()


@pytest.mark.parametrize("token,expected", [
    ("doesnot", "does not"),
    ("thatthe", "that the"),
    ("isemployed", "is employed"),
    ("shallascertain", "shall ascertain"),
    ("theofficer", "the officer"),
    ("oneday", "one day"),
    ("anyperson", "any person"),
])
def test_genuine_artifacts_split(token, expected):
    assert split_token(token, DICT) == expected


@pytest.mark.parametrize("token", [
    # British spellings + irregular/inflected forms that burned us in Week 1
    "offence", "defence", "labour", "became", "arising", "attempting",
    "undertaken", "another", "being", "island", "issue", "notary",
    "inability", "oral", "bears",
])
def test_real_words_left_alone(token):
    assert split_token(token, DICT) is None
