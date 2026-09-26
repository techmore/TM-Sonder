const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function catalog(items, progress = []) {
  const source = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  // A minimal DOM: visibleItems() reads the filter selects through $, and the
  // book-part tests need to drive them ("watched" / "unwatched").
  const fields = { q: '', watched: 'all', sort: 'title', coverFilter: 'all' };
  const $ = sel => {
    const key = String(sel).replace(/^[#.]/, '');
    const node = { dataset: {}, hidden: false, style: {},
             classList: { toggle(){}, add(){}, remove(){}, contains(){ return false; } },
             addEventListener(){}, removeEventListener(){}, querySelector(){ return null; },
             querySelectorAll(){ return []; }, setAttribute(){}, getAttribute(){ return null; },
             appendChild(){}, remove(){}, focus(){}, closest(){ return null; },
             textContent: '', innerHTML: '', load(){}, play(){ return Promise.resolve(); },
             pause(){}, duration: 0, currentTime: 0, paused: true, src: '' };
    // The selects are read through `.value`, and a test has to be able to set
    // them, so the property reads and writes the shared field rather than a copy.
    return Object.defineProperty(node, 'value', {
      get() { return fields[key] ?? ''; }, set(v) { fields[key] = v; }, configurable: true,
    });
  };
  const context = vm.createContext({ window: { Sonder: {
    api: value => value,
    escapeHTML: value => String(value),
    formatTime: value => String(value),
  } }, $, document: { createElement: () => ({ canPlayType: () => 'probably' }),
                     addEventListener(){}, querySelector: $, querySelectorAll(){ return []; },
                     body: { classList: { add(){}, remove(){} } } },
    navigator: {}, localStorage: { getItem: () => null, setItem(){}, removeItem(){} } });
  context.fields = fields;
  vm.runInContext(source.slice(0, source.indexOf('    function seasonLabel')), context);
  context.input = items;
  context.progress = progress;
  vm.runInContext('items = input; for (const p of progress) progressByID.set(p.id, p); rebuildCopyGroups();', context);
  return expression => JSON.parse(JSON.stringify(vm.runInContext(expression, context)));
}

const movie = (id, year, extra = {}) => ({ id, title: 'The Thing', kind: 'movie', year, format: 'mkv', ...extra });
const librarySource = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
const libraryHTML = fs.readFileSync(`${__dirname}/library.html`, 'utf8');

test('library shows its version and keeps audiobook layout in Settings', () => {
  assert.match(libraryHTML, /id="appVersion"/);
  assert.match(libraryHTML, /id="audiobookLayoutSel"/);
  assert.match(libraryHTML, /href="https:\/\/stoverparc\.org:8096\/#audiobooks"/);
  assert.match(libraryHTML, /<svg[^>]+class="size-6"/);
  assert.doesNotMatch(libraryHTML, /id="layoutBadge"|class="layout-badge"/);
  assert.doesNotMatch(libraryHTML, /data-tab="lists"/);
  assert.match(libraryHTML, /id="movieCatalog" class="catalog-layout detail-collapsed"/);
  assert.match(libraryHTML, /id="movieDetailToggle"[^>]+aria-expanded="false">Expand/);
  assert.match(libraryHTML, /id="pager"[\s\S]*id="listsPanel"/);
  assert.doesNotMatch(libraryHTML, /id="audiobookPlayerLink"/);
  assert.match(librarySource, /body\.audiobookLayout = document\.querySelector\("#audiobookLayoutSel"\)\.value/);
});

test('rail cards keep a fixed width even when titles are long', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  assert.match(css, /\.grid\.rail-mode \.card\s*\{[^}]*flex:0 0 165px;[^}]*min-width:0;/);
  assert.match(css, /\.card\s*\{\s*min-width:0;/);
});

test('Rails replaces the fallback grid and exposes complete content shelves', () => {
  assert.match(librarySource, /data-action="browse-tab"/);
  assert.match(librarySource, /data-action="expand-library-shelf"/);
  assert.match(librarySource, /\$\("#grid"\)\.hidden = true;/);
});

test('playback plan plays audio inline and transcodes unsupported video', () => {
  const get = catalog([
    { id: 'm4b', kind: 'audiobook', format: 'm4b' },
    { id: 'mp3', kind: 'audiobook', format: 'mp3' },
    { id: 'mp4', kind: 'movie', format: 'mp4' },
    { id: 'mkv', kind: 'movie', format: 'mkv' },
    { id: 'epub', kind: 'ebook', format: 'epub' },
  ]);
  // Audiobooks play inline: .m4b is MP4/AAC, not a special case.
  assert.deepEqual(get("playbackPlan(items[0])"), { mode: 'audio', url: '/stream/m4b' });
  assert.deepEqual(get("playbackPlan(items[1])"), { mode: 'audio', url: '/stream/mp3' });
  // Browser-safe video streams directly...
  assert.deepEqual(get("playbackPlan(items[2])"), { mode: 'video', url: '/stream/mp4' });
  // ...while containers browsers cannot decode fall back to the server transcode.
  assert.deepEqual(get("playbackPlan(items[3])"), { mode: 'video', url: '/stream/mkv?transcode=1' });
  // Books are not streamable.
  assert.equal(get("playbackPlan(items[4])"), null);
});

test('empty media tabs are omitted from the populated navigation set', () => {
  const get = catalog([
    movie('movie', 2024),
    { id: 'book', title: 'Book', kind: 'ebook', format: 'epub' },
    { id: 'placeholder', title: 'Missing', kind: 'documentary', isPlaceholder: true },
  ]);
  assert.deepEqual(
    get('Array.from(populatedLibraryTabs(items)).sort()'),
    ['all', 'books', 'movies', 'optimize', 'storage'],
  );
});

test('movie Rails shelves prioritize in-progress titles and keep catalog groups', () => {
  const get = catalog([
    movie('started', 2020, { title: 'Started', durationSeconds: 3600 }),
    movie('fresh', 2024, { title: 'Fresh', durationSeconds: 5400 }),
    movie('epic', 2021, { title: 'Epic', durationSeconds: 9000 }),
    movie('quick', 2022, { title: 'Quick', durationSeconds: 1800 }),
  ], [{ id: 'started', seconds: 120, duration: 3600, updatedAt: '2026-09-24T12:00:00Z' }]);
  assert.deepEqual(get('movieShelfGroups(items).continueWatching.map(i => i.id)'), ['started']);
  assert.equal(get('movieSpotlight(items).id'), 'started');
  assert.deepEqual(get('movieShelfGroups(items).featured.map(i => i.id)'), ['fresh', 'quick', 'epic']);
  assert.deepEqual(get('movieShelfGroups(items).quick.map(i => i.id)'), ['started', 'fresh', 'quick']);
  assert.deepEqual(get('movieShelfGroups(items).long.map(i => i.id)'), ['epic']);
});

test('movie detail starts collapsed and opens when a title is selected', () => {
  assert.match(librarySource, /let movieDetailCollapsed = true;/);
  assert.match(librarySource, /selectedMovieID = id;\n\s+movieDetailCollapsed = false;/);
});

test('quality copies group, while remakes and split parts stay distinct', () => {
  const get = catalog([
    movie('hd', 1982, { probedHeight: 1080 }), movie('uhd', 1982, { probedHeight: 2160 }),
    movie('remake', 2011), movie('part1', 1982, { splitPart: '1' }), movie('part2', 1982, { splitPart: '2' }),
  ]);
  assert.equal(get('copyGroups.size'), 4);
  assert.deepEqual(get('copiesOf(items[0]).map(i => i.id)'), ['uhd', 'hd']);
});

test('movie cards do not show a copy-count overlay', () => {
  const get = catalog([
    movie('hd', 1982, { probedHeight: 1080 }), movie('uhd', 1982, { probedHeight: 2160 }),
  ]);
  assert.doesNotMatch(get('cardHTML(items[0])'), /VERSIONS/);
});

test('codec suffixes do not create a second movie card', () => {
  const get = catalog([
    { id: 'blade-runner', title: 'Blade Runner', year: 2049, kind: 'movie', probedHeight: 800, format: 'mkv' },
    { id: 'blade-runner-xvid', title: 'Blade Runner XviD', year: 2049, kind: 'movie', format: 'mkv' },
  ]);
  assert.equal(get('copyGroups.size'), 1);
  assert.deepEqual(get('copiesOf(items[0]).map(i => i.id)'), ['blade-runner', 'blade-runner-xvid']);
});

test('shared movie metadata IDs join alternate titles and release labels', () => {
  const get = catalog([
    { id: 'tagged', title: 'The Matrix', year: 1999, kind: 'movie', metadataIDSource: 'imdb', metadataID: 'tt0133093' },
    { id: 'renamed', title: 'Matrix XviD', year: 1999, kind: 'movie', metadataIDSource: 'imdb', metadataID: 'TT0133093' },
  ]);
  assert.equal(get('copyGroups.size'), 1);
  assert.equal(get('copiesOf(items[0]).length'), 2);
});

test('sequel numbers, split parts, and comparison extras stay separate', () => {
  const get = catalog([
    { id: 'predator', title: 'Predator', year: 1987, kind: 'movie' },
    { id: 'predator-2', title: 'Predator 2', year: 1987, kind: 'movie' },
    { id: 'merlin-1', title: 'Merlin Part I', year: 1998, kind: 'movie' },
    { id: 'merlin-2', title: 'Merlin Part II', year: 1998, kind: 'movie' },
    { id: 'parade', title: 'Storyboard Comparisons - The Parade Scene', kind: 'movie' },
    { id: 'ruins', title: 'Storyboard Comparisons - The Ruins Scene', kind: 'movie' },
  ]);
  assert.equal(get('copyGroups.size'), 6);
});

test('resume position takes priority over resolution', () => {
  const get = catalog([movie('hd', 1982, { probedHeight: 1080 }), movie('uhd', 1982, { probedHeight: 2160 })],
    [{ id: 'hd', seconds: 100, duration: 1000 }]);
  assert.equal(get('copiesOf(items[0])[0].id'), 'hd');
});

test('TV groups by show and episode, with one episode per quality set', () => {
  const episode = (id, showTitle, episodeNumber, probedHeight) => ({ id, kind: 'tvShow', title: 'Pilot', showTitle, seasonNumber: 1, episodeNumber, probedHeight });
  const get = catalog([episode('a', 'Show A', 1, 720), episode('b', 'Show A', 1, 1080), episode('c', 'Show B', 1, 1080), episode('d', 'Show A', 2, 720)]);
  assert.equal(get('copyGroups.size'), 3);
  assert.deepEqual(get('buildShowGroups().map(s => [...s.seasons.values()].flat().map(e => e.id))'), [['b', 'd'], ['c']]);
});

test('fuzzy matching cannot merge different release years', () => {
  const get = catalog([movie('a', 1982, { title: 'Long Movie Name' }), movie('b', 2011, { title: 'Long Movie Names' })]);
  assert.equal(get('copyGroups.size'), 2);
});

test('All page has one show card, never a card for each episode', () => {
 const get = catalog([1,2,3].map(n => ({id:String(n),title:'Jack '+n,kind:'tvShow',showTitle:'Samurai Jack',seasonNumber:n,episodeNumber:1})));
 assert.deepEqual(get('mainPageEntries(items).map(i => i.title)'), ['Samurai Jack']);
 assert.equal(get('mainPageEntries(items)[0].show.seasons.size'), 3);
});

test('folder identity collapses show aliases but retains mixed-show episodes', () => {
 const episode = (id,showTitle) => ({id,kind:'tvShow',title:'Pilot',showTitle,showGroupID:'folder-jack',showGroupTitle:'Samurai Jack (2001)',seasonNumber:1,episodeNumber:1});
 const get=catalog([episode('a','Samurai Jack'),episode('b','Samurai Jack (2001)'),episode('c','SamuraiChamploo')]);
 assert.equal(get('buildShowGroups().length'),1);
 assert.equal(get('buildShowGroups()[0].seasons.get(1).length'),2);
 assert.equal(get('copiesOf(items[0]).length'),2);
 assert.equal(get('copiesOf(items[2]).length'),1);
});

test('unknown episode numbers never merge same-title files across shows', () => {
 const get=catalog(['a','b'].map(id=>({id,kind:'tvShow',title:'Extras',showTitle:'Show '+id,showGroupID:id})));
 assert.equal(get('copyGroups.size'),2);
 assert.equal(get('buildShowGroups().length'),2);
});

test('TV show cards honor the visible episode filter and search episode metadata', () => {
 const get = catalog([
   {id:'a',kind:'tvShow',title:'Pilot',subtitle:'Alpha - S01E01',showTitle:'Alpha',showGroupID:'alpha',showGroupTitle:'Alpha',seasonNumber:1,episodeNumber:1},
   {id:'b',kind:'tvShow',title:'The Finale',subtitle:'Beta - S01E01',showTitle:'Beta',showGroupID:'beta',showGroupTitle:'Beta',seasonNumber:1,episodeNumber:1},
 ]);
 assert.deepEqual(get('buildShowGroups(new Set(["a"])).map(s => s.name)'), ['Alpha']);
 assert.match(get('buildShowGroups().find(s => s.name === "Beta").searchText'), /Finale/);
});

test('packed episode duplicate suffix groups only with same-season sibling', () => {
 const ep=(id,title,seasonNumber)=>({id,title,seasonNumber,kind:'tvShow',showTitle:'Jack',showGroupID:'jack'});
 const get=catalog([ep('a','102 - Jack',1),ep('b','102 - Jack-1',1),ep('c','102 - Jack-1',2),ep('d','103 - Different-1',1)]);
 assert.equal(get('copyGroups.size'),3);
 assert.equal(get('copiesOf(items[0]).length'),2);
 assert.equal(get('copiesOf(items[2]).length'),1);
});

// Facet rendering needs just enough DOM to hold innerHTML; `$` reads the
// `document` global lazily, so it can be injected after the script runs.
function facetHarness(items, state = '') {

  const source = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  const nodes = new Map();
  const node = key => {
    if (!nodes.has(key)) {
      nodes.set(key, {
        innerHTML: '', textContent: '', hidden: false, value: '', placeholder: '',
        classList: { toggle() {}, remove() {}, add() {}, contains: () => false },
        querySelector: () => null, focus() {},
      });
    }
    return nodes.get(key);
  };
  const context = vm.createContext({ window: { Sonder: { escapeHTML: s => String(s) } } });
  vm.runInContext(source.slice(0, source.indexOf('    function seasonLabel')), context);
  context.document = { querySelector: node };
  context.input = items;
  vm.runInContext(`items = input; activeTab = "all"; ${state} renderFacets();`, context);
  const html = () => node('#metadataChips').innerHTML;
  return {
    node,
    chips: () => (html().match(/data-facet-value=/g) || []).length,
    categories: () => [...html().matchAll(/data-genre-category="([^"]+)"/g)].map(m => m[1]),
    html,
    footer: () => node('#chipsFooter').innerHTML,
    run: expression => vm.runInContext(expression, context),
  };
}

// 40 single-count subjects: over the collapse threshold, but stable to assert on.
const subjects = n => Array.from({ length: n }, (_, i) => ({
  id: `b${i}`, kind: 'ebook', title: `Book ${i}`, tags: [`subject ${i}`, 'open-library'],
}));

test('genres lead with super-categories, not one chip per subject', () => {
  const h = facetHarness(subjects(40));
  // Level 1 is navigation only: "All" plus one button per populated category.
  assert.equal(h.chips(), 1);
  assert.deepEqual(h.categories(), ['unsorted']);
  assert.match(h.html(), /data-genre-category="unsorted"[^>]*>Unsorted<span class="chip-count">40</,
    'category chip carries the number of titles it covers');
  assert.doesNotMatch(h.html(), /data-facet-value="subject 0"/);
});

test('opening a category lists its sub-genres, still collapsed', () => {
  const h = facetHarness(subjects(40));
  h.run('genreCategory = "unsorted"; renderFacets();');
  // "All" plus the 18 most-used sub-genres, the rest behind a footer toggle.
  assert.equal(h.chips(), 19);
  assert.match(h.html(), /data-genre-back/);
  assert.match(h.footer(), /\+22 more/);
  assert.equal(h.node('#chipsFooter').hidden, false);
  // Nothing lives in a category chip's worth of markup any more.
  assert.deepEqual(h.categories(), []);
});

test('expanding and searching reveal every facet value', () => {
  const h = facetHarness(subjects(40), 'genreCategory = "unsorted"; chipsExpanded = true;');
  assert.equal(h.chips(), 41);
  assert.match(h.footer(), /Show fewer/);
  // A search is never collapsed and never stays inside one category: hiding a
  // match would misreport the result.
  const searched = facetHarness(subjects(40), 'genreCategory = "unsorted"; facetQuery = "subject 3";');
  assert.equal(searched.chips(), 12); // "subject 3" plus "subject 30".."subject 39"
  assert.equal(searched.footer(), '');
  assert.match(searched.html(), /data-facet-value="subject 30"/);
});

test('a selected value stays visible when the list is collapsed', () => {
  const h = facetHarness(subjects(40), 'genreCategory = "unsorted"; selectedFacetValues = new Set(["subject 39"]);');
  assert.equal(h.chips(), 20); // 18 most-used + the selection + All
  assert.match(h.html(), /data-facet-value="subject 39"/);
  assert.match(h.html(), /class="chip selected"[^>]*data-facet-value="subject 39"/);
});

test('super-categories place the subjects this library actually has', () => {
  const h = facetHarness([]);
  const cases = {
    'science fiction': 'fiction',
    'history': 'history',
    'world war, 1939-1945': 'history',
    'psychology': 'psychology',
    'hinduism': 'philosophy',
    'political science': 'politics',
    'political science--philosophy': 'philosophy', // philosophy is checked first
    'capitalism': 'politics',
    'investments': 'business',
    'machine learning': 'science',
    'computer crimes': 'science',                 // override beats /crime/
    'cooking (codfish)': 'lifestyle',
    'examinations, questions, etc.': 'reference',
    'judith butler': 'people',
    'lacan, jacques, 1901-1981': 'people',
    '320/.01': 'unsorted',
    'ja71 .b88 2000': 'unsorted',
  };
  for (const [value, expected] of Object.entries(cases)) {
    assert.equal(h.run(`genreCategoryFor(${JSON.stringify(value)})`), expected, value);
  }
});

test('provider markers and people never render as genres', () => {
  const h = facetHarness([
    { id: 'a', kind: 'ebook', title: 'A', author: 'Frank Herbert', tags: ['wikipedia', 'open-library', 'Frank Herbert', 'desert'] },
    { id: 'b', kind: 'ebook', title: 'B', narrator: 'Someone Else', tags: ['audnexus', 'Someone Else', 'politics'] },
  ], 'activeFacet = "genres";');
  assert.doesNotMatch(h.html(), /wikipedia|open-library|audnexus/i);
  assert.doesNotMatch(h.html(), /Frank Herbert|Someone Else/);
  h.run('facetQuery = "desert"; renderFacets();');
  assert.match(h.html(), /data-facet-value="desert"/);
  h.run('facetQuery = "politics"; renderFacets();');
  assert.match(h.html(), /data-facet-value="politics"/);
  h.run('facetQuery = "frank"; renderFacets();');
  assert.doesNotMatch(h.html(), /data-facet-value="Frank Herbert"/);
});

test('show navigation displays seasons before episodes and encodes names once', () => {
  const source = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  const nodes = new Map();
  const node = key => { if (!nodes.has(key)) nodes.set(key, {}); return nodes.get(key); };
  const context = vm.createContext({
    window: { Sonder: { escapeHTML: s => s, formatTime: s => String(s) } },
    document: { querySelector: node },
    location: { hash: '' },
    history: { pushState: (_a, _b, hash) => node('hash').value = hash },
  });
  vm.runInContext(source.slice(0, source.indexOf('    window.addEventListener("popstate"')), context);
  vm.runInContext(`items = [{id:'a', kind:'tvShow', title:'Pilot', showTitle:'Law & Order', seasonNumber:1, episodeNumber:1}];
    rebuildCopyGroups(); openShow = 'Law & Order'; renderShowPage(); syncHash(true);`, context);
  assert.match(node('#grid').innerHTML, /open-season/);
  assert.equal(node('#seasonList').innerHTML, '');
  assert.equal(node('hash').value, '#all/show/Law%20%26%20Order');
  vm.runInContext('openSeason = 1; renderShowPage();', context);
  assert.equal(node('#grid').innerHTML, '');
  assert.match(node('#seasonList').innerHTML, /Pilot/);
});

// The curated shelves, the candidate pools, and the rails built from them all
// live above the list-form wiring, so one harness reaches the whole engine
// without dragging in the rest of the page.
function shelfHarness(items, state = '') {
  const source = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  const cut = source.indexOf('    on("#listCreateForm"');
  const context = vm.createContext({
    window: {
      Sonder: { api: v => v, escapeHTML: v => String(v ?? ''), formatTime: v => String(v) },
      addEventListener: () => {},
    },
    document: { querySelector: () => null, querySelectorAll: () => [] },
  });
  vm.runInContext(source.slice(0, cut), context);
  context.input = items;
  vm.runInContext(`items = input; lists = []; ${state} rebuildCopyGroups();`, context);
  return expression => JSON.parse(JSON.stringify(vm.runInContext(expression, context)));
}

const film = (title, year, extra = {}) => ({ id: `${title}-${year}`, title, year, kind: 'movie', format: 'mkv', ...extra });
const doc = (title, year) => ({ id: `doc-${title}`, title, year, kind: 'documentary', format: 'mkv' });
const episode = (id, showTitle, seasonNumber, episodeNumber) =>
  ({ id, kind: 'tvShow', title: `Episode ${episodeNumber}`, showTitle, showGroupTitle: showTitle, seasonNumber, episodeNumber });

test('a curated entry with a year resolves the right cut of a remade title', () => {
  const get = shelfHarness([film('The Thing', 1982), film('The Thing', 2011)]);
  // Same normalized title, two catalog entries: only the year separates them.
  assert.equal(get('candidateForEntry(parseCuratedEntry("The Thing (1982)"), ["movie"]).item.year'), 1982);
  assert.equal(get('candidateForEntry(parseCuratedEntry("The Thing (2011)"), ["movie"]).item.year'), 2011);
});

test('a year-less curated entry still matches, and a wrong year does not', () => {
  const get = shelfHarness([film('Dune', 1984), film('Fargo', 1996)]);
  // No year on the entry: fall back to whatever the catalog holds.
  assert.equal(get('candidateForEntry(parseCuratedEntry("Fargo"), ["movie"]).item.title'), 'Fargo');
  // A year the catalog genuinely does not hold is a miss, not a near miss.
  assert.equal(get('candidateForEntry(parseCuratedEntry("Fargo (1978)"), ["movie"])'), null);
  assert.equal(get('candidateForEntry(parseCuratedEntry("Blade Runner (1982)"), ["movie"])'), null);
});

test('a shelf only matches titles inside the kinds it declares', () => {
  const get = shelfHarness([doc('The Cove', 2009), film('The Thing', 1982)]);
  assert.equal(get('candidateForEntry(parseCuratedEntry("The Cove (2009)"), ["movie"])'), null);
  assert.equal(get('candidateForEntry(parseCuratedEntry("The Cove (2009)"), ["documentary"]).item.kind'), 'documentary');
  // Books and audiobooks still share one pool, as they always did.
  assert.deepEqual(get('listCandidateIndex(CURATED_KINDS.book).records.length'), 0);
  assert.equal(get('curatedListsForKinds(["movie"]).every(l => l.kinds.includes("movie"))'), true);
  assert.equal(get('curatedListsForKinds(["documentary"]).some(l => l.kinds.includes("ebook"))'), false);
});

test('TV shelves match show groups rather than individual episodes', () => {
  const get = shelfHarness([
    episode('a', 'The Wire', 1, 1), episode('b', 'The Wire', 1, 2), episode('c', 'The Wire', 2, 1),
    episode('d', 'Succession', 1, 1),
  ]);
  // Three episodes of one show are one candidate, and it knows its seasons.
  const records = get('listCandidateIndex(["tvShow"]).records');
  assert.equal(records.length, 2);
  const wire = records.find(r => r.title === 'The Wire');
  assert.equal(wire.show.name, 'The Wire');
  // All three Wire episodes hang off the one candidate, across two seasons.
  assert.deepEqual(
    get('[...listCandidateIndex(["tvShow"]).records.find(r => r.title === "The Wire").show.seasons.values()].flat().map(e => e.id).sort()'),
    ['a', 'b', 'c'],
  );
  // The show name is what a list entry names; the episode title is not.
  assert.equal(get('candidateForEntry(parseCuratedEntry("The Wire"), ["tvShow"]).show.id'), 'The Wire');
  assert.equal(get('candidateForEntry(parseCuratedEntry("Succession"), ["tvShow"]).show.id'), 'Succession');
});

test('a curated shelf becomes a rail only once the library covers enough of it', () => {
  const partial = shelfHarness([film('The Departed', 2006), film('No Country for Old Men', 2007)]);
  assert.equal(partial('curatedShelves(["movie"]).length'), 0, 'two of fifty is not a shelf worth browsing');

  const covered = shelfHarness([
    film('The Departed', 2006), film('No Country for Old Men', 2007), film('Slumdog Millionaire', 2008),
    film('The Hurt Locker', 2009), film("The King's Speech", 2010), film('Unrelated Comedy', 2021),
  ]);
  const shelves = covered('curatedShelves(["movie"], 3).map(s => ({ name: s.list.name, owned: s.records.length, sub: curatedShelfSub(s), cards: curatedShelfCards(s).length }))');
  const awardShelf = shelves.find(s => s.name === 'Recent Award Winners');
  assert.ok(awardShelf, 'the most-covered shelf is offered');
  assert.equal(awardShelf.owned, 5);
  assert.equal(awardShelf.cards, 5);
  // The subtitle states the gap, which is the discovery signal: what is left.
  assert.match(awardShelf.sub, /^5 of 25 owned · 20 still to find$/);
  // A rail keeps the shelf's own ranking, not the catalog's alphabetical order.
  assert.deepEqual(
    covered('curatedShelves(["movie"], 1)[0].records.slice(0, 5).map(r => r.item.title)'),
    ['The Departed', 'No Country for Old Men', 'Slumdog Millionaire', 'The Hurt Locker', "The King's Speech"],
  );
});

test('a curated television rail shows series cards, not episode posters', () => {
  const get = shelfHarness(
    ['The Sopranos', 'The Wire', 'Breaking Bad', 'Succession', 'The Leftovers']
      .flatMap(name => [episode(`a-${name}`, name, 1, 1), episode(`b-${name}`, name, 1, 2)]),
  );
  const cards = get('curatedShelfCards(curatedShelves(["tvShow"], 1)[0])');
  assert.equal(cards.length, 5);
  assert.match(cards[0], /data-action="open-show"/);
  assert.doesNotMatch(cards[0], /data-action="open-detail"/);
});

test('a saved shelf offers to reopen itself instead of adding a duplicate', () => {
  const get = shelfHarness([film('The Departed', 2006)], 'lists = [{ id: "l1", name: "Recent Award Winners" }];');
  // Nothing in this library is covered, so reach for the shelf object directly.
  const control = get('curatedShelfControl({ list: { id: "curated-0", name: "Recent Award Winners" } })');
  assert.match(control, /Saved · open/);
  assert.match(control, /data-action="use-recommended-list"/);
  get('lists = []');
  assert.match(
    get('curatedShelfControl({ list: { id: "curated-0", name: "Recent Award Winners" } })'),
    /Save shelf/,
  );
});

test('the browser layout never overrides the palette, so every theme is selectable', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  // The layout is a structure choice. When it re-declared the palette tokens it
  // outranked every theme block and the picker silently did nothing.
  const block = css.slice(css.indexOf('/* ── Rails browser'), css.indexOf('/* ── Persistent now-playing'));
  assert.doesNotMatch(block, /--(bg|panel|panel2|line|text|muted|accent|gold|accent-dark)\s*:/,
    'the Rails layout declares no palette tokens');
  // Earthy is the :root default; every other palette declares its own tokens at
  // body level, where it outranks the defaults.
  assert.match(css, /:root \{[^}]*--bg:/);
  for (const theme of ['techmore', 'bunny']) {
    assert.match(css, new RegExp(`body\\[data-theme="${theme}"\\][^{]*\\{[^}]*--bg:`), theme);
  }
  assert.match(libraryHTML, /<option value="bunny">Space Bunny<\/option>/);
});

