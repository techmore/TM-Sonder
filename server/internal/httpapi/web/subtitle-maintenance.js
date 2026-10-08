/* Subtitle coverage is visible to the server owner in Settings. */
(() => {
  "use strict";
  const dialog = document.querySelector("#settingsDlg");
  if (!dialog || !window.Sonder) return;
  const section = document.createElement("section");
  section.hidden = true;
  section.style.cssText = "padding:0 18px 18px;border-bottom:1px solid var(--line)";
  section.innerHTML = `<h3>Automatic subtitles</h3><p data-summary role="status" aria-live="polite"></p><p data-provider style="color:var(--muted)"></p><div class="actions"><button type="button" data-check>Check subtitle coverage</button><button type="button" data-refresh>Refresh status</button></div><details><summary>Titles still needing subtitles</summary><div data-missing></div></details>`;
  dialog.querySelector(".modal-head")?.after(section);
  let timer = 0, loading = false;
  const { api, escapeHTML } = window.Sonder;
  async function refresh() {
    if (!dialog.open || loading) return;
    loading = true;
    try {
      const response = await fetch(api("/api/settings/subtitles"), { cache: "no-store" });
      if (response.status === 401 || response.status === 403) { section.hidden = true; return; }
      if (!response.ok) throw new Error("Subtitle status is unavailable");
      const state = await response.json();
      section.hidden = false;
      const entries = Object.values(state.items || {}), counts = state.counts || {};
      section.querySelector("[data-summary]").textContent = `${state.running ? "Checking library… · " : ""}${counts.available || 0} subtitle sets available · ${entries.length - (counts.available || 0)} still needed · ${(state.languages || []).join(", ")}`;
      section.querySelector("[data-provider]").textContent = state.providerConfigured
        ? "Automatically extracts embedded text and searches OpenSubtitles. Missing matches and provider limits are retried."
        : "Embedded text extraction is active. OpenSubtitles downloads need an API key configured on the server.";
      const missing = entries.filter(item => item.status !== "available");
      section.querySelector("[data-missing]").innerHTML = missing.slice(0, 100).map(item => `<p><strong>${escapeHTML(item.title)}</strong> · ${escapeHTML(item.language)} · ${escapeHTML(item.status)}<br><span style="color:var(--muted)">${escapeHTML(item.detail || "Queued for checking")}${item.retryAt && !item.retryAt.startsWith("0001") ? ` · retry ${escapeHTML(new Date(item.retryAt).toLocaleString())}` : ""}</span></p>`).join("") || "<p>No missing subtitles in the checked titles.</p>";
      if (missing.length > 100) section.querySelector("[data-missing]").insertAdjacentHTML("beforeend", `<p>${missing.length - 100} more titles are in the background queue.</p>`);
    } catch (error) {
      section.hidden = false;
      section.querySelector("[data-summary]").textContent = error.message;
    } finally { loading = false; }
  }
  section.querySelector("[data-refresh]").addEventListener("click", refresh);
  section.querySelector("[data-check]").addEventListener("click", async event => {
    const button = event.currentTarget;
    button.disabled = true;
    try {
      const response = await fetch(api("/api/settings/subtitles"), { method: "POST" });
      if (!response.ok) throw new Error("Could not schedule subtitle check");
      section.querySelector("[data-summary]").textContent = "Subtitle check scheduled. Existing retry limits still apply.";
    } catch (error) { section.querySelector("[data-summary]").textContent = error.message; }
    finally { button.disabled = false; }
  });
  new MutationObserver(() => {
    clearInterval(timer);
    if (dialog.open) { void refresh(); timer = setInterval(refresh, 10000); }
  }).observe(dialog, { attributes: true, attributeFilter: ["open"] });
})();
