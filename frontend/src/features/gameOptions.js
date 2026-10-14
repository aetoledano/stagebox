import { GetGameOptions, SetGameVirtualInput } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";

let overlay, title, toggle, stateLabel, status, close;
let settings = null, generation = 0, busy = false, index = 0;
let onClose = () => {};
export const gameOptionsOpen = () => !!overlay && !overlay.hidden;

function items() { return [toggle, close].filter((item) => !item.disabled); }
function focus(at = index) {
  if (!gameOptionsOpen()) return;
  const controls = items();
  overlay.querySelectorAll(".gp-focus").forEach((item) => item.classList.remove("gp-focus"));
  if (!controls.length) return;
  index = ((at % controls.length) + controls.length) % controls.length;
  controls[index].classList.add("gp-focus");
  controls[index].focus({ preventScroll: true });
}
function render() {
  const enabled = !!settings?.enableVirtualInput;
  toggle.setAttribute("aria-checked", String(enabled));
  stateLabel.textContent = enabled ? "On" : "Off";
  toggle.disabled = busy || !settings || (!enabled && !settings.virtualInputAvailable);
  close.disabled = busy;
  if (settings) status.textContent = settings.virtualInputMessage || "";
}
export function closeGameOptions() {
  if (!gameOptionsOpen() || busy) return;
  generation++;
  overlay.hidden = true;
  settings = null;
  onClose();
}
export async function openGameOptions(card) {
  if (!overlay || gameOptionsOpen() || !card || card.dataset.steamAppId !== "0") return;
  const current = ++generation;
  settings = null; busy = false; index = 0;
  title.textContent = card.dataset.name || "Game options";
  status.textContent = "Loading game settings…";
  status.dataset.tone = "";
  overlay.hidden = false;
  render(); focus();
  try {
    const result = await GetGameOptions(card.dataset.id);
    if (current !== generation || !gameOptionsOpen()) return;
    settings = result;
    render(); focus(0);
  } catch (error) {
    if (current !== generation || !gameOptionsOpen()) return;
    status.textContent = String(error?.message || error);
    status.dataset.tone = "error";
    focus(0);
  }
}
async function toggleVirtualInput() {
  if (!settings || busy) return;
  const current = generation;
  busy = true; render();
  status.textContent = "Saving…";
  try {
    const result = await SetGameVirtualInput(settings.id, !settings.enableVirtualInput);
    if (generation !== current) return;
    settings = result;
    status.dataset.tone = "";
  } catch (error) {
    if (generation !== current) return;
    status.textContent = String(error?.message || error);
    status.dataset.tone = "error";
  } finally {
    if (generation === current) {
      const errorText = status.dataset.tone === "error" ? status.textContent : "";
      busy = false; render();
      if (errorText) status.textContent = errorText;
      focus(0);
    }
  }
}
export function handleGameOptionsNav(direction) {
  if (!gameOptionsOpen()) return false;
  if (!busy) focus(index + (["up", "left"].includes(direction) ? -1 : 1));
  return true;
}
export function handleGameOptionsPress(name) {
  if (!gameOptionsOpen() || name === "KILL") return false;
  if (name === "B") closeGameOptions();
  else if (name === "A" && !busy) items()[index]?.click();
  return true;
}
export function initGameOptions(closeCallback) {
  onClose = closeCallback || onClose;
  overlay = document.createElement("div");
  overlay.className = "game-options-overlay";
  overlay.hidden = true;
  overlay.innerHTML = `<section class="game-options-panel" role="dialog" aria-modal="true" aria-labelledby="game-options-title">
    <div class="game-options-head"><span>GAME OPTIONS</span><h2 id="game-options-title"></h2></div>
    <button class="game-options-toggle" role="switch" aria-checked="false">
      <span><strong>Virtual Xbox input for this game</strong><small>Also convert controllers saved as Native. Controllers saved as Virtual already use Xbox input.</small></span>
      <span class="game-options-state">Off</span>
    </button>
    <p class="game-options-status" role="status"></p>
    <button class="game-options-close">Back</button>
    <div class="game-options-hint">A · select &nbsp; B · back</div>
  </section>`;
  (document.getElementById("stagebox-scale-root") || document.body).append(overlay);
  title = overlay.querySelector("h2"); toggle = overlay.querySelector(".game-options-toggle");
  stateLabel = overlay.querySelector(".game-options-state"); status = overlay.querySelector(".game-options-status");
  close = overlay.querySelector(".game-options-close");
  toggle.addEventListener("click", toggleVirtualInput);
  close.addEventListener("click", closeGameOptions);
  overlay.addEventListener("click", (event) => { if (event.target === overlay) closeGameOptions(); });
  window.addEventListener("keydown", (event) => {
    if (!gameOptionsOpen()) return;
    if (event.key === "Tab") { event.preventDefault(); focus(index + (event.shiftKey ? -1 : 1)); }
    else if (event.key === "Escape") { event.preventDefault(); closeGameOptions(); }
    else if (event.key.startsWith("Arrow")) { event.preventDefault(); handleGameOptionsNav(event.key.slice(5).toLowerCase()); }
  });
  EventsOn("game:launch-begin", () => { busy = false; closeGameOptions(); });
}
