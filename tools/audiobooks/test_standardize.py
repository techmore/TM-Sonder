import pathlib
import tempfile
import unittest

from standardize import apply_plan, build_plan, clean_name, looks_like_parts


def touch(path: pathlib.Path, data: bytes = b"payload") -> pathlib.Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return path


class CleanNameTest(unittest.TestCase):
    def test_provider_and_bitrate_tags_are_dropped(self):
        self.assertEqual(clean_name("Project Hail Mary {audible-B08G}"), "Project Hail Mary")
        self.assertEqual(clean_name("Dune [64kbps]"), "Dune")
        self.assertEqual(clean_name("Some Book 128k"), "Some Book")
        self.assertEqual(clean_name("A Book [417mb]"), "A Book")

    def test_name_without_noise_is_untouched(self):
        self.assertEqual(clean_name("The Lathe of Heaven"), "The Lathe of Heaven")

    def test_empty_result_falls_back_to_original(self):
        self.assertEqual(clean_name("{tag}"), "{tag}")

    # An edition marker distinguishes two genuinely different recordings, so
    # dropping it would collapse them into one book.
    def test_edition_markers_are_preserved(self):
        self.assertEqual(clean_name("Jurassic Park [Unabridged]"), "Jurassic Park [Unabridged]")
        self.assertEqual(clean_name("Dune[Unabridged]"), "Dune [Unabridged]")
        self.assertEqual(clean_name("Book [Full Cast]"), "Book [Full Cast]")

    def test_two_editions_do_not_collapse_to_one_name(self):
        self.assertNotEqual(clean_name("Jurassic Park"),
                            clean_name("Jurassic Park [Unabridged]"))


class PartDetectionTest(unittest.TestCase):
    def test_part_names_are_recognized(self):
        for name in ("Dune-Part01", "cd01-02 track", "03 - Dune", "PWE01-10 Peter Watts",
                     "Ringworld Engineers 07", "Dune, Vol 2"):
            self.assertTrue(looks_like_parts(name), name)

    def test_plain_book_names_are_not_parts(self):
        for name in ("Dune", "Project Hail Mary", "The Left Hand of Darkness"):
            self.assertFalse(looks_like_parts(name), name)


class PlanTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def folder(self, plan, rel):
        return next(f for f in plan["folders"] if f["relative_folder"] == rel)

    def test_canonical_layout_is_kept(self):
        touch(self.root / "Andy Weir/Project Hail Mary/Project Hail Mary.m4b")
        f = self.folder(build_plan(self.root), "Andy Weir/Project Hail Mary")
        self.assertEqual((f["action"], f["group"]), ("keep", "single_file"))

    def test_noisy_single_file_in_clean_folder_is_renamed(self):
        touch(self.root / "Andy Weir/Dune/Dune [64kbps].m4b")
        f = self.folder(build_plan(self.root), "Andy Weir/Dune")
        self.assertEqual(f["action"], "rename")
        self.assertTrue(f["target_file"].endswith("Andy Weir/Dune/Dune.m4b"))

    def test_noisy_folder_name_moves_the_whole_book(self):
        touch(self.root / "Miguel de Cervantes/Don Quixote {audible-XYZ} (2020)/"
                         "Don Quixote {audible-XYZ} (2020).m4b")
        f = self.folder(build_plan(self.root),
                        "Miguel de Cervantes/Don Quixote {audible-XYZ} (2020)")
        self.assertEqual(f["action"], "rename_book")
        self.assertEqual(f["book"], "Don Quixote (2020)")
        self.assertTrue(f["target_dir"].endswith("Miguel de Cervantes/Don Quixote (2020)"))

    # An edition marker in the folder name is meaningful, not noise, so the
    # folder must not be renamed just because the layout prefers Book/Book.m4b.
    def test_edition_marker_folder_is_left_alone(self):
        touch(self.root / "C. S. Lewis/Narnia [Unabridged]/Narnia [Unabridged].m4b")
        f = self.folder(build_plan(self.root), "C. S. Lewis/Narnia [Unabridged]")
        self.assertEqual(f["action"], "keep")

    def test_multipart_book_is_kept_not_renamed(self):
        for i in (1, 2, 3):
            touch(self.root / f"Peter Watts/Echopraxia/Echopraxia Part {i:02d}.mp3")
        f = self.folder(build_plan(self.root), "Peter Watts/Echopraxia")
        self.assertEqual(f["action"], "keep_multi_part")
        self.assertEqual(f["group"], "multi_part_book")

    def test_numbered_mp3_book_is_recognized_as_multipart(self):
        for i in range(1, 6):
            touch(self.root / f"Larry Niven/Ringworld Engineers/Ringworld Engineers {i:02d}.mp3")
        f = self.folder(build_plan(self.root), "Larry Niven/Ringworld Engineers")
        self.assertEqual(f["group"], "multi_part_book")

    def test_competing_editions_are_flagged_for_review(self):
        touch(self.root / "Author/Book/Book.m4b", b"a" * 100)
        touch(self.root / "Author/Book/Book copy.m4b", b"b" * 200)
        plan = build_plan(self.root)
        reviews = [f for f in plan["folders"] if f["action"] == "review"]
        self.assertEqual(len(reviews), 1)
        self.assertEqual(reviews[0]["group"], "multiple_editions")

    # Two editions of one book are two books, not a collision. Stripping the
    # edition marker would wrongly merge them.
    def test_unabridged_edition_is_not_a_collision(self):
        touch(self.root / "Michael Crichton/Jurassic Park/Jurassic Park.m4b")
        touch(self.root / "Michael Crichton/Jurassic Park [Unabridged]/"
                         "Jurassic Park [Unabridged].m4b")
        plan = build_plan(self.root)
        self.assertEqual(plan["summary"]["collision_targets"], 0)
        for f in plan["folders"]:
            self.assertEqual(f["action"], "keep", f["relative_folder"])
        self.assertIn("Unabridged", self.folder(
            build_plan(self.root), "Michael Crichton/Jurassic Park [Unabridged]")["book"])

    def test_wrapper_twin_of_an_existing_book_is_flagged(self):
        touch(self.root / "Iain M. Banks/The Player of Games/The Player of Games.m4b", b"a" * 50)
        touch(self.root / "M4B Forge Compact/compact-m4b-80k/Iain M. Banks/The Player of Games/The Player of Games.m4b", b"a" * 50)
        plan = build_plan(self.root)
        reviews = [f for f in plan["folders"] if f["action"] == "review"]
        self.assertEqual(len(reviews), 2)
        self.assertTrue(all("target_collision" in f["reasons"] for f in reviews))
        self.assertEqual(plan["summary"]["collision_targets"], 1)

    def test_identical_size_copies_within_one_folder_are_flagged(self):
        touch(self.root / "Author/Book/Book.m4b", b"a" * 50)
        touch(self.root / "Author/Book/Book copy.m4b", b"a" * 50)
        f = self.folder(build_plan(self.root), "Author/Book")
        self.assertEqual(f["group"], "identical_size_duplicates")
        self.assertEqual(f["action"], "review")

    def test_leftover_temp_artifact_is_identified(self):
        touch(self.root / "Author/Book/Book.m4b", b"a" * 10)
        touch(self.root / "Author/Book/Book.m4b.sonder-retag.m4b", b"")
        f = self.folder(build_plan(self.root), "Author/Book")
        self.assertEqual(f["action"], "review")
        self.assertEqual(f["group"], "single_book_plus_leftovers")
        self.assertIn("zero_byte_file", f["reasons"])
        self.assertTrue(f["files"][1]["temp_artifact"])

    def test_series_collection_is_reviewed(self):
        for i, n in enumerate(("Foundation", "Foundation and Empire", "Second Foundation")):
            touch(self.root / f"Isaac Asimov/Foundation - The Complete Series/"
                             f"Isaac Asimov - Foundation [0{i}] {n}/Book 0{i}.m4b", b"a" * (10 * (i + 1)))
        plan = build_plan(self.root)
        for f in plan["folders"]:
            self.assertEqual(f["group"], "series_collection")
            self.assertEqual(f["action"], "review")
            self.assertIn("series_collection_folder", f["reasons"])

    def test_multiple_editions_in_one_folder_are_reviewed(self):
        touch(self.root / "Author/Book/Book.m4b", b"a" * 10)
        touch(self.root / "Author/Book/Book - Unabridged.m4b", b"b" * 20)
        f = self.folder(build_plan(self.root), "Author/Book")
        self.assertEqual(f["group"], "multiple_editions")
        self.assertEqual(f["action"], "review")

    def test_unknown_author_is_reviewed(self):
        touch(self.root / "Unknown Author/VALIS/VALIS.m4b")
        f = self.folder(build_plan(self.root), "Unknown Author/VALIS")
        self.assertEqual(f["action"], "review")
        self.assertIn("unknown_author_folder", f["reasons"])

    def test_file_directly_under_root_is_reviewed(self):
        touch(self.root / "Loose Book.m4b")
        f = self.folder(build_plan(self.root), ".")
        self.assertIn("file_directly_under_library_root", f["reasons"])

    def test_file_directly_under_author_folder_is_reviewed(self):
        touch(self.root / "Atul Gawande/Better.m4b")
        f = self.folder(build_plan(self.root), "Atul Gawande")
        self.assertIn("file_directly_under_author_folder", f["reasons"])

    def test_zero_byte_file_is_reviewed(self):
        touch(self.root / "Author/Book/Book.m4b", b"")
        f = self.folder(build_plan(self.root), "Author/Book")
        self.assertEqual(f["action"], "review")
        self.assertIn("zero_byte_file", f["reasons"])

    def test_appledouble_sidecars_are_ignored(self):
        touch(self.root / "Andy Weir/Dune/Dune.m4b")
        touch(self.root / "Andy Weir/Dune/._Dune.m4b")
        self.assertEqual(len(build_plan(self.root)["folders"]), 1)

    def test_wrapper_copy_itself_is_marked_wrapped(self):
        touch(self.root / "M4B Forge Compact/compact-m4b-80k/Mark Skousen/Book/Book.m4b")
        f = self.folder(build_plan(self.root), "M4B Forge Compact/compact-m4b-80k/Mark Skousen/Book")
        self.assertTrue(f["wrapped_in_legacy_wrapper"])
        self.assertEqual(f["author"], "Mark Skousen")

    def test_stray_file_inside_the_wrapper_is_reviewed(self):
        touch(self.root / "compact-m4b-80k/Test-ebook/Test-ebook.m4b")
        f = self.folder(build_plan(self.root), "compact-m4b-80k/Test-ebook")
        self.assertIn("partial_legacy_wrapper_path", f["reasons"])
        self.assertEqual(f["action"], "review")

    def test_planning_never_touches_disk(self):
        touch(self.root / "Andy Weir/Dune/Dune [64kbps].m4b")
        before = sorted(p.relative_to(self.root).as_posix() for p in self.root.rglob("*"))
        build_plan(self.root)
        after = sorted(p.relative_to(self.root).as_posix() for p in self.root.rglob("*"))
        self.assertEqual(before, after)

    def test_summary_counts_cover_every_folder(self):
        touch(self.root / "A/Keep/Keep.m4b")
        touch(self.root / "A/Rename/Old Name.m4b")
        touch(self.root / "A/Review/Review.m4b", b"")
        plan = build_plan(self.root)
        self.assertEqual(plan["summary"]["folders"], 3)
        self.assertEqual(sum(plan["summary"]["actions"].values()), 3)
        self.assertEqual(plan["summary"]["files"], 3)


class ApplyTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def test_apply_renames_and_preserves_bytes(self):
        src = touch(self.root / "David Brooks/Book/Book - 01 - Part 1 of 10.m4b", b"audio")
        plan = build_plan(self.root)
        self.assertEqual(plan["folders"][0]["action"], "rename")
        receipt = apply_plan(plan, None)
        dst = self.root / "David Brooks/Book/Book.m4b"
        self.assertEqual(len(receipt["applied"]), 1)
        self.assertTrue(dst.is_file() and not src.exists())
        self.assertEqual(dst.read_bytes(), b"audio")

    def test_apply_moves_book_folder_and_matches_file_name(self):
        touch(self.root / "Author/Book {audible-XYZ} (2020)/Book {audible-XYZ} (2020).m4b",
              b"audio")
        plan = build_plan(self.root)
        self.assertEqual(plan["folders"][0]["action"], "rename_book")
        receipt = apply_plan(plan, None)
        self.assertEqual(len(receipt["applied"]), 1)
        moved = self.root / "Author/Book (2020)"
        self.assertTrue(moved.is_dir())
        self.assertEqual([p.name for p in moved.iterdir()], ["Book (2020).m4b"])
        self.assertEqual((moved / "Book (2020).m4b").read_bytes(), b"audio")
        self.assertFalse((self.root / "Author/Book {audible-XYZ} (2020)").exists())

    def test_apply_refuses_to_overwrite_an_existing_book_folder(self):
        touch(self.root / "Author/Book {audible-XYZ}/Book {audible-XYZ}.m4b", b"new")
        existing = touch(self.root / "Author/Book/Book.m4b", b"existing")
        receipt = apply_plan(build_plan(self.root), None)
        self.assertEqual(receipt["applied"], [])
        self.assertEqual(existing.read_bytes(), b"existing")
        self.assertTrue((self.root / "Author/Book {audible-XYZ}").is_dir())

    def test_apply_never_overwrites_an_existing_target(self):
        touch(self.root / "Author/Book/Old Name.m4b", b"new")
        dst = touch(self.root / "Author/Book/Book.m4b", b"existing")
        receipt = apply_plan(build_plan(self.root), None)
        self.assertEqual(receipt["applied"], [])
        self.assertEqual(dst.read_bytes(), b"existing")

    def test_apply_leaves_review_and_multipart_folders_alone(self):
        review = touch(self.root / "Unknown Author/Dune/Dune.m4b", b"")
        part1 = touch(self.root / "A/B/B Part 01.mp3", b"1")
        part2 = touch(self.root / "A/B/B Part 02.mp3", b"2")
        receipt = apply_plan(build_plan(self.root), None)
        self.assertEqual(receipt["applied"], [])
        self.assertTrue(review.exists() and part1.exists() and part2.exists())

    def test_limit_caps_the_number_of_renames(self):
        for i in (1, 2, 3):
            touch(self.root / f"Author/Book {i}/Old {i}.m4b")
        receipt = apply_plan(build_plan(self.root), 2)
        self.assertEqual(len(receipt["applied"]), 2)
        self.assertTrue(any(s["reason"] == "limit_reached" for s in receipt["skipped"]))


if __name__ == "__main__":
    unittest.main()
