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
  let mode = "dock", pending = "", active = false, drag = null, enteringPiP = false;
  const iPhonePlayer = typeof navigator !== "undefined" && /iPhone|iPod/.test(navigator.userAgent) && typeof video.webkitEnterFullscreen === "function";
  function configureNativePlayer(fullscreenVideo) {
    // iOS owns automatic PiP on app suspension through its native fullscreen
    // player. Omitting playsinline lets the initial play enter that player even
    // when metadata arrives after the Play gesture.
    video.playsInline = !(iPhonePlayer && fullscreenVideo);
    video.controls = !!(iPhonePlayer && fullscreenVideo);
  }
  const toolbar = document.createElement("div");
  toolbar.className = "np-presentation";
  toolbar.hidden = true;
  toolbar.innerHTML = `<span class="np-move" role="button" tabindex="0" aria-label="Move floating player with arrow keys" title="Drag to move · arrow keys also work">⠿ <span>Video player</span></span><button type="button" class="np-resize" aria-label="Resize floating player" title="Drag to resize · arrow keys also work">↘</button><div class="np-view-actions"><button type="button" data-view="fullscreen">Fullscreen</button><button type="button" data-view="floating">Pop-out</button><button type="button" data-view="pip">Picture in picture</button><button type="button" data-view="dock">Dock</button></div><span class="np-view-status" role="status" aria-live="polite"></span>`;
  host.prepend(toolbar);
  const status = toolbar.querySelector(".np-view-status");
  const pipButton = document.querySelector("#npPictureInPicture");
  const nativePiP = () => document.pictureInPictureElement === video || video.webkitPresentationMode === "picture-in-picture";
  const supportsPiP = () => !!((video.requestPictureInPicture && document.pictureInPictureEnabled) || video.webkitSupportsPresentationMode?.("picture-in-picture"));
  function syncPiPButton() {
    const available = supportsPiP(), opened = nativePiP();
    for (const button of [pipButton, toolbar.querySelector('[data-view="pip"]')]) {
      if (!button) continue;
      button.disabled = !active || !video.readyState || !available;
      button.setAttribute("aria-pressed", String(opened));
      button.setAttribute("aria-label", opened ? "Return video from picture in picture" : "Open picture in picture");
      button.title = !available ? "Picture in picture is unavailable in this browser" : !video.readyState ? "Available once video starts" : opened ? "Return video to Sonder" : "Watch video in picture in picture";
    }
    if (pipButton) pipButton.hidden = !active;
  }
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
  // A fixed z-index cannot rise above modal dialogs. Keep the existing media
  // element in the browser top layer without moving it or resetting playback.
  function lowerPlayer() {
    if (!host.hasAttribute("popover")) return;
    try { if (host.matches(":popover-open")) host.hidePopover(); } catch (_) {}
    host.removeAttribute("popover");
  }
  function raisePlayer() {
    if (!active || !["fullscreen", "floating"].includes(mode) || nativeFullscreen()) return;
    if (typeof host.showPopover !== "function") return;
    try {
      host.setAttribute("popover", "manual");
      if (!host.matches(":popover-open")) host.showPopover();
    } catch (_) { lowerPlayer(); }
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
    if (next === "fullscreen" || next === "floating") raisePlayer();
    else lowerPlayer();
    if (next === "floating") clampFloating();
    window.SonderMediaPresentationChanged?.();
  }
  async function fullscreen() {
    layout("fullscreen");
    // The viewport-sized player remains usable if browser fullscreen is denied.
    try {
      if (host.requestFullscreen && document.fullscreenEnabled) {
        lowerPlayer(); // requestFullscreen rejects an already-open popover.
        await host.requestFullscreen();
      }
      else if (host.webkitRequestFullscreen && document.webkitFullscreenEnabled) {
        lowerPlayer(); host.webkitRequestFullscreen();
      }
      else if (video.webkitEnterFullscreen) {
        if (!video.readyState) { pending = "fullscreen"; return; }
        video.webkitEnterFullscreen();
      } else status.textContent = "Full-window player · browser fullscreen unavailable";
    } catch (_) { status.textContent = "Full-window player · tap Fullscreen to hide browser controls"; }
    raisePlayer();
  }
  async function pictureInPicture() {
    if (nativePiP()) {
      exitPiP();
      return;
    }
    if (!video.readyState) { status.textContent = "Tap PiP once video starts"; syncPiPButton(); return; }
    await enterPictureInPicture(false);
  }
  async function enterPictureInPicture(automatic) {
    if (enteringPiP || nativePiP() || !active) return;
    enteringPiP = true;
    try {
      // Keep Safari's request in the original click stack. On suspension Safari
      // may reject or silently ignore it; its native player handles auto PiP.
      if (video.webkitSupportsPresentationMode?.("picture-in-picture")) {
        video.webkitSetPresentationMode("picture-in-picture");
        // The presentation event confirms entry. Never claim PiP opened just
        // because WebKit returned without throwing.
        if (nativePiP()) layout("pip");
      } else if (video.requestPictureInPicture && document.pictureInPictureEnabled) {
        await video.requestPictureInPicture();
        if (active && nativePiP()) { layout("pip"); exitFullscreen(); }
        else if (!active && nativePiP()) exitPiP();
      } else throw new Error("unavailable");
    } catch (_) {
      if (!automatic) status.textContent = "Picture in picture could not open. Tap PiP again while video is playing.";
    } finally { enteringPiP = false; syncPiPButton(); }
  }
  function present(next) {
    if (!active) return;
    document.querySelector("#detail")?.close?.();
    configureNativePlayer(next === "fullscreen");
    if (next !== "fullscreen" && next !== "pip") exitFullscreen();
    if (next !== "pip") exitPiP();
    if (next === "fullscreen") void fullscreen();
    else if (next === "pip") void pictureInPicture();
    else layout(next);
  }
  toolbar.addEventListener("click", event => {
    const button = event.target.closest("[data-view]");
    if (button) present(button.dataset.view);
  });
  pipButton?.addEventListener("click", () => { if (active) void pictureInPicture(); });
  video.addEventListener("loadedmetadata", () => {
    if (!active) return;
    syncPiPButton();
    if (pending === "fullscreen") void fullscreen();
  });
  video.addEventListener("canplay", syncPiPButton);
  video.addEventListener("emptied", syncPiPButton);
  video.addEventListener("enterpictureinpicture", () => { if (active) layout("pip"); syncPiPButton(); });
  video.addEventListener("leavepictureinpicture", () => { if (active && mode === "pip") layout("floating"); syncPiPButton(); });
  video.addEventListener("webkitpresentationmodechanged", () => {
    if (active && nativePiP()) layout("pip");
    else if (active && mode === "pip") layout("floating");
    syncPiPButton();
  });
  video.addEventListener("webkitbeginfullscreen", () => window.SonderMediaPresentationChanged?.());
  function fullscreenChanged() {
    // iOS can leave native fullscreen during loading, rotation, or app resume.
    // Keep the movie visible; only an explicit Dock action should hide it.
    if (active && mode === "fullscreen" && !nativeFullscreen()) layout("fullscreen");
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
    event.preventDefault(); event.stopPropagation();
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
    event.preventDefault(); event.stopPropagation(); const rect = host.getBoundingClientRect(), delta = event.shiftKey ? 40 : 10;
    sizePlayer(rect.width + (event.key === "ArrowLeft" ? -delta : event.key === "ArrowRight" ? delta : 0), rect.height + (event.key === "ArrowUp" ? -delta : event.key === "ArrowDown" ? delta : 0));
  });
  window.addEventListener("resize", clampFloating);
  window.addEventListener("pageshow", raisePlayer);
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") raisePlayer();
    else if (document.visibilityState === "hidden" && active && !video.paused && !video.ended && video.readyState >= 2 && supportsPiP() && !nativePiP()) {
      // Best effort on browsers permitting background entry. Do not toggle an
      // existing PiP window, resume paused media, or disturb playback on denial.
      void enterPictureInPicture(true);
    }
  });
  if (window.ResizeObserver) new ResizeObserver(clampFloating).observe(host);
  window.SonderVideoPresentation = {
    start(mediaMode) {
      active = mediaMode === "video";
      toolbar.hidden = !active;
      configureNativePlayer(active && preference === "fullscreen");
      if (active && preference === "pip") { layout("floating"); status.textContent = "Tap PiP once video starts"; }
      else if (active) present(preference);
      else { exitFullscreen(); exitPiP(); layout("dock"); host.classList.add("np-audio-mode"); }
      syncPiPButton();
    },
    stop() {
      active = false; pending = ""; toolbar.hidden = true;
      configureNativePlayer(false);
      exitFullscreen(); exitPiP(); layout("dock");
      syncPiPButton();
    },
    isFullscreen() { return active && !nativePiP() && (mode === "fullscreen" || !!nativeFullscreen()); },
    present,
  };
})();