test('no palette token is aliased on :root, where var() would freeze it to Earthy', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  // A custom property declared on :root that references another custom property
  // resolves against :root's value, not the body-level theme value. `--chip-bg:
  // var(--panel2)` therefore stayed pinned to the Earthy dark #1f2618 and every
  // theme put its own text colour on a dark chip. Palettes own their tokens.
  const aliases = [...css.matchAll(/^\s*:root\s*\{([^}]*)\}/gm)]
    .flatMap(([, block]) => [...block.matchAll(/(--[\w-]+)\s*:\s*var\(/g)].map(m => m[1]));
  assert.deepEqual(aliases, [], `aliased on :root: ${aliases.join(', ')}`);
});

test('the phone layout docks the tab bar and stops the page scrolling sideways', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  const phone = css.slice(css.indexOf('@media (max-width: 700px)'));
  assert.ok(phone.length > 0, 'a phone breakpoint exists');
  // Docked, so a thumb reaches the catalog switcher without scrolling past
  // the header. It stays the same <nav>, so tab state logic is untouched.
  assert.match(phone, /nav\.tabs\s*\{[^}]*position:\s*fixed/);
  assert.match(phone, /nav\.tabs\s*\{[^}]*overflow-x:\s*auto/);
  assert.match(phone, /nav\.tabs\s*\{[^}]*env\(safe-area-inset-bottom/);
  // The tab bar was the widest thing on the page (567px inside a 393px
  // viewport) and was not a scroll container, so the whole page panned.
  assert.match(phone, /\.header-tools\s*\{[^}]*overflow-x:\s*auto/);
  assert.match(phone, /\.filter-modes\s*\{[^}]*flex-wrap:\s*nowrap[^}]*overflow-x:\s*auto/);
  // The facet chip wall was 484px of an 852px screen.
  assert.match(phone, /#metadataChips\s*\{[^}]*flex-wrap:\s*nowrap/);
  // Touch targets were 21-29px tall.
  assert.match(phone, /min-height:\s*44px/);
  // backdrop-filter on the header would make it the containing block for the
  // fixed tab bar and trap it inside the sticky header.
  assert.match(phone, /header\s*\{[^}]*backdrop-filter:\s*none/);
});

