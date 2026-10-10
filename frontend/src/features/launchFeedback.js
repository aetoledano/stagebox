import "../css/launchFeedback.css";
import { EventsEmit, EventsOn } from "/wailsjs/runtime";
import { closeMenu } from "./topbarMenu.js";

let overlay, heading, message, progress, close, artwork, eyebrow, indeterminate;
let errorOpen = false, pendingError = null, activeID = "", ready = false;
let restoreFocus = null;
let getPresentation = () => null, artworkSource = "";
const isOpen = () => overlay && !overlay.hidden;

function setArtwork(id) {
  const url = getPresentation(id)?.thumbnailUrl || "";
  if (url === artworkSource) return;
  artworkSource = url;
  artwork.hidden = !url;
  if (url) artwork.src = url;
  else artwork.removeAttribute("src");
}

function hide() {
  if (!overlay) return;
  overlay.hidden = true;
  errorOpen = false;
}

function showError(payload) {
  closeMenu();
  setArtwork(payload?.id || activeID);
  activeID = "";
  errorOpen = true;
  heading.textContent = payload?.name || "Unable to launch game";
  message.textContent = payload?.error || "Unable to launch the game. Please try again.";
  progress.hidden = true;
  indeterminate.hidden = true;
  eyebrow.textContent = "Launch interrupted";
  overlay.classList.add("launch-feedback-error");
  close.hidden = false;
  overlay.hidden = false;
  close.focus({ preventScroll: true });
}

function dismiss() {
  hide();
  restoreFocus?.();
}

export function handleLaunchFeedbackNav() { return isOpen(); }
export function handleLaunchFeedbackPress(name) {
  if (!isOpen() || name === "KILL") return false;
  if (errorOpen && ["A", "B", "START"].includes(name)) dismiss();
  return true;
}

export function initLaunchFeedback(onDismiss, resolvePresentation = () => null) {
  restoreFocus = onDismiss;
  getPresentation = resolvePresentation;
  overlay = document.createElement("div");
  overlay.className = "launch-feedback";
  overlay.hidden = true;
  overlay.setAttribute("role", "dialog");
  overlay.setAttribute("aria-modal", "true");
  overlay.setAttribute("aria-labelledby", "launch-feedback-heading");
  overlay.setAttribute("aria-describedby", "launch-feedback-message");
  overlay.innerHTML = `<div class="launch-feedback-panel">
    <div class="launch-feedback-scenery" aria-hidden="true">
      <div class="launch-feedback-moon"></div>
      <div class="launch-feedback-ridge launch-feedback-ridge-far"></div>
      <div class="launch-feedback-ridge launch-feedback-ridge-near"></div>
      <div class="launch-feedback-tree launch-feedback-tree-one"></div>
      <div class="launch-feedback-tree launch-feedback-tree-two"></div>
      <div class="launch-feedback-tree launch-feedback-tree-three"></div>
      <img class="launch-feedback-artwork" alt="" decoding="async" hidden>
      <div class="launch-feedback-dust"></div>
    </div>
    <div class="launch-feedback-content">
      <div class="launch-feedback-eyebrow" aria-hidden="true">
        <span class="launch-feedback-dot"></span>
        <span class="launch-feedback-label">Launching game</span>
      </div>
      <h2 id="launch-feedback-heading"></h2>
      <p id="launch-feedback-message" role="status" aria-live="polite"></p>
      <div class="launch-feedback-indeterminate" aria-hidden="true"><span></span></div>
      <progress class="launch-feedback-progress" max="100" aria-label="Game update progress" hidden></progress>
      <button type="button" class="btn gp-focus" hidden>OK</button>
    </div>
    <div class="launch-feedback-edge" aria-hidden="true"></div>
  </div>`;
  (document.getElementById("stagebox-scale-root") || document.body).append(overlay);
  heading = overlay.querySelector("h2");
  message = overlay.querySelector("p");
  progress = overlay.querySelector("progress");
  close = overlay.querySelector("button");
  artwork = overlay.querySelector(".launch-feedback-artwork");
  eyebrow = overlay.querySelector(".launch-feedback-label");
  indeterminate = overlay.querySelector(".launch-feedback-indeterminate");
  // Missing artwork keeps the CSS landscape behind the game title.
  artwork.addEventListener("error", () => { artwork.hidden = true; });
  close.addEventListener("click", dismiss);
  window.addEventListener("keydown", (event) => {
    if (!isOpen()) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (errorOpen && ["Enter", "Escape", " "].includes(event.key)) dismiss();
  }, true);

  EventsOn("game:launch-begin", ({ id }) => {
    activeID = id;
    ready = false;
    pendingError = null;
    hide();
    setArtwork(id);
  });
  EventsOn("game:launch-status", (payload) => {
    if (!payload?.id || payload.id !== activeID) return;
    if (ready) return;
    closeMenu();
    errorOpen = false;
    overlay.classList.remove("launch-feedback-error");
    heading.textContent = payload.name || "Launching game";
    eyebrow.textContent = payload.stage === "updating" ? "Updating game" :
      payload.stage === "steam" ? "Starting Steam" : "Launching game";
    const value = Number(payload.progress);
    const hasProgress = Number.isFinite(value) && value >= 0;
    message.textContent = (payload.message || "Starting game…") + (hasProgress ? ` ${value}%` : "");
    progress.hidden = !hasProgress;
    indeterminate.hidden = hasProgress;
    if (hasProgress) progress.value = Math.min(100, value);
    close.hidden = true;
    overlay.hidden = false;
  });
  EventsOn("game:window-ready", ({ id, ack }) => {
    if (id !== activeID) return;
    ready = true;
    hide();
    // Let WebView paint the cleared panel before Go hides the native window.
    if (ack) requestAnimationFrame(() => requestAnimationFrame(() => EventsEmit(ack)));
  });
  EventsOn("game:launch-blocked", showError);
  EventsOn("game:launch-error", (payload) => { pendingError = payload; });
  EventsOn("game:end", ({ ack } = {}) => {
    activeID = "";
    ready = false;
    if (pendingError) { showError(pendingError); pendingError = null; }
    else if (!errorOpen) hide();
    // A hidden WebView may throttle animation frames. Confirm the DOM state
    // synchronously so Go can restore the window after clearing the panel.
    if (ack) {
      overlay.getBoundingClientRect();
      EventsEmit(ack);
    }
  });
}
