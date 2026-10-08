/* Full, reusable movie downloads. Browser copies are explicit, bounded, and removable.
 * Native Files downloads are the recommended option for large/offline movies. */
(() => {
  "use strict";
  const LIMIT = 2 * 1024 ** 3;
  const CHUNK = 1024 ** 2;
  const active = new Map();
  const states = new Map();
  const saved = new Map();
  const checking = new Set();
  let databasePromise;
  let openURL = null;
  const endpoint = path => typeof window.Sonder?.api === "function" ? window.Sonder.api(path) : path;
  const size = bytes => `${(Number(bytes || 0) / 1024 ** 2).toFixed(0)} MB`;
  const el = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text) node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  const button = (text, handler) => {
    const node = el("button", text);
    node.type = "button";
    node.addEventListener("click", handler);
    return node;
  };
  const css = el("style");
  css.textContent = `.movie-download-tools{margin-top:20px;padding:16px;border:1px solid var(--line);border-radius:12px;background:var(--panel2);font-size:13px}.movie-download-tools h3{margin:0 0 8px}.movie-download-tools p{color:var(--muted);line-height:1.5;margin:8px 0}.movie-download-tools .actions{display:flex;flex-wrap:wrap;gap:8px}.movie-download-tools a{color:var(--accent)}.movie-download-tools progress{width:100%}.movie-device-player{width:min(900px,95vw);background:var(--panel);color:var(--text);border:1px solid var(--line);border-radius:12px}.movie-device-player video{width:100%;max-height:70vh}.movie-device-player::backdrop{background:#000b}`;
  document.head.append(css);

  async function database() {
    if (!databasePromise) databasePromise = (async () => {
      if (!window.indexedDB) throw new Error("Device storage is not available. Download the MP4 to Files instead.");
      const response = await fetch(endpoint("/api/auth/session"), { cache: "no-store" });
      if (!response.ok) throw new Error("Sign in before saving a device copy.");
      const session = await response.json();
      if (!session.username) throw new Error("Sign in before saving a device copy.");
      return new Promise((resolve, reject) => {
        // Account-scoped stores keep different signed-in users' download lists separate.
        const request = indexedDB.open("sonder-movies-v1-" + encodeURIComponent(session.username), 1);
        request.onupgradeneeded = () => {
          const db = request.result;
          db.createObjectStore("movies", { keyPath: "id" });
          db.createObjectStore("chunks", { keyPath: ["id", "index"] });
        };
        request.onsuccess = () => resolve(request.result);
        request.onerror = () => reject(request.error);
        request.onblocked = () => reject(new Error("Close other Sonder tabs and try again."));
      });
    })().catch(error => { databasePromise = null; throw error; });
    return databasePromise;
  }

  async function transact(stores, mode, action) {
    const db = await database();
    return new Promise((resolve, reject) => {
      const tx = db.transaction(stores, mode);
      let result;
      try { result = action(tx); } catch (error) { tx.abort(); reject(error); return; }
      tx.oncomplete = () => resolve(result?.result);
      tx.onerror = () => reject(tx.error || new Error("Could not access device storage."));
      tx.onabort = () => reject(tx.error || new Error("Device storage operation was interrupted."));
    });
  }

  async function remove(id) {
    await transact(["movies", "chunks"], "readwrite", tx => {
      tx.objectStore("movies").delete(id);
      tx.objectStore("chunks").delete(IDBKeyRange.bound([id, 0], [id, Number.MAX_SAFE_INTEGER]));
    });
    saved.delete(id);
  }

  async function loadSaved() {
    try {
      const records = await transact(["movies"], "readonly", tx => tx.objectStore("movies").getAll());
      for (const record of records || []) {
        if (record.complete) saved.set(record.id, record);
        else if (!active.has(record.id)) await remove(record.id); // Interrupted downloads are never presented as playable.
      }
      refreshAll();
    } catch (_) { /* Storage is optional; preparing and Files downloads remain available. */ }
  }

  async function json(path, method = "GET") {
    const response = await fetch(endpoint(path), { method, cache: "no-store" });
    if (!response.ok) {
      const detail = await response.json().catch(() => null);
      throw new Error(detail?.error || `Movie preparation is unavailable (${response.status}). Try again shortly.`);
    }
    return response.json();
  }

  function preparationChanged(id, state) {
    states.set(id, state);
    document.dispatchEvent(new CustomEvent("sonder:movie-prepared", {
      detail: { id, playlistURL: state.playlistURL || "", status: state.status }
    }));
  }

  async function check(id) {
    if (checking.has(id)) return;
    checking.add(id);
    try {
      preparationChanged(id, await json(`/api/movies/${encodeURIComponent(id)}/preparation`));
    } catch (error) { states.set(id, { status: "unavailable", error: error.message }); }
    finally { checking.delete(id); refreshAll(); }
  }

  async function prepare(id) {
    states.set(id, { status: "preparing" });
    refreshAll();
    try { preparationChanged(id, await json(`/api/movies/${encodeURIComponent(id)}/prepare`, "POST")); }
    catch (error) { states.set(id, { status: "failed", error: error.message }); }
    refreshAll();
  }

  async function keep(id, title, url) {
    if (active.has(id)) return;
    const job = { controller: new AbortController(), bytes: 0, total: 0, message: "Checking device storage…" };
    active.set(id, job);
    if (states.has(id)) states.set(id, { ...states.get(id), deviceError: "" });
    refreshAll();
    let reader;
    try {
      // Opening IDB before a request avoids fetching a large file when storage is unavailable.
      await database();
      if (navigator.storage?.persist) {
        try { job.persistent = await navigator.storage.persist(); } catch (_) { job.persistent = false; }
      }
      const estimate = await navigator.storage?.estimate?.();
      const response = await fetch(endpoint(url), { signal: job.controller.signal, cache: "no-store" });
      if (!response.ok || !response.body) throw new Error("The prepared MP4 could not be downloaded. Prepare the movie again.");
      job.total = Number(response.headers.get("Content-Length")) || 0;
      if (job.total > LIMIT) throw new Error("This movie exceeds the 2 GB browser copy limit. Use Download MP4 to save it to Files instead.");
      const available = estimate?.quota && estimate.quota - (estimate.usage || 0);
      if (Number.isFinite(available) && job.total && job.total * 1.15 > available) throw new Error("There is not enough browser storage. Remove saved movies or download to Files instead.");
      await remove(id);
      await transact(["movies"], "readwrite", tx => tx.objectStore("movies").put({ id, title, complete: false, bytes: 0, created: Date.now() }));
      reader = response.body.getReader();
      let index = 0;
      let pending = [];
      let pendingBytes = 0;
      let lastPaint = 0;
      const flush = async () => {
        if (!pendingBytes) return;
        const blob = new Blob(pending, { type: "video/mp4" });
        await transact(["chunks"], "readwrite", tx => tx.objectStore("chunks").put({ id, index: index++, blob }));
        pending = []; pendingBytes = 0;
      };
      job.message = "Saving device copy… Keep this page open.";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) break;
        if (job.bytes + value.byteLength > LIMIT) throw new Error("This movie exceeds the 2 GB browser copy limit. Download to Files instead.");
        // Aggregate at most 1 MiB, then await its transaction before reading more network data.
        for (let offset = 0; offset < value.byteLength;) {
          job.controller.signal.throwIfAborted();
          const count = Math.min(CHUNK - pendingBytes, value.byteLength - offset);
          pending.push(new Blob([value.subarray(offset, offset + count)]));
          pendingBytes += count; offset += count;
          if (pendingBytes === CHUNK) await flush();
        }
        job.bytes += value.byteLength;
        if (Date.now() - lastPaint > 250) { refreshAll(); lastPaint = Date.now(); }
      }
      await flush();
      job.controller.signal.throwIfAborted();
      if (!job.bytes || (job.total && job.bytes !== job.total)) throw new Error("Download was incomplete. Please try again.");
      const record = { id, title, complete: true, bytes: job.bytes, chunks: index, created: Date.now(), persistent: !!job.persistent };
      await transact(["movies"], "readwrite", tx => tx.objectStore("movies").put(record));
      saved.set(id, record);
      job.message = "Saved on this device.";
    } catch (error) {
      job.controller.abort();
      try { await reader?.cancel(); } catch (_) { /* Abort already closed the response. */ }
      try { await remove(id); } catch (_) { /* A failed transaction is never marked complete. */ }
      const message = error.name === "AbortError" ? "Device download cancelled." : error.name === "QuotaExceededError" ? "Browser storage is full. Remove copies or save the MP4 to Files." : error.message;
      const state = states.get(id) || {};
      states.set(id, { ...state, deviceError: message });
    } finally {
      active.delete(id);
      refreshAll();
    }
  }

  async function playSaved(id) {
    const record = saved.get(id);
    if (!record) return;
    let dialog;
    try {
      const chunks = await transact(["chunks"], "readonly", tx => tx.objectStore("chunks").getAll(IDBKeyRange.bound([id, 0], [id, Number.MAX_SAFE_INTEGER])));
      if (chunks.length !== record.chunks || chunks.reduce((total, chunk) => total + chunk.blob.size, 0) !== record.bytes) {
        await remove(id);
        throw new Error("The browser removed part of this copy. Save it again, or use Files for dependable offline viewing.");
      }
      // Blob parts refer to stored Blob data, rather than reading the full movie into JS ArrayBuffers.
      const blob = new Blob(chunks.map(chunk => chunk.blob), { type: "video/mp4" });
      document.querySelector(".movie-device-player")?.close();
      if (openURL) URL.revokeObjectURL(openURL);
      const playbackURL = URL.createObjectURL(blob);
      openURL = playbackURL;
      dialog = el("dialog", "", "movie-device-player");
      const heading = el("h2", record.title);
      const video = el("video");
      video.controls = true;
      video.playsInline = true;
      video.src = openURL;
      video.preload = "metadata";
      const note = el("p", "Device copy · Press Play. This player works without the network while Sonder is open.");
      const cleanup = () => {
        video.pause(); video.removeAttribute("src"); video.load();
        URL.revokeObjectURL(playbackURL);
        if (openURL === playbackURL) openURL = null;
        dialog.remove();
      };
      dialog.append(heading, video, note, button("Close", () => dialog.close()));
      dialog.addEventListener("close", cleanup, { once: true });
      video.addEventListener("error", () => { note.textContent = "The device cannot play this copy. Download MP4 to Files and use a compatible player."; });
      document.body.append(dialog);
      dialog.showModal();
    } catch (error) {
      states.set(id, { ...(states.get(id) || {}), deviceError: error.message });
      refreshAll();
    }
  }

  function identify(article) {
    const action = article.querySelector('[data-action="play-item"][data-id]');
    if (action) return action.dataset.id;
    const link = article.querySelector('a[href*="/stream/"]');
    return link?.getAttribute("href")?.match(/\/stream\/([^/?]+)/)?.[1];
  }

  function render(section, id, title) {
    const state = states.get(id);
    const job = active.get(id);
    const record = saved.get(id);
    section.replaceChildren(el("h3", "Playback preparation & downloads"));
    const actions = el("div", "", "actions");
    if (state?.status === "ready") {
      section.append(el("p", "Ready: a reusable HLS movie stream and an Apple-compatible MP4. Seeking uses prepared media."));
      const link = el("a", "Download MP4 ↓");
      link.href = endpoint(state.downloadURL || `/stream/${encodeURIComponent(id)}/vod/download.mp4`);
      link.download = title + ".mp4";
      actions.append(link);
      if (!job && !record) actions.append(button("Keep on this device", () => keep(id, title, state.downloadURL || `/stream/${encodeURIComponent(id)}/vod/download.mp4`)));
    } else if (state?.status === "preparing" || state?.status === "queued") {
      section.append(el("p", "Preparing the complete movie on the server. You can leave this page; preparation continues.", "muted"));
      if (Number.isFinite(state.progress)) {
        const progress = el("progress"); progress.max = 100; progress.value = state.progress;
        progress.setAttribute("aria-label", "Movie preparation progress"); section.append(progress);
      }
    } else {
      section.append(el("p", "Prepare this movie once for reliable seeking, repeat playback, and downloads. Originals are retained."));
      actions.append(button(state?.status === "failed" ? "Retry preparation" : "Prepare movie", () => prepare(id)));
      if (state?.error) section.append(el("p", state.error));
    }
    if (job) {
      section.append(el("p", `${job.message} ${size(job.bytes)}${job.total ? " / " + size(job.total) : ""}`, "muted"));
      if (job.total) { const progress = el("progress"); progress.max = job.total; progress.value = job.bytes; progress.setAttribute("aria-label", "Device download progress"); section.append(progress); }
      actions.append(button("Cancel download", () => job.controller.abort()));
    }
    if (record) {
      section.append(el("p", `Device copy: ${size(record.bytes)} · ${record.persistent ? "Persistent storage granted" : "Browser may remove this copy when storage is low"}.`));
      actions.append(button("Play device copy", () => playSaved(id)));
      actions.append(button("Remove device copy", async () => {
        try { await remove(id); refreshAll(); }
        catch (error) { states.set(id, { ...(states.get(id) || {}), deviceError: error.message }); refreshAll(); }
      }));
    }
    if (state?.deviceError) section.append(el("p", state.deviceError));
    section.append(actions, el("p", "For dependable offline viewing, download the MP4 to Files and open it in a compatible player. Device copies have a 2 GB limit, require this page to stay open during download, and do not make Sonder available offline after closing it."));
    const total = [...saved.values()].reduce((sum, movie) => sum + movie.bytes, 0);
    if (total) section.append(el("p", `Sonder device copies: ${saved.size} · ${size(total)}. Remove each copy from its movie page.`));
  }

  function renderDeviceLibrary() {
    const host = document.querySelector("#movieRails");
    if (!host) return;
    let panel = host.querySelector(".movie-device-library");
    if (!saved.size && !active.size) { panel?.remove(); return; }
    const wasOpen = panel?.open;
    if (!panel) { panel = el("details", "", "movie-download-tools movie-device-library"); host.prepend(panel); }
    panel.replaceChildren(el("summary", `On this device · ${saved.size} saved movie${saved.size === 1 ? "" : "s"}`));
    panel.open = !!wasOpen;
    panel.append(el("p", "These copies play without a network connection while this page stays open. Files downloads are recommended when you need to close Sonder and watch offline later."));
    for (const record of saved.values()) {
      const row = el("div", "", "actions");
      row.append(el("span", `${record.title} · ${size(record.bytes)}`), button("Play", () => playSaved(record.id)), button("Remove", async () => {
        try { await remove(record.id); refreshAll(); }
        catch (error) { row.append(el("p", error.message)); }
      }));
      panel.append(row);
    }
  }

  function refreshAll() {
    renderDeviceLibrary();
    document.querySelectorAll(".movie-download-tools[data-movie-id]").forEach(section => render(section, section.dataset.movieId, section.dataset.movieTitle));
  }

  function mount() {
    document.querySelectorAll(".movie-detail-content").forEach(article => {
      if (article.querySelector(".movie-download-tools")) return;
      const id = identify(article);
      if (!id) return;
      const title = article.querySelector("h2")?.textContent || "Movie";
      const section = el("section", "", "movie-download-tools");
      section.dataset.movieId = id; section.dataset.movieTitle = title;
      article.append(section);
      render(section, id, title);
      if (!states.has(id)) check(id);
    });
  }
  // Catalog metadata refreshes replace the detail markup; restore the tools without editing its renderer.
  const observer = new MutationObserver(() => {
    if ([...document.querySelectorAll(".movie-detail-content")].some(article => !article.querySelector(".movie-download-tools"))) mount();
    if (saved.size && document.querySelector("#movieRails") && !document.querySelector("#movieRails .movie-device-library")) renderDeviceLibrary();
  });
  observer.observe(document.body, { childList: true, subtree: true });
  mount();
  loadSaved();
  setInterval(() => {
    document.querySelectorAll(".movie-download-tools[data-movie-id]").forEach(section => {
      const status = states.get(section.dataset.movieId)?.status;
      if (status === "preparing" || status === "queued") check(section.dataset.movieId);
    });
  }, 5000);
  window.addEventListener("pagehide", () => { if (openURL) URL.revokeObjectURL(openURL); });
})();
