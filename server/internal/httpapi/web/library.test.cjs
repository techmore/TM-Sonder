const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function catalog(items, progress = []) {
  const source = fs.readFileSync(`${__dirname}/library.js`, 'utf8');
  const context = vm.createContext({ window: { Sonder: {
    api: value => value,
    escapeHTML: value => String(value),
    formatTime: value => String(value),
  } } });
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
  assert.doesNotMatch(libraryHTML, /data-tab="lists"/);
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
