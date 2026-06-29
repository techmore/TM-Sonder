enum SonderWebInterface {
    nonisolated static let html = """
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TM Sonder</title>
  <style>
    :root { color-scheme: dark; --bg:#10130f; --panel:#171d15; --panel2:#202819; --line:#344128; --text:#eff4e8; --muted:#a8b39c; --accent:#b6d56d; }
    body { margin:0; font:14px -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:var(--bg); color:var(--text); }
    header { position:sticky; top:0; z-index:1; display:flex; align-items:center; gap:16px; padding:18px 22px; background:rgba(16,19,15,.94); border-bottom:1px solid var(--line); }
    h1 { margin:0; font-size:26px; }
    input, select { background:var(--panel); border:1px solid var(--line); color:var(--text); border-radius:8px; padding:9px 11px; }
    main { display:grid; grid-template-columns:repeat(auto-fill,minmax(220px,1fr)); gap:16px; padding:22px; }
    article { background:var(--panel); border:1px solid var(--line); border-radius:8px; overflow:hidden; }
    .poster { aspect-ratio:2/3; display:grid; place-items:center; background:linear-gradient(145deg,#334124,#11160f); color:var(--accent); font-size:42px; }
    .body { padding:13px; }
    h2 { margin:0 0 4px; font-size:17px; }
    p { margin:0 0 10px; color:var(--muted); line-height:1.35; }
    progress { width:100%; accent-color:var(--accent); }
    video { width:100%; margin-top:10px; background:#050604; border-radius:6px; }
    .pill { display:inline-block; color:#11160f; background:var(--accent); border-radius:4px; padding:3px 6px; font-size:11px; font-weight:700; margin-bottom:8px; }
    .badge { display:inline-block; color:#11160f; background:#e0a14a; border-radius:4px; padding:2px 6px; font-size:10px; font-weight:700; margin-left:6px; }
    .toolbar { display:flex; gap:10px; align-items:center; padding:0 22px 6px; }
    button, a.button { background:var(--panel2); border:1px solid var(--line); color:var(--text); border-radius:8px; padding:8px 10px; text-decoration:none; cursor:pointer; }
    .show-hero { grid-column:1/-1; background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:18px; }
    .season { grid-column:1/-1; background:var(--panel); border:1px solid var(--line); border-radius:8px; overflow:hidden; }
    .season h2 { padding:13px; border-bottom:1px solid var(--line); }
    .episode { display:grid; grid-template-columns:1fr auto; gap:12px; align-items:center; padding:12px 13px; border-top:1px solid rgba(52,65,40,.55); }
  </style>
</head>
<body>
  <header>
    <h1>TM Sonder</h1>
    <input id="q" placeholder="Search movies, shows, documentaries">
    <select id="kind"><option>All</option><option>Movie</option><option>TV Show</option><option>Documentary</option><option>Audiobook</option><option>Book</option></select>
    <a class="button" href="/audiobooks">Audiobooks</a>
  </header>
  <main id="grid"></main>
  <script>
    let items = [];
    let selectedShowName = null;
    const grid = document.querySelector("#grid");
    const q = document.querySelector("#q");
    const kind = document.querySelector("#kind");

    // Browsers can only directly play a subset of the formats Sonder catalogs.
    // Formats the browser cannot render are still listed, but the inline player
    // is hidden so the user is not shown a broken player. Open the stream URL in
    // a native player (VLC, IINA, QuickTime) for those formats.
    const BROWSER_PLAYABLE = new Set(["mp4", "m4v", "mov", "webm"]);

    function render() {
      const selected = kind.value;
      if (selectedShowName) {
        renderShowDetail(selectedShowName);
        return;
      }
      if (selected === "TV Show") {
        renderShows();
        return;
      }
      renderCatalog(selected);
    }

    function renderCatalog(selected) {
      const term = q.value.toLowerCase();
      grid.innerHTML = items
        .filter(i => (selected === "All" || kindLabel(i.kind) === selected) && kindLabel(i.kind) !== "TV Show" && JSON.stringify(i).toLowerCase().includes(term))
        .map(renderMediaCard)
        .join("");
    }

    function renderShows() {
      const term = q.value.toLowerCase();
      const shows = groupShows().filter(show => JSON.stringify(show).toLowerCase().includes(term));
      grid.innerHTML = shows.map(show => `
        <article>
          <button class="poster" onclick="openShow('${escapeAttribute(show.name)}')">▶</button>
          <div class="body">
            <span class="pill">TV Show</span>
            <h2>${escapeHTML(show.name)}</h2>
            <p>${show.seasons.length} season(s), ${show.episodeCount} episode(s)</p>
            <button onclick="openShow('${escapeAttribute(show.name)}')">Open show</button>
          </div>
        </article>`).join("");
    }

    function renderShowDetail(name) {
      const show = groupShows().find(candidate => candidate.name === name);
      if (!show) { selectedShowName = null; render(); return; }
      grid.innerHTML = `
        <section class="show-hero">
          <button onclick="closeShow()">TV Shows</button>
          <h1>${escapeHTML(show.name)}</h1>
          <p>${show.seasons.length} season(s), ${show.episodeCount} episode(s). Direct-play episodes stream in the browser; catalog-only formats open through the stream URL.</p>
        </section>
        ${show.seasons.map(season => `
          <section class="season">
            <h2>${escapeHTML(seasonLabel(season.number))}</h2>
            ${season.episodes.map(renderEpisodeRow).join("")}
          </section>`).join("")}`;
    }

    function renderEpisodeRow(i) {
      const ext = (i.format || "").toLowerCase();
      const playable = i.hasFile && BROWSER_PLAYABLE.has(ext);
      const label = i.seasonNumber != null && i.episodeNumber != null ? `S${String(i.seasonNumber).padStart(2, "0")}E${String(i.episodeNumber).padStart(2, "0")} - ${escapeHTML(i.title)}` : escapeHTML(i.title);
      const action = playable
        ? `<video controls src="/stream/${i.id}" data-id="${i.id}" onloadedmetadata="resumeProgress(this)" ontimeupdate="saveProgress('${i.id}', this.currentTime, this.duration)"></video>`
        : i.hasFile
          ? `<a class="button" href="/stream/${i.id}" target="_blank">Open stream</a>`
          : `<span>File unavailable</span>`;
      return `<div class="episode"><div><strong>${label}</strong><p>${escapeHTML(i.subtitle)}</p><progress max="${Math.max(i.durationSeconds, 1)}" value="${progressFor(i.id).seconds}"></progress></div>${action}</div>`;
    }

    function renderMediaCard(i) {
      const ext = (i.format || "").toLowerCase();
      const playable = i.hasFile && BROWSER_PLAYABLE.has(ext);
      const player = playable
        ? `<video controls src="/stream/${i.id}" data-id="${i.id}" onloadedmetadata="resumeProgress(this)" ontimeupdate="saveProgress('${i.id}', this.currentTime, this.duration)"></video>`
        : i.hasFile
          ? `<p>Format <strong>${escapeHTML(ext || "unknown").toUpperCase()}</strong> is not playable in a browser. <a href="/stream/${i.id}" target="_blank">Open in a native player</a>.</p>`
          : `<p>Import a playable file in the Mac app to stream here.</p>`;
      return `
      <article>
        <div class="poster">▶</div>
        <div class="body">
          <span class="pill">${escapeHTML(kindLabel(i.kind))}</span>
          ${i.hasFile && !playable ? `<span class="badge">CATALOG ONLY</span>` : ""}
          <h2>${escapeHTML(i.title)}</h2>
          <p>${escapeHTML(i.subtitle)}</p>
          <progress max="${Math.max(i.durationSeconds, 1)}" value="${progressFor(i.id).seconds}"></progress>
          ${player}
        </div>
      </article>`;
    }
    function openShow(name) {
      selectedShowName = name;
      render();
    }
    function closeShow() {
      selectedShowName = null;
      render();
    }
    function groupShows() {
      const map = new Map();
      items.filter(i => kindLabel(i.kind) === "TV Show").forEach(item => {
        const name = item.showTitle || item.title;
        const show = map.get(name) || { name, seasonsByNumber:new Map(), seasons:[], episodeCount:0 };
        const seasonNumber = item.seasonNumber ?? -1;
        const season = show.seasonsByNumber.get(seasonNumber) || { number:seasonNumber, episodes:[] };
        season.episodes.push(item);
        show.seasonsByNumber.set(seasonNumber, season);
        show.episodeCount += item.isPlaceholder ? 0 : 1;
        map.set(name, show);
      });
      return Array.from(map.values()).map(show => {
        show.seasons = Array.from(show.seasonsByNumber.values()).sort((a, b) => seasonSort(a.number) - seasonSort(b.number));
        show.seasons.forEach(season => season.episodes.sort((a, b) => (a.episodeNumber ?? 0) - (b.episodeNumber ?? 0) || String(a.title).localeCompare(String(b.title))));
        return show;
      }).sort((a, b) => a.name.localeCompare(b.name));
    }
    function seasonSort(number) {
      return number === -1 ? Number.MAX_SAFE_INTEGER : number;
    }
    function seasonLabel(number) {
      if (number === -1) return "Unsorted Season";
      return number === 0 ? "Specials" : `Season ${number}`;
    }
    function kindLabel(value) {
      return ({ movie:"Movie", tvShow:"TV Show", documentary:"Documentary", audiobook:"Audiobook", ebook:"Book", all:"All" })[value] || value;
    }
    function escapeAttribute(value) {
      return String(value ?? "").replace(/\\/g, "\\\\").replace(/'/g, "\\'");
    }
    function escapeHTML(value) {
      return String(value ?? "").replace(/[&<>"']/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" }[c]));
    }
    function progressFor(id) {
      return progressByID.get(id) ?? { seconds:0, duration:1 };
    }
    function resumeProgress(video) {
      const progress = progressFor(video.dataset.id);
      if (progress.seconds > 5 && progress.seconds < Math.max(video.duration - 8, 0)) {
        video.currentTime = progress.seconds;
      }
    }
    async function saveProgress(id, seconds, duration) {
      if (!duration || Math.floor(seconds) % 15 !== 0) return;
      await fetch(`/api/progress/${id}`, { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({seconds, duration}) });
    }
    const progressByID = new Map();
    fetch("/api/library").then(r => r.json()).then(data => {
      items = data.items;
      (data.progress ?? []).forEach(p => progressByID.set(p.itemID, p));
      render();
    });
    q.oninput = render;
    kind.onchange = () => { selectedShowName = null; render(); };
  </script>
</body>
</html>
"""

    nonisolated static let audiobooksHTML = """
<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TM Sonder Audiobooks</title>
  <style>
    :root { color-scheme: dark; --bg:#10130f; --panel:#171d15; --panel2:#202819; --line:#344128; --text:#eff4e8; --muted:#a8b39c; --accent:#b6d56d; --warn:#e0a14a; }
    * { box-sizing:border-box; }
    body { margin:0; font:14px -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:var(--bg); color:var(--text); }
    header { position:sticky; top:0; z-index:2; display:grid; grid-template-columns:auto minmax(220px, 1fr) auto; gap:12px; align-items:center; padding:16px 20px; background:rgba(16,19,15,.96); border-bottom:1px solid var(--line); }
    h1 { margin:0; font-size:24px; }
    h2 { margin:0 0 5px; font-size:18px; }
    h3 { margin:0; font-size:15px; }
    p { margin:0; color:var(--muted); line-height:1.35; }
    input { width:100%; background:var(--panel); border:1px solid var(--line); color:var(--text); border-radius:8px; padding:10px 12px; }
    button, a.button { background:var(--panel2); border:1px solid var(--line); color:var(--text); border-radius:8px; padding:8px 10px; text-decoration:none; cursor:pointer; }
    button.primary { color:#11160f; background:var(--accent); border-color:var(--accent); font-weight:700; }
    main { display:grid; grid-template-columns:minmax(280px, 420px) minmax(0, 1fr); min-height:calc(100vh - 65px); }
    aside { border-right:1px solid var(--line); background:#12170f; overflow:auto; }
    .listHeader { display:flex; justify-content:space-between; align-items:center; padding:14px 16px; border-bottom:1px solid var(--line); }
    .book { display:grid; grid-template-columns:56px minmax(0,1fr); gap:12px; width:100%; padding:13px 16px; border:0; border-bottom:1px solid rgba(52,65,40,.65); border-radius:0; background:transparent; color:var(--text); text-align:left; }
    .book:hover, .book.selected { background:var(--panel); }
    .cover { width:56px; aspect-ratio:2/3; display:grid; place-items:center; border-radius:6px; overflow:hidden; background:linear-gradient(145deg,#334124,#11160f); color:var(--accent); font-weight:800; }
    .cover img { width:100%; height:100%; object-fit:cover; }
    .meta { display:flex; flex-wrap:wrap; gap:6px; margin-top:7px; color:var(--muted); font-size:12px; }
    .pill { color:#11160f; background:var(--accent); border-radius:4px; padding:2px 6px; font-size:11px; font-weight:700; }
    .detail { padding:22px; overflow:auto; }
    .hero { display:grid; grid-template-columns:150px minmax(0,1fr); gap:20px; align-items:start; margin-bottom:20px; }
    .hero .cover { width:150px; }
    audio { width:100%; margin:15px 0 12px; }
    progress { width:100%; accent-color:var(--accent); }
    .chapters { display:grid; gap:8px; margin-top:18px; }
    .chapter { display:grid; grid-template-columns:72px minmax(0,1fr) auto; gap:10px; align-items:center; padding:10px 12px; border:1px solid var(--line); border-radius:8px; background:var(--panel); }
    .chapter.current { border-color:var(--accent); background:#202819; }
    .time { color:var(--muted); font-variant-numeric:tabular-nums; }
    .empty { padding:30px; color:var(--muted); }
    @media (max-width: 820px) {
      header { grid-template-columns:1fr; }
      main { grid-template-columns:1fr; }
      aside { max-height:45vh; border-right:0; border-bottom:1px solid var(--line); }
      .hero { grid-template-columns:96px minmax(0,1fr); }
      .hero .cover { width:96px; }
      .chapter { grid-template-columns:62px minmax(0,1fr); }
      .chapter button { grid-column:1/-1; }
    }
  </style>
</head>
<body>
  <header>
    <h1>Audiobooks</h1>
    <input id="q" placeholder="Search title, author, narrator, series, or tag">
    <a class="button" href="/">Library</a>
  </header>
  <main>
    <aside>
      <div class="listHeader"><strong id="count">0 titles</strong><span class="time" id="updated"></span></div>
      <div id="results" class="empty">Loading audiobooks...</div>
    </aside>
    <section class="detail" id="detail"><p>Select an audiobook.</p></section>
  </main>
  <script>
    let books = [];
    let selectedID = null;
    let saveTimer = null;
    const q = document.querySelector("#q");
    const results = document.querySelector("#results");
    const detail = document.querySelector("#detail");
    const count = document.querySelector("#count");
    const updated = document.querySelector("#updated");

    async function loadBooks() {
      const response = await fetch(`/api/audiobooks?q=${encodeURIComponent(q.value)}`);
      const data = await response.json();
      books = data.items ?? [];
      count.textContent = `${data.count ?? books.length} title${(data.count ?? books.length) === 1 ? "" : "s"}`;
      updated.textContent = data.generatedAt ? new Date(data.generatedAt).toLocaleTimeString([], { hour:"numeric", minute:"2-digit" }) : "";
      renderList();
      if (!selectedID && books.length > 0) openBook(books[0].id);
      if (selectedID && !books.some(book => book.id === selectedID)) {
        selectedID = null;
        detail.innerHTML = `<p>No matching audiobook selected.</p>`;
      }
    }

    function renderList() {
      if (books.length === 0) {
        results.className = "empty";
        results.textContent = "No audiobooks found.";
        return;
      }
      results.className = "";
      results.innerHTML = books.map(book => `
        <button class="book ${book.id === selectedID ? "selected" : ""}" onclick="openBook('${book.id}')">
          <div class="cover">${book.posterURL ? `<img src="${book.posterURL}" alt="">` : "A"}</div>
          <div>
            <h3>${escapeHTML(book.title)}</h3>
            <p>${escapeHTML([book.author, book.series].filter(Boolean).join(" • ") || book.subtitle)}</p>
            <div class="meta"><span>${formatTime(book.durationSeconds)}</span><span>${book.chapterCount} chapters</span>${book.playback?.currentChapterTitle ? `<span>${escapeHTML(book.playback.currentChapterTitle)}</span>` : ""}</div>
            <progress max="1" value="${book.playback?.percent ?? 0}"></progress>
          </div>
        </button>`).join("");
    }

    async function openBook(id) {
      selectedID = id;
      renderList();
      detail.innerHTML = `<p>Loading...</p>`;
      const response = await fetch(`/api/audiobooks/${id}`);
      if (!response.ok) {
        detail.innerHTML = `<p>Audiobook unavailable.</p>`;
        return;
      }
      const data = await response.json();
      renderDetail(data.item, data.chapters ?? []);
    }

    function renderDetail(book, chapters) {
      const playback = book.playback ?? { seconds:0, duration:book.durationSeconds, percent:0 };
      detail.innerHTML = `
        <div class="hero">
          <div class="cover">${book.posterURL ? `<img src="${book.posterURL}" alt="">` : "A"}</div>
          <div>
            <span class="pill">Audiobook</span>
            <h1>${escapeHTML(book.title)}</h1>
            <p>${escapeHTML([book.author, book.series, book.narrator ? `Narrated by ${book.narrator}` : null].filter(Boolean).join(" • "))}</p>
            <audio id="player" controls src="/stream/${book.id}"></audio>
            <progress id="detailProgress" max="1" value="${playback.percent ?? 0}"></progress>
            <p id="nowPlaying">${escapeHTML(playback.currentChapterTitle ?? "Chapter 1")} • ${formatTime(playback.seconds ?? 0)} / ${formatTime(playback.duration ?? book.durationSeconds)}</p>
          </div>
        </div>
        <p>${escapeHTML(book.summary)}</p>
        <div class="chapters">
          ${chapters.map(chapter => renderChapter(book.id, chapter, playback)).join("")}
        </div>`;
      const player = document.querySelector("#player");
      player.addEventListener("loadedmetadata", () => {
        const resume = playback.seconds ?? 0;
        if (resume > 5 && resume < Math.max(player.duration - 8, 0)) player.currentTime = resume;
      });
      player.addEventListener("timeupdate", () => updatePlayback(book.id, chapters, player));
    }

    function renderChapter(id, chapter, playback) {
      const isCurrent = playback.currentChapterIndex === chapter.index;
      return `<div class="chapter ${isCurrent ? "current" : ""}" data-chapter="${chapter.index}">
        <span class="time">${formatTime(chapter.startSeconds)}</span>
        <div><strong>${escapeHTML(chapter.title)}</strong><p>${formatTime(chapter.endSeconds ? chapter.endSeconds - chapter.startSeconds : 0)}</p></div>
        <button onclick="jumpTo(${chapter.startSeconds})">Play</button>
      </div>`;
    }

    function updatePlayback(id, chapters, player) {
      const duration = player.duration || 0;
      const seconds = player.currentTime || 0;
      const progress = document.querySelector("#detailProgress");
      const nowPlaying = document.querySelector("#nowPlaying");
      const current = currentChapter(chapters, seconds, duration);
      if (progress && duration > 0) progress.value = Math.min(Math.max(seconds / duration, 0), 1);
      if (nowPlaying) nowPlaying.textContent = `${current?.title ?? "Chapter"} • ${formatTime(seconds)} / ${formatTime(duration)}`;
      document.querySelectorAll(".chapter").forEach(row => row.classList.toggle("current", Number(row.dataset.chapter) === current?.index));
      if (duration > 0) {
        clearTimeout(saveTimer);
        saveTimer = setTimeout(() => saveProgress(id, seconds, duration), 500);
      }
    }

    function currentChapter(chapters, seconds, duration) {
      return [...chapters].reverse().find(chapter => seconds >= chapter.startSeconds && seconds < (chapter.endSeconds ?? duration)) ?? chapters[chapters.length - 1];
    }

    function jumpTo(seconds) {
      const player = document.querySelector("#player");
      if (!player) return;
      player.currentTime = seconds;
      player.play();
    }

    async function saveProgress(id, seconds, duration) {
      await fetch(`/api/progress/${id}`, { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({ seconds, duration }) });
    }

    function formatTime(value) {
      const total = Math.max(0, Math.floor(Number(value) || 0));
      const hours = Math.floor(total / 3600);
      const minutes = Math.floor((total % 3600) / 60);
      const seconds = total % 60;
      return hours > 0 ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}` : `${minutes}:${String(seconds).padStart(2, "0")}`;
    }

    function escapeHTML(value) {
      return String(value ?? "").replace(/[&<>"']/g, c => ({ "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" }[c]));
    }

    let debounce = null;
    q.addEventListener("input", () => {
      clearTimeout(debounce);
      debounce = setTimeout(loadBooks, 180);
    });
    loadBooks();
  </script>
</body>
</html>
"""
}
