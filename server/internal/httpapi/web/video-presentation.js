/* Change presentation without replacing the media element or restarting its stream. */
(() => {
  "use strict";
  const host = document.querySelector("#nowPlaying");
  const video = document.querySelector("#npMedia");
  if (!host || !video) return;
  const key = "sonder.videoPresentation.v1";
  const choices = ["fullscreen", "floating", "pip", "dock"];
  let preference = "fullscreen";
  try { const value = localStorage.getItem(key); if (choices.includes(value)) preference = value; } catch (_) {}
  let mode = "dock", pending = "", active = false, drag = null;
  const toolbar = document.createElement("div");
  toolbar.className = "np-presentation";
  toolbar.hidden = true;
  toolbar.innerHTML = `<span class="np-move" role="button" tabindex="0" aria-label="Move floating player with arrow keys" title="Drag to move · arrow keys also work">⠿ <span>Video player</span></span><button type="button" class="np-resize" aria-label="Resize floating player" title="Drag to resize · arrow keys also work">↘</button><div class="np-view-actions"><button type="button" data-view="fullscreen">Fullscreen</button><button type="button" data-view="floating">Pop-out</button><button type="button" data-view="pip">Picture in picture</button><button type="button" data-view="dock">Dock</button></div><span class="np-view-status" role="status" aria-live="polite"></span>`;
  host.prepend(toolbar);
  const status = toolbar.querySelector(".np-view-status");
  const select = document.querySelector("#videoPresentationSel");
  if (select) {
    select.value = preference;
    select.addEventListener("change", () => {
      preference = choices.includes(select.value) ? select.value : "fullscreen";
      try { localStorage.setItem(key, preference); } catch (_) {}
    });
  }
  const nativeFullscreen = () => document.fullscreenElement || document.webkitFullscreenElement || video.webkitDisplayingFullscreen;
  function exitFullscreen() {
    try {
      if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
      else if (document.webkitFullscreenElement) document.webkitExitFullscreen?.();
      else if (video.webkitDisplayingFullscreen) video.webkitExitFullscreen?.();
    } catch (_) {}
  }
  function exitPiP() {
    try {
      if (document.pictureInPictureElement === video) document.exitPictureInPicture().catch(() => {});
      else if (video.webkitPresentationMode === "picture-in-picture") video.webkitSetPresentationMode("inline");
    } catch (_) {}
  }
  function clampFloating() {
    if (mode !== "floating") return;
    const rect = host.getBoundingClientRect();
    host.style.left = Math.max(0, Math.min(rect.left, innerWidth - Math.min(rect.width, innerWidth))) + "px";
    host.style.top = Math.max(0, Math.min(rect.top, innerHeight - Math.min(rect.height, innerHeight))) + "px";
  }
  function layout(next) {
    mode = next;
    pending = "";
    status.textContent = "";
    window.SonderPlayerExpanded?.(false);
    host.classList.toggle("np-immersive", next === "fullscreen");
    host.classList.toggle("np-floating", next === "floating");
    host.classList.toggle("np-video-docked", next === "dock" || next === "pip");
    if (active) host.classList.remove("np-audio-mode");
    document.body.classList.toggle("np-video-immersive", next === "fullscreen");
    if (next !== "floating") {
      for (const property of ["left", "top", "width", "height"]) host.style.removeProperty(property);
    }
    toolbar.querySelectorAll("[data-view]").forEach(button => button.setAttribute("aria-pressed", String(button.dataset.view === next)));
    toolbar.querySelector(".np-move").setAttribute("aria-disabled", String(next !== "floating"));
    if (next === "floating") clampFloating();
  }
  async function fullscreen() {
    layout("fullscreen");
    // The viewport-sized player remains usable if browser fullscreen is denied.
    try {
      if (host.requestFullscreen && document.fullscreenEnabled) await host.requestFullscreen();
      else if (host.webkitRequestFullscreen && document.webkitFullscreenEnabled) host.webkitRequestFullscreen();
      else if (video.webkitEnterFullscreen) {
        if (!video.readyState) { pending = "fullscreen"; return; }
        video.webkitEnterFullscreen();
      } else status.textContent = "Full-window player · browser fullscreen unavailable";
    } catch (_) { status.textContent = "Full-window player · tap Fullscreen to hide browser controls"; }
  }
  async function pictureInPicture() {
    layout("pip");
    if (!video.readyState) { pending = "pip"; status.textContent = "Picture in picture opens when video is ready"; return; }
    try {
      if (video.requestPictureInPicture && document.pictureInPictureEnabled) await video.requestPictureInPicture();
      else if (video.webkitSupportsPresentationMode?.("picture-in-picture")) video.webkitSetPresentationMode("picture-in-picture");
      else throw new Error("unavailable");
    } catch (_) { layout("floating"); status.textContent = "Using a floating player · native picture in picture unavailable"; }
  }
  function present(next) {
    if (!active) return;
    document.querySelector("#detail")?.close?.();
    if (next !== "fullscreen") exitFullscreen();
    if (next !== "pip") exitPiP();
    if (next === "fullscreen") void fullscreen();
    else if (next === "pip") void pictureInPicture();
    else layout(next);
  }
  toolbar.addEventListener("click", event => {
    const button = event.target.closest("[data-view]");
    if (button) present(button.dataset.view);
  });
  video.addEventListener("loadedmetadata", () => {
    if (!active) return;
    if (pending === "fullscreen") void fullscreen();
    else if (pending === "pip") void pictureInPicture();
  });
  video.addEventListener("leavepictureinpicture", () => { if (active && mode === "pip") layout("dock"); });
  video.addEventListener("webkitpresentationmodechanged", () => {
    if (active && mode === "pip" && video.webkitPresentationMode === "inline") layout("dock");
  });
  function fullscreenChanged() {
    if (active && mode === "fullscreen" && !nativeFullscreen()) layout("dock");
  }
  document.addEventListener("fullscreenchange", fullscreenChanged);
  document.addEventListener("webkitfullscreenchange", fullscreenChanged);
  video.addEventListener("webkitendfullscreen", fullscreenChanged);
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && active && (mode === "fullscreen" || mode === "floating")) {
      exitFullscreen(); layout("dock");
    }
  });
  const handle = toolbar.querySelector(".np-move");
  handle.addEventListener("pointerdown", event => {
    if (mode !== "floating" || event.button !== 0) return;
    event.preventDefault();
    const rect = host.getBoundingClientRect();
    drag = { x: event.clientX, y: event.clientY, left: rect.left, top: rect.top };
    handle.setPointerCapture(event.pointerId);
  });
  handle.addEventListener("pointermove", event => {
    if (!drag) return;
    host.style.left = drag.left + event.clientX - drag.x + "px";
    host.style.top = drag.top + event.clientY - drag.y + "px";
    clampFloating();
  });
  handle.addEventListener("pointerup", () => { drag = null; });
  handle.addEventListener("pointercancel", () => { drag = null; });
  handle.addEventListener("keydown", event => {
    if (mode !== "floating" || !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) return;
    event.preventDefault();
    const rect = host.getBoundingClientRect(), delta = event.shiftKey ? 40 : 10;
    host.style.left = rect.left + (event.key === "ArrowLeft" ? -delta : event.key === "ArrowRight" ? delta : 0) + "px";
    host.style.top = rect.top + (event.key === "ArrowUp" ? -delta : event.key === "ArrowDown" ? delta : 0) + "px";
    clampFloating();
  });
  const resizer = toolbar.querySelector(".np-resize");
  let sizing;
  function sizePlayer(width, height) {
    const rect = host.getBoundingClientRect();
    host.style.width = Math.max(Math.min(320, innerWidth), Math.min(width, innerWidth - rect.left)) + "px";
    host.style.height = Math.max(Math.min(240, innerHeight), Math.min(height, innerHeight - rect.top)) + "px";
    clampFloating();
  }
  resizer.addEventListener("pointerdown", event => {
    if (mode !== "floating" || event.button !== 0) return;
    event.preventDefault(); const rect = host.getBoundingClientRect();
    sizing = { x:event.clientX, y:event.clientY, width:rect.width, height:rect.height }; resizer.setPointerCapture(event.pointerId);
  });
  resizer.addEventListener("pointermove", event => { if (sizing) sizePlayer(sizing.width + event.clientX - sizing.x, sizing.height + event.clientY - sizing.y); });
  for (const event of ["pointerup", "pointercancel"]) resizer.addEventListener(event, () => { sizing = null; });
  resizer.addEventListener("keydown", event => {
    if (mode !== "floating" || !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)) return;
    event.preventDefault(); const rect = host.getBoundingClientRect(), delta = event.shiftKey ? 40 : 10;
    sizePlayer(rect.width + (event.key === "ArrowLeft" ? -delta : event.key === "ArrowRight" ? delta : 0), rect.height + (event.key === "ArrowUp" ? -delta : event.key === "ArrowDown" ? delta : 0));
  });
  window.addEventListener("resize", clampFloating);
  if (window.ResizeObserver) new ResizeObserver(clampFloating).observe(host);
  window.SonderVideoPresentation = {
    start(mediaMode) {
      active = mediaMode === "video";
      toolbar.hidden = !active;
      if (active) present(preference);
      else { exitFullscreen(); exitPiP(); layout("dock"); host.classList.add("np-audio-mode"); }
    },
    stop() {
      active = false; pending = ""; toolbar.hidden = true;
      exitFullscreen(); exitPiP(); layout("dock");
    },
    present,
  };
})();