test('the phone movie detail is an opaque bottom sheet, not a side rail', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  const phone = css.slice(css.indexOf('@media (max-width: 700px)'));
  // Tapping a card used to update a panel below the entire page, so it looked
  // like the tap did nothing.
  assert.match(phone, /body \.catalog-layout \.catalog-detail\s*\{[^}]*position:\s*fixed/);
  assert.match(phone, /body \.catalog-layout \.catalog-detail\s*\{[^}]*max-height:\s*84vh/);
  // Opaque, so shelf art does not bleed through the synopsis. The selector
  // needs the body prefix to outrank a palette's translucent panel gradient.
  assert.match(phone, /body \.catalog-layout \.catalog-detail\s*\{[^}]*background:\s*var\(--panel\)[^}]*background-image:\s*none/);
  // Collapsed means "no sheet", not the 54px desktop sliver.
  assert.match(phone, /\.catalog-layout\.detail-collapsed \.catalog-detail\s*\{\s*display:\s*none/);
});

test('the header control cluster is a display:contents wrapper on desktop', () => {
  // The phone layout needs a hook to make the search and the filter selects one
  // swipeable row. `display: contents` gives it one without changing the
  // desktop header, which must stay a single flex row.
  assert.match(libraryHTML, /<div class="header-tools">[\s\S]*id="watched"[\s\S]*id="sort"[\s\S]*id="settingsBtn"[\s\S]*<\/div>/);
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  assert.match(css, /\.header-tools\s*\{\s*display:\s*contents/);
});

test('Space Bunny is a registered palette, not just a stylesheet', () => {
  // The Go page renderer rewrites data-theme before the first paint and falls
  // back to earthy for an unknown preset, so an unregistered name would be
  // served as Earthy and then re-painted as Space Bunny.
  const webui = fs.readFileSync(`${__dirname}/../webui.go`, 'utf8');
  assert.match(webui, /preset != "dark" && preset != "techmore" && preset != "bunny"/);
  const handlers = fs.readFileSync(`${__dirname}/../handlers.go`, 'utf8');
  assert.match(handlers, /case "bunny":[\s\S]{0,320}?Accent:\s+"#6FE3C3"/);
});

// --- multi-part books -----------------------------------------------------
//
// A book delivered as many files is one book. The real library has a 147-file
// recording whose head part is 142 seconds and whose whole runtime is 7.8
// hours: before this, the card read "2:22" and pressing play streamed that one
// file and stopped.

const part = (id, bookGroupID, bookPartIndex, durationSeconds, extra = {}) => ({
  id, title: 'Complications: A Surgeon\'s Notes', kind: 'audiobook', year: 2003,
  format: 'm4b', bookGroupID, bookPartIndex, bookPartCount: 3,
  durationSeconds, ...extra,
});

test('a multi-part book is one card, represented by its head part', () => {
  const get = catalog([
    part('p1', 'bk', 1, 142), part('p2', 'bk', 2, 900), part('p3', 'bk', 3, 800),
    { id: 'single', title: 'Dune', kind: 'audiobook', year: 1965, format: 'm4b', durationSeconds: 3600 },
  ]);
  const shown = get('visibleItems().map(i => i.id)');
  assert.deepEqual(shown.sort(), ['p1', 'single']);
  // The head part is what represents the book, never a trailing file.
  assert.equal(get('visibleItems().some(i => i.id === "p2")'), false);
});

test('a book collapses to one card even with dedup switched off', () => {
  // The collapse is about the book's identity, not about duplicate suppression.
  // Leaving it inside the dedup branch meant "show every copy" turned a
  // 147-part book back into 147 cards.
  const get = catalog([
    part('p1', 'bk', 1, 142), part('p2', 'bk', 2, 900), part('p3', 'bk', 3, 800),
  ]);
  assert.equal(get('(dedupEnabled = false, visibleItems().length)'), 1);
});

test('a book card reports the whole runtime, not one file', () => {
  const get = catalog([part('p1', 'bk', 1, 142), part('p2', 'bk', 2, 900), part('p3', 'bk', 3, 800)]);
  // The harness stubs formatTime to String(), so this asserts the *total* the
  // card is given -- 1842s rather than the head part's 142s.
  assert.equal(get('runtimeLabel(items[0])'), '1842');
  assert.match(get('cardHTML(items[0])'), /3 files/);
});

test('two different books that share a title stay two cards', () => {
  // The grouping is by bookGroupID, never by title: two recordings of the same
  // book are different books.
  const get = catalog([
    part('a1', 'bkA', 1, 600), part('a2', 'bkA', 2, 600),
    part('b1', 'bkB', 1, 700), part('b2', 'bkB', 2, 700),
  ]);
  assert.equal(get('visibleItems().length'), 2);
});

test('parts are ordered by their index, not by arrival', () => {
  const get = catalog([part('p3', 'bk', 3, 800), part('p1', 'bk', 1, 142), part('p2', 'bk', 2, 900)]);
  // Only the head part is a key: a trailing file is reachable *through* the
  // head, which is why the head is what represents the book.
  assert.equal(get('partsOf(items[0])'), null);
  assert.deepEqual(get('partsOf(items.find(i => i.id === "p1")).map(p => p.id)'), ['p1', 'p2', 'p3']);
});

test('a book is not finished until every part is', () => {
  // One part's seconds against the whole book's runtime reported a completed
  // recording as barely started.
  const get = catalog(
    [part('p1', 'bk', 1, 1000), part('p2', 'bk', 2, 1000), part('p3', 'bk', 3, 1000)],
    [{ id: 'p1', seconds: 990, duration: 1000 }, { id: 'p2', seconds: 990, duration: 1000 }],
  );
  const agg = get('progressForItem(items[0])');
  assert.equal(agg.seconds, 1980);
  assert.equal(agg.duration, 3000);
  assert.equal(get('isWatched(progressForItem(items[0]), items[0])'), false);
  assert.equal(get('inProgress(progressForItem(items[0]), items[0])'), true);
});

test('a single-file book is untouched by any of this', () => {
  const get = catalog([{ id: 'dune', title: 'Dune', kind: 'audiobook', year: 1965, format: 'm4b', durationSeconds: 3600 }],
                       [{ id: 'dune', seconds: 1800, duration: 3600 }]);
  assert.equal(get('partsOf(items[0])'), null);
  assert.equal(get('progressForItem(items[0]).seconds'), 1800);
  assert.equal(get('runtimeLabel(items[0])'), '3600');
  assert.equal(get('visibleItems().length'), 1);
});

test('resume opens the part that was started, not the first one', () => {
  const get = catalog(
    [part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100), part('p3', 'bk', 3, 100)],
    [{ id: 'p1', seconds: 100, duration: 100 }, { id: 'p2', seconds: 40, duration: 100 }],
  );
  assert.equal(get('resumePartIndex(partsOf(items[0]))'), 1);
});

