"""Tests for the author-correction classifier.

Every rule here exists because the obvious heuristic got a real book wrong. The
cases marked with a comment are the actual library data that motivated the rule.
"""

from author_fix_manifest import (
    classify,
    credits_author,
    edit_distance_le1,
    is_nested_collection,
    is_plausible_author,
    name_variant,
    near_duplicate_author,
    resolved_author_dir,
    _starts_with,
)

INTELLIQUEST = "IntelliQuest"


def row(**kw):
    base = {
        "path": "",
        "relative": f"{INTELLIQUEST}/Book",
        "author_folder": INTELLIQUEST,
        "book_folder": "Book",
        "file_count": 1,
        "files": ["/audiobooks/IntelliQuest/Book/Book.m4b"],
        "tag_title": "Book",
        "tag_album": "Book",
        "tag_artist": "",
        "tag_comment": "",
        "tag_narrator": "",
        "issues": [],
        "narrator_from_folder": "",
    }
    base.update(kw)
    return base


# --- is_plausible_author -------------------------------------------------


def test_placeholder_and_publisher_names_are_not_authors():
    for name in ("IntelliQuest", "Unknown Author", "Various", "Black Library",
                 "Foundation - The Complete Series", "Audiobooks", "Tantor Media",
                 "Plex Library", ""):
        assert not is_plausible_author(name), name


def test_people_and_coauthors_are_authors():
    for name in ("Leo Tolstoy", "Ursula K. Le Guin", "Jean-Jacques Rousseau",
                 "Samuel Taylor Coleridge", "Larry Niven and Jerry Pournelle",
                 "Iain M. Banks"):
        assert is_plausible_author(name), name


# --- name_variant --------------------------------------------------------


def test_name_variant_catches_spelling_drift():
    # "Iain Banks" (tag) vs "Iain M. Banks" (folder): dropped middle initial.
    assert name_variant("Iain Banks", "Iain M. Banks")
    # Punctuation only.
    assert name_variant("C.S. Lewis", "C. S. Lewis")
    # A surname change is a different person, not a variant.
    assert not name_variant("Leo Tolstoy", "Ghost Writer")
    assert not name_variant("Anton Chekhov", "Anton Chekhova")


# --- credits_author ------------------------------------------------------


def test_credit_line_counts_as_authorship_evidence():
    # The real comment on the Three-Body Problem files.
    c = ("Death's End - Cixin Liu - 2016\r\n"
         "The Three-Body Problem Series, Book 3\r\n\r\nBy: Cixin Liu, Ken Liu")
    assert credits_author(c, "Cixin Liu")
    assert credits_author(c, "Ken Liu")
    assert credits_author("Written by: Leo Tolstoy", "Leo Tolstoy")
    assert credits_author("Author: Ursula K. Le Guin", "Ursula K. Le Guin")


def test_narrator_line_is_not_an_authorship_credit():
    # The trap: "Narrated by X" contains the word "by". A reader must never be
    # read as the writer, or the file gets moved under the narrator's name.
    assert not credits_author("Narrated by Scott Brick", "Scott Brick")
    assert not credits_author("Read by Pringle Matthew", "Pringle Matthew")


def test_blurb_without_a_credit_is_not_evidence():
    # The real comment on The Road to Character, which is a jacket blurb.
    blurb = ("With the wisdom, humor, curiosity, and sharp insights that have "
             "brought millions of readers to his books")
    assert not credits_author(blurb, "Arthur Morey")
    assert not credits_author("", "Leo Tolstoy")


# --- nesting / author resolution -----------------------------------------


def test_resolved_author_dir_matches_the_shipped_go_helper():
    # Kept in step with library/audiobook_paths.go on purpose.
    assert resolved_author_dir(
        "/a/Isaac Asimov/Foundation - The Complete Series/"
        "Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}/f.m4b"
    ) == "Isaac Asimov"
    # The legacy compact wrapper is also nested three deep, but its parent is
    # the author -- depth alone is not the discriminator.
    assert resolved_author_dir(
        "/a/M4B Forge Compact/compact-m4b-80k/Iain M. Banks/The Player of Games/f.m4b"
    ) == "Iain M. Banks"
    assert resolved_author_dir("/a/Frank Herbert/Dune/Dune.m4b") == "Frank Herbert"
    assert resolved_author_dir("/Dune.m4b") is None


def test_starts_with_requires_a_word_boundary():
    assert _starts_with("Isaac Asimov - Foundation [01]", "Isaac Asimov")
    assert not _starts_with("Dune", "Frank Herbert")
    assert not _starts_with("Foundation", "Foundation - The Complete Series")


