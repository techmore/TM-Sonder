package library

import "testing"

func TestSplitTitleStripsLeadingYear(t *testing.T) {
	cases := []struct {
		in, display, sort string
		year              int
	}{
		{"2011 - The Martian", "The Martian", "martian", 2011},
		{"2021 - Project Hail Mary", "Project Hail Mary", "project hail mary", 2021},
		{"2017 - Artemis", "Artemis", "artemis", 2017},
		{"2019 - Randomize", "Randomize", "randomize", 2019},
	}
	for _, c := range cases {
		p := SplitTitle(c.in)
		if p.Display != c.display || p.Sort != c.sort || p.Year != c.year {
			t.Errorf("%q -> display=%q sort=%q year=%d, want %q/%q/%d",
				c.in, p.Display, p.Sort, p.Year, c.display, c.sort, c.year)
		}
	}
}

func TestSplitTitleStripsLeadingOrdinal(t *testing.T) {
	cases := []struct{ in, display, series string }{
		{"04 The Shadow Rising", "The Shadow Rising", "4"},
		{"06 Lord of Chaos", "Lord of Chaos", "6"},
		{"13 Towers of Midnight", "Towers of Midnight", "13"},
		{"03 - The State of the Art (BBC Adaptation)", "The State of the Art (BBC Adaptation)", "3"},
		{"14b River of Souls", "River of Souls", "14b"},
	}
	for _, c := range cases {
		p := SplitTitle(c.in)
		if p.Display != c.display || p.Series != c.series {
			t.Errorf("%q -> display=%q series=%q, want %q/%q",
				c.in, p.Display, p.Series, c.display, c.series)
		}
	}
}

func TestSplitTitleStripsTrailingYear(t *testing.T) {
	p := SplitTitle("Children of Ruin (2019)")
	if p.Display != "Children of Ruin" || p.Year != 2019 {
		t.Errorf("display=%q year=%d", p.Display, p.Year)
	}
	// A leading year wins over a trailing one; both are the same publication.
	q := SplitTitle("The Martian (2011)")
	if q.Display != "The Martian" || q.Year != 2011 {
		t.Errorf("display=%q year=%d", q.Display, q.Year)
	}
}

func TestSplitTitleLeavesRealTitlesAlone(t *testing.T) {
	// The dangerous failure mode is mangling a title that starts with a number
	// but is not a prefix, or stripping a series word that is part of the name.
	for _, in := range []string{
		"1984", "2001: A Space Odyssey", "Fahrenheit 451",
		"Book", "The Book of Bill", "A Wizard of Earthsea",
		"Stand on Zanzibar", "From a Buick 8",
	} {
		p := SplitTitle(in)
		if p.Display != in {
			t.Errorf("%q was mangled to %q", in, p.Display)
		}
	}
}

func TestSplitTitleDropsArticleFromSortOnly(t *testing.T) {
	p := SplitTitle("The Lathe of Heaven")
	if p.Display != "The Lathe of Heaven" {
		t.Errorf("display lost the article: %q", p.Display)
	}
	if p.Sort != "lathe of heaven" {
		t.Errorf("sort = %q, want %q", p.Sort, "lathe of heaven")
	}
	// An and A too.
	if got := SplitTitle("An Anthology").Sort; got != "anthology" {
		t.Errorf("An Anthology sort = %q", got)
	}
	if got := SplitTitle("A Study in Scarlet").Sort; got != "study in scarlet" {
		t.Errorf("A Study sort = %q", got)
	}
	// A title that merely starts with those letters is not an article.
	if got := SplitTitle("Theology of Sand").Sort; got != "theology of sand" {
		t.Errorf("Theology sort = %q", got)
	}
}

