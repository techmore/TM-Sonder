// Shared front-end helpers for the TM Sonder web pages (library, audiobooks,
// ebooks). Loaded before each page's own <script>; exposes window.Sonder.
(function () {
  "use strict";

  // When a page is opened with ?token= (LAN pairing), propagate it to every
  // same-origin request the page makes.
  const TOKEN = new URLSearchParams(location.search).get("token");

  function api(path) {
    if (!TOKEN) return path;
    return path + (path.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(TOKEN);
  }

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>"']/g, c => (
      { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]
    ));
  }

  function formatTime(value) {
    const total = Math.max(0, Math.floor(Number(value) || 0));
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = total % 60;
    return h > 0
      ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
      : `${m}:${String(s).padStart(2, "0")}`;
  }

  window.Sonder = { TOKEN, api, escapeHTML, formatTime };
})();