def test_nested_collection_excludes_the_compact_wrapper():
    # These 20 are the Foundation set: the author is one collection level up and
    # the code fix already resolves it, so no file may be moved.
    nested = row(
        relative="Isaac Asimov/Foundation - The Complete Series/"
                 "Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}",
        author_folder="Foundation - The Complete Series",
        book_folder="Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}",
        files=["/audiobooks/Isaac Asimov/Foundation - The Complete Series/"
               "Isaac Asimov - Foundation [01] Foundation [1951] {Jack Fox}/f.m4b"],
        tag_artist="Isaac Asimov",
    )
    assert is_nested_collection(nested)
    assert classify(nested)[0] == "code_fixed"

    wrapped = row(
        relative="M4B Forge Compact/compact-m4b-80k/Iain M. Banks/Transition",
        author_folder="Iain M. Banks",
        files=["/audiobooks/M4B Forge Compact/compact-m4b-80k/"
               "Iain M. Banks/Transition/f.m4b"],
    )
    assert not is_nested_collection(wrapped)


# --- classify ------------------------------------------------------------


def test_move_is_proposed_when_the_folder_author_is_not_a_person():
    r = row(tag_artist="Leo Tolstoy")
    action, _ = classify(r)
    assert action == "move_and_retag"


def test_plausible_folder_author_with_uncredited_tag_is_held_back():
    # Arthur Morey reads The Road to Character; the tag's artist field holds the
    # reader, not the writer. The folder is right.
    r = row(author_folder="David Brooks",
            book_folder="The Road to Character",
            files=["/audiobooks/David Brooks/The Road to Character/f.m4b"],
            tag_artist="Arthur Morey",
            tag_title="Part 1 of 10",
            tag_comment="With the wisdom, humor, curiosity, and sharp insights "
                        "that have brought millions of readers to his books")
    assert classify(r)[0] == "unverified_attribution"


def test_a_credited_tag_over_a_plausible_folder_is_a_move():
    r = row(author_folder="Ursula K. Le Guin",
            book_folder="The Dispossessed",
            files=["/audiobooks/Ursula K. Le Guin/The Dispossessed/f.m4b"],
            tag_artist="Iain M. Banks",
            tag_comment="By: Iain M. Banks")
    assert classify(r)[0] == "move_and_retag"


def test_name_variant_and_narrator_categories():
    # Punctuation-only drift folds to the same name, so it is reported as
    # agreement rather than as a move.
    assert classify(row(author_folder="C. S. Lewis", tag_artist="C.S. Lewis",
                        files=["/a/C. S. Lewis/B/f.m4b"]))[0] == "none"
    # A dropped initial is a real name variant, and the folder is still right.
    assert classify(row(author_folder="Iain M. Banks", tag_artist="Iain Banks",
                        files=["/a/Iain M. Banks/Transition/f.m4b"]))[0] == "same_person"
    # The real Death's End tags: the comment credits the author and then names
    # the reader. Ochlan must not win over Liu.
    assert classify(row(author_folder="Cixin Liu", tag_artist="P. J. Ochlan",
                        files=["/a/Cixin Liu/Deaths End/f.m4b"],
                        tag_comment="By: Cixin Liu\r\nNarrated by P. J. Ochlan")
                   )[0] == "narrator_in_artist"


def test_untagged_placeholder_needs_metadata():
    assert classify(row(tag_artist=""))[0] == "needs_metadata"
    # A person folder with no tag at all: nothing contradicts it.
    r = row(author_folder="Real Person", book_folder="Book",
            files=["/audiobooks/Real Person/Book/Book.m4b"], tag_artist="")
    assert classify(r)[0] == "no_evidence"


# --- typo detection ------------------------------------------------------


def test_edit_distance_le1():
    assert edit_distance_le1("leo tolstory", "leo tolstoy")
    assert edit_distance_le1("h g well", "h g wells")
    assert not edit_distance_le1("leo tolstoy", "leo tolstoy")
    assert not edit_distance_le1("leo tolstoy", "voltaire")


def test_typo_is_reported_with_its_origin():
    # Not in the library, only proposed here: the two spellings are both in the
    # tags and a human picks the right one.
    got = near_duplicate_author("Leo Tolstoy", set(), {"Leo Tolstory"})
    assert got == ("Leo Tolstory", "proposed by this manifest")
    got2 = near_duplicate_author("H. G. Well", {"H. G. Wells"}, set())
    assert got2 == ("H. G. Wells", "in the library")
    assert near_duplicate_author("Isaac Asimov", {"Leo Tolstoy"},
                                 {"Leo Tolstory"}) is None