// This is the actual complaint: a shelf of Andy Weir books came out ordered by
// the leading year, with each title duplicated under a different name.
func TestSplitTitleMakesAWearShelfSortCorrectly(t *testing.T) {
	raw := []string{
		"2011 - The Martian (Read by R.C. Bray)",
		"2011 - The Martian (Read by Wil Wheaton)",
		"2017 - Artemis",
		"2017 - James Moriarty, Consulting Criminal",
		"2017 - The Egg and Other Stories",
		"2019 - Randomize",
		"2021 - Project Hail Mary",
		"Artemis (2017)",
		"Project Hail Mary (2021)",
		"Randomize (2019)",
		"The Martian (Read by R.C. Bray) (2011)",
		"The Martian (Read by Wil Wheaton) (2011)",
		"The Egg and Other Stories (2017)",
	}
	var keys []string
	for _, r := range raw {
		keys = append(keys, SplitTitle(r).Sort)
	}
	// The leading-year and no-prefix spellings of the same book must produce
	// the same key, which is what makes the shelf orderable and de-duplicable.
	byKey := map[string]int{}
	for _, k := range keys {
		byKey[k]++
	}
	// Every spelling of The Martian must land on one key, including the two
	// that differ only by a narrator credit.
	if byKey["martian"] != 4 {
		t.Errorf("The Martian spellings did not collapse onto one key: %v", byKey)
	}
	if byKey["project hail mary"] != 2 {
		t.Errorf("Project Hail Mary spellings did not collapse: %v", byKey)
	}
	if byKey["artemis"] != 2 {
		t.Errorf("Artemis spellings did not collapse: %v", byKey)
	}
	// No key may begin with a bare year, which was the original defect.
	for _, k := range keys {
		if k != "" && k[0] >= '0' && k[0] <= '9' && len(k) > 4 && k[4] == ' ' {
			t.Errorf("sort key still starts with a year: %q", k)
		}
	}
}

func TestSplitTitleHandlesEmptyAndWhitespace(t *testing.T) {
	// A name that was entirely a prefix must fall back to something showable
	// rather than becoming blank.
	for _, in := range []string{"()", "2011 - "} {
		if p := SplitTitle(in); p.Display == "" {
			t.Errorf("%q produced an empty display", in)
		}
	}
	for _, in := range []string{"", "   "} {
		if p := SplitTitle(in); p.Display != "" || p.Sort != "" {
			t.Errorf("%q produced %q/%q, want empty", in, p.Display, p.Sort)
		}
	}
}

// A narrator credit in the folder name must not split one book into two
// alphabetical positions, but it must still be visible to a reader.
func TestSplitTitleDropsNarratorFromSortNotDisplay(t *testing.T) {
	cases := []struct{ in, display, sort string }{
		{"Ubik (Daniels)", "Ubik (Daniels)", "ubik"},
		{"UBIK", "UBIK", "ubik"},
		{"The Lathe of Heaven (O'Malley)", "The Lathe of Heaven (O'Malley)", "lathe of heaven"},
		{"Dune (Read by R.C. Bray)", "Dune (Read by R.C. Bray)", "dune"},
		{"Valis (Gigante)", "Valis (Gigante)", "valis"},
	}
	for _, c := range cases {
		p := SplitTitle(c.in)
		if p.Display != c.display || p.Sort != c.sort {
			t.Errorf("%q -> display=%q sort=%q, want %q/%q",
				c.in, p.Display, p.Sort, c.display, c.sort)
		}
	}
	if p := SplitTitle("Ubik (Daniels)"); p.NarratorHint != "Daniels" {
		t.Errorf("narrator hint = %q, want %q", p.NarratorHint, "Daniels")
	}
}

func TestSplitTitleIsIdempotent(t *testing.T) {
	// Applying it to an already-clean title must not change it, so a caller can
	// run it on stored or derived names without checking first.
	for _, in := range []string{
		"Project Hail Mary", "The Lathe of Heaven", "Children of Ruin",
		"04 The Shadow Rising", "2011 - The Martian",
	} {
		once := SplitTitle(in)
		twice := SplitTitle(once.Display)
		if once.Display != twice.Display {
			t.Errorf("%q: second pass changed display %q -> %q",
				in, once.Display, twice.Display)
		}
		if once.Sort != twice.Sort {
			t.Errorf("%q: second pass changed sort %q -> %q", in, once.Sort, twice.Sort)
		}
	}
}

func TestSplitTitleSortIgnoresPunctuationAndCase(t *testing.T) {
	base := SplitTitle("Sci-Fi").Sort
	for _, variant := range []string{"Sci Fi", "sci-fi", "SCI-FI", "Sci_Fi"} {
		if got := SplitTitle(variant).Sort; got != base {
			t.Errorf("%q sort = %q, want %q", variant, got, base)
		}
	}
}