test('resume skips parts that are already finished', () => {
  const get = catalog(
    [part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100), part('p3', 'bk', 3, 100)],
    [{ id: 'p1', seconds: 100, duration: 100 }, { id: 'p2', seconds: 50, duration: 100 }],
  );
  assert.equal(get('resumePartIndex(partsOf(items.find(i => i.id === "p1")))'), 1);
});

test('a book nobody started opens at part one', () => {
  const get = catalog([part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100)]);
  assert.equal(get('resumePartIndex(partsOf(items[0]))'), 0);
});

test('the player walks the parts and stops after the last one', () => {
  const get = catalog([part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100), part('p3', 'bk', 3, 100)]);
  // advanceBookPart walks forward and reports when the book is over, which is
  // what stops the "ended" handler from looping past the last file.
  assert.equal(get('(startBookPlaybackTest = (nowPlayingParts = partsOf(items[0]), nowPlayingPartIndex = 0, advanceBookPart()))'), true);
  assert.equal(get('nowPlayingPartIndex'), 1);
  assert.equal(get('advanceBookPart()'), true);
  assert.equal(get('nowPlayingPartIndex'), 2);
  assert.equal(get('advanceBookPart()'), false);
});

test('progress is written to the part being played, not the book head', () => {
  // The server keys progress by item id, and the head row's id is a real file
  // the listener is not currently hearing. Writing the head's id would resume
  // the book at the wrong file.
  const get = catalog([part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100)]);
  assert.equal(get('(nowPlayingParts = partsOf(items[0]), nowPlayingPartIndex = 1, currentProgressTargetID())'), 'p2');
  assert.equal(get('(nowPlayingParts = null, currentProgressTargetID())'), null);
});

