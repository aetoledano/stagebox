import "../css/launchFeedback.css";
import { EventsEmit, EventsOn } from "/wailsjs/runtime";
import { SteamLaunchAction } from "/wailsjs/go/main/App.js";
import { closeMenu } from "./topbarMenu.js";

let overlay, heading, message, progress, close, artwork, eyebrow, indeterminate, actions, hint;
let errorOpen = false, pendingError = null, activeID = "", ready = false;
let restoreFocus = null;
let availableActions = [], selectedAction = 0, actionPending = false, choosingSetup = false;
let steamOpening = false, lastStatus = null;
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
  availableActions = [];
  actionPending = false;
  choosingSetup = false;
  steamOpening = false;
}

function showError(payload) {
  closeMenu();
  setArtwork(payload?.id || activeID);
  activeID = "";
  errorOpen = true;
  availableActions = [];
  actionPending = false;
  choosingSetup = false;
  steamOpening = false;
  heading.textContent = payload?.name || "Unable to launch game";
  message.textContent = payload?.error || "Unable to launch the game. Please try again.";
  progress.hidden = true;
  indeterminate.hidden = true;
  eyebrow.textContent = "Launch interrupted";
  overlay.classList.add("launch-feedback-error");
  close.hidden = false;
  close.disabled = false;
  close.textContent = "OK";
  hint.hidden = true;
  actions.hidden = true;
  overlay.hidden = false;
  close.focus({ preventScroll: true });
}

function dismiss() {
  hide();
  restoreFocus?.();
}

function selectAction(index) {
  selectedAction = (index + availableActions.length) % availableActions.length;
  [...actions.children].forEach((button, i) => button.classList.toggle("gp-focus", i === selectedAction));
  actions.children[selectedAction]?.focus({ preventScroll: true });
}

function renderActions(payload) {
  const next = [];
  if (payload.canOpenSteam === true) next.push({ action: "steam", label: "Open in Steam" });
  if (payload.canUpdateInSteam === true) next.push({ action: "update", label: "Update in Steam" });
  if (payload.canGoBack === true) next.push({ action: "back", label: "Back to launcher" });
  const changed = JSON.stringify(next) !== JSON.stringify(availableActions);
  if (choosingSetup && payload.canGoBack !== true && !steamOpening) actionPending = false;
  choosingSetup = payload.canGoBack === true;
  availableActions = next;
  close.hidden = true;
  actions.hidden = !next.length;
  hint.hidden = !next.length;
  hint.textContent = "Choose an action · A / Enter · B / Esc to return";
  if (changed) {
    selectedAction = 0;
    actions.replaceChildren(...next.map(({ action, label }) => {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = label;
      button.addEventListener("click", () => launchAction(action));
      return button;
    }));
  }
  [...actions.children].forEach((button) => { button.disabled = actionPending; });
  if (changed && next.length) selectAction(0);
}

async function launchAction(action) {
  if (actionPending || !availableActions.some((item) => item.action === action)) return;
  const id = activeID;
  actionPending = true;
  if (["steam", "update"].includes(action)) {
    steamOpening = true;
    renderStatus(lastStatus || { id, name: heading.textContent });
  }
  [...actions.children].forEach((button) => { button.disabled = true; });
  try {
    await SteamLaunchAction(id, action);
  } catch (error) {
    if (id !== activeID || ready) return;
    actionPending = false;
    steamOpening = lastStatus?.stage === "steam-opening";
    if (lastStatus) renderStatus(lastStatus);
    if (!steamOpening) message.textContent = String(error);
  }
}

export function handleLaunchFeedbackNav(dir) {
  if (!isOpen()) return false;
  if (availableActions.length && !actionPending) {
    selectAction(selectedAction + (["left", "up"].includes(dir) ? -1 : 1));
  }
  return true;
}
export function handleLaunchFeedbackPress(name) {
  if (!isOpen()) return false;
  if (errorOpen && ["A", "B", "START"].includes(name)) { dismiss(); return true; }
  if (availableActions.length) {
    if (name === "A") launchAction(availableActions[selectedAction].action);
    if (["B", "KILL"].includes(name) && choosingSetup) launchAction("back");
  }
  if (name === "KILL" && !choosingSetup) return false;
  return true;
}

function renderStatus(payload) {
  if (payload.stage === "steam-opening") steamOpening = true;
  if (steamOpening) payload = { ...payload, stage: "steam-opening", message: "Launching Steam…", progress: -1,
    canOpenSteam: false, canUpdateInSteam: false, canGoBack: false };
  closeMenu();
  errorOpen = false;
  overlay.classList.remove("launch-feedback-error");
  heading.textContent = payload.name || "Launching game";
  eyebrow.textContent = payload.stage === "updating" ? "Updating game" :
    payload.stage === "installer" ? "RetroBat setup" :
    payload.stage === "setup" ? "Steam update" :
    payload.stage === "steam-opening" ? "Launching Steam" :
    payload.stage === "steam" ? "Starting Steam" : "Launching game";
  const value = Number(payload.progress);
  const hasProgress = Number.isFinite(value) && value >= 0;
  message.textContent = (payload.message || "Starting game…") + (hasProgress ? ` ${value}%` : "");
  progress.hidden = !hasProgress;
  indeterminate.hidden = hasProgress || payload.stage === "setup";
  if (hasProgress) progress.value = Math.min(100, value);
  overlay.hidden = false;
  renderActions(payload);
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
      <div class="launch-feedback-actions" hidden></div>
      <p class="launch-feedback-update-hint" hidden></p>
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
  actions = overlay.querySelector(".launch-feedback-actions");
  hint = overlay.querySelector(".launch-feedback-update-hint");
  // Missing artwork keeps the CSS landscape behind the game title.
  artwork.addEventListener("error", () => { artwork.hidden = true; });
  close.addEventListener("click", dismiss);
  window.addEventListener("keydown", (event) => {
    if (!isOpen()) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    if (errorOpen && ["Enter", "Escape", " "].includes(event.key)) dismiss();
    else if (event.key === "Escape" && choosingSetup) launchAction("back");
    else if (["Enter", " "].includes(event.key) && availableActions.length) launchAction(availableActions[selectedAction].action);
    else if (["ArrowUp", "ArrowLeft", "ArrowDown", "ArrowRight"].includes(event.key)) {
      handleLaunchFeedbackNav(["ArrowUp", "ArrowLeft"].includes(event.key) ? "up" : "down");
    }
  }, true);

  EventsOn("game:launch-begin", ({ id }) => {
    activeID = id;
    ready = false;
    pendingError = null;
    lastStatus = null;
    hide();
    setArtwork(id);
  });
  EventsOn("game:launch-status", (payload) => {
    if (!payload?.id || payload.id !== activeID) return;
    if (ready) return;
    lastStatus = payload;
    renderStatus(payload);
  });
  const hideForHandoff = ({ id, ack }) => {
    if (id !== activeID) return;
    const alreadyHidden = ready;
    ready = true;
    hide();
    // Let WebView paint the cleared panel before Go hides the native window.
    if (ack) {
      if (alreadyHidden) EventsEmit(ack);
      else requestAnimationFrame(() => requestAnimationFrame(() => EventsEmit(ack)));
    }
  };
  EventsOn("game:window-ready", hideForHandoff);
  EventsOn("game:steam-ui-ready", hideForHandoff);
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