test('the now-playing panel says which part of the book is playing', () => {
  const get = catalog([part('p1', 'bk', 1, 100), part('p2', 'bk', 2, 100), part('p3', 'bk', 3, 100)]);
  assert.equal(get('(nowPlayingParts = partsOf(items[0]), nowPlayingPartIndex = 2, bookPartLabel())'), 'Part 3 of 3');
});

test('a single-file book is not part-labelled', () => {
  const get = catalog([{ id: 'dune', title: 'Dune', kind: 'audiobook', year: 1965, format: 'm4b', durationSeconds: 3600 }]);
  assert.equal(get('bookPartLabel()'), '');
});

// --- the phone player ------------------------------------------------------

test('the tab bar is icon-only on a phone but keeps its accessible names', () => {
  // Hiding the labels must not take the accessible name with them, so the text
  // lives in a .tab-label span and each button carries an aria-label.
  for (const tab of ['all', 'movies', 'tvshows', 'documentaries', 'audiobooks',
                     'books', 'storage', 'optimize']) {
    const re = new RegExp(`<button data-tab="${tab}"[^>]*aria-label="[^"]+"[^>]*>` +
                          `<span class="tab-ico" aria-hidden="true">`);
    assert.match(libraryHTML, re, `tab ${tab} needs an aria-label and an icon span`);
  }
  assert.match(libraryHTML, /<span class="tab-label">Audiobooks<\/span>/);

  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  const phone = css.slice(css.indexOf('@media (max-width: 700px)'));
  assert.match(phone, /nav\.tabs \.tab-label \{ display: none; \}/);
  // Equal shares, so all eight fit without the bar scrolling sideways. Two
  // sections being off-screen with no affordance was the original complaint.
  assert.match(phone, /nav\.tabs button \{\s*flex: 1 1 0;/);
});

test('the phone grid is denser than the desktop one', () => {
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  const phone = css.slice(css.indexOf('@media (max-width: 700px)'));
  const m = phone.match(/main\.grid \{ grid-template-columns: repeat\(auto-fill, minmax\((\d+)px/);
  assert.ok(m, 'the phone grid overrides its column width');
  assert.ok(Number(m[1]) < 132, `phone cards got bigger (${m[1]}px), not denser`);
});

test('the player shows time left in the whole book, not the current file', () => {
  // A 7-hour book on a 5-minute file would otherwise always read "4:55 left",
  // which reads as though the book were nearly over.
  const get = catalog([part('p1', 'bk', 1, 300), part('p2', 'bk', 2, 300), part('p3', 'bk', 3, 300)]);
  assert.equal(get('(nowPlayingParts = partsOf(items[0]), nowPlayingPartIndex = 0, bookTimeline().total)'), 900);
  assert.equal(get('bookTimeline().offset'), 0);
  assert.equal(get('bookTimeline().multi'), true);
  // Part 2 starts 300s into the book, not at zero.
  assert.equal(get('(nowPlayingPartIndex = 1, bookTimeline().offset)'), 300);
  // ...and part 3 at 600.
  assert.equal(get('(nowPlayingPartIndex = 2, bookTimeline().offset)'), 600);
});

test('a single-file item reports its own duration and position', () => {
  const get = catalog([{ id: 'dune', title: 'Dune', kind: 'audiobook', year: 1965, format: 'm4b', durationSeconds: 3600 }]);
  assert.equal(get('(nowPlayingParts = null, bookTimeline().total)'), 0);
  assert.equal(get('bookTimeline().multi'), false);
});

test('the seek bar addresses the whole book', () => {
  // iOS reports seekto in whatever coordinates setPositionState published. If
  // the bar were per-file, the lock screen would drive a 7-hour book from a
  // 5-minute file's range.
  const get = catalog([part('p1', 'bk', 1, 600), part('p2', 'bk', 2, 600), part('p3', 'bk', 3, 600)]);
  const src = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  // The slider maps through bookTimeline, not media.duration.
  assert.match(src, /const target = \(Number\(event\.target\.value\) \/ 1000\) \* tl\.total;/);
  assert.doesNotMatch(src, /Number\(event\.target\.value\) \/ 1000\) \* media\.duration/);
  // And the lock screen routes through the same function rather than assigning
  // media.currentTime, which would be per-file.
  const seekto = src.slice(src.indexOf('handler("seekto"'), src.indexOf('handler("stop"'));
  assert.match(seekto, /seekBookTo\(details\.seekTime\)/);
  assert.doesNotMatch(seekto, /media\.currentTime = details\.seekTime/);
  assert.equal(get('(nowPlayingParts = partsOf(items[0]), nowPlayingPartIndex = 2, bookTimeline().offset)'), 1200);
});

test('the lock screen gets the author, the narrator and the series', () => {
  // An audiobook's useful credits are author/narrator/series; "album" was only
  // ever a TV show's title, so the island showed a blank line.
  const src = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  assert.match(src, /narr\. \$\{item\.narrator\}/);
  assert.match(src, /album: item\.showTitle \|\| item\.series/);
});

test('the player panel has an ETA element and a thumb-sized pause', () => {
  assert.match(libraryHTML, /id="npEta"/);
  const css = fs.readFileSync(`${__dirname}/library.css`, 'utf8');
  const phone = css.slice(css.indexOf('@media (max-width: 700px)'));
  assert.match(phone, /\.np-play \{[^}]*min-height: 48px/);
  // And it is only shown when there is a book to have an ETA for.
  const src = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  assert.match(src, /if \(!tl\.multi \|\| !\(tl\.total > 0\)\) \{ eta\.hidden = true; return; \}/);
});

test('a book is filtered by the watched filter using its whole progress', () => {
  const get = catalog(
    [part('p1', 'bk', 1, 1000), part('p2', 'bk', 2, 1000)],
    [{ id: 'p1', seconds: 1000, duration: 1000 }, { id: 'p2', seconds: 1000, duration: 1000 }],
  );
  assert.equal(get('($("#watched").value = "watched", visibleItems().length)'), 1);
  assert.equal(get('($("#watched").value = "unwatched", visibleItems().length)'), 0);
});

