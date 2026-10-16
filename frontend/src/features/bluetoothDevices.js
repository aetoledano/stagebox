import { EventsOn } from "/wailsjs/runtime";
import { GetPairedBluetoothDevices, ForgetBluetoothDevice, OpenBluetoothPairing } from "/wailsjs/go/main/App.js";

let overlay, panel, content, status, refresh, pair, footer, heading, subtitle, count;
let devices = [], selected = null, busy = false, removing = false, suspended = false, generation = 0, index = 0;
let returnDevice = "";
let listComplete = true;
let listWarning = "";
let onClose = () => {};
const isOpen = () => !!overlay && !overlay.hidden;
const errorText = (error) => String(error?.message || error);
const buttons = () => [...panel.querySelectorAll("button:not(:disabled)")].filter((button) => !button.closest("[hidden]"));
const btIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m7 7 10 10-5 4V3l5 4L7 17"/></svg>`;
const arrowIcon = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>`;
function message(text, error = false) { status.textContent = text; status.dataset.error = String(error); status.hidden = !text; }
function applyList(result, success = "", successWarning = false) {
  devices = result.devices || [];
  listComplete = !!result.complete;
  listWarning = result.warning || "";
  message(result.warning ? [success,result.warning].filter(Boolean).join(" ") : success, !listComplete || successWarning);
}
function focus(at = index) {
  if (!isOpen()) return;
  const items = buttons();
  panel.querySelectorAll(".gp-focus").forEach((button) => button.classList.remove("gp-focus"));
  if (!items.length) return;
  index = Math.max(0, Math.min(at, items.length - 1));
  const item = items[index];
  item.classList.add("gp-focus"); item.focus({preventScroll:true});
  // Only the list scrolls; toolbar and footer remain in place.
  if (item.dataset.group === "devices") item.scrollIntoView({block:"nearest",inline:"nearest"});
}
function focusKey(key, fallback = 0) {
  const at = buttons().findIndex((button) => button.dataset.focusKey === key);
  focus(at < 0 ? fallback : at);
}
function action(text, handler, key, group, tone = "") {
  const button = document.createElement("button"); button.type = "button"; button.className = `btd-button ${tone}`;
  button.textContent = text; button.dataset.focusKey = key; button.dataset.group = group;
  button.disabled = busy; button.addEventListener("click", handler);
  return button;
}
function deviceCard(device, interactive = false) {
  const card = document.createElement(interactive ? "button" : "article");
  card.className = `btd-device${interactive ? "" : " btd-device-selected"}`;
  if (interactive) {
    card.type = "button"; card.disabled = busy; card.dataset.group = "devices"; card.dataset.focusKey = device.id;
    card.addEventListener("click", () => { selected = device; returnDevice = device.id; render(); message(""); focusKey("cancel"); });
  }
  const icon = document.createElement("span"); icon.className = "btd-device-icon"; icon.innerHTML = btIcon;
  const text = document.createElement("span"); text.className = "btd-device-text";
  const name = document.createElement("strong"); name.textContent = device.name;
  const address = document.createElement("small"); address.textContent = device.transport === "LE" ? `${device.address || "Address unavailable"} · Bluetooth LE` : device.address;
  text.append(name,address);
  const badge = document.createElement("span"); badge.className = `btd-badge${device.connected ? " connected" : ""}`;
  badge.textContent = device.connected ? "Connected" : "Paired";
  card.append(icon,text,badge);
  if (interactive) { const arrow = document.createElement("span"); arrow.className = "btd-chevron"; arrow.innerHTML = arrowIcon; card.append(arrow); }
  return card;
}
function render() {
  content.replaceChildren(); footer.replaceChildren(); refresh.disabled = pair.disabled = busy;
  refresh.hidden = pair.hidden = !!selected; count.hidden = !!selected;
  pair.parentElement.hidden = !!selected;
  heading.textContent = selected ? "Forget device?" : "Bluetooth devices";
  subtitle.textContent = selected ? "" : "Paired with this PC · Classic + LE"; subtitle.hidden = !!selected;
  count.textContent = `${devices.length}`; count.setAttribute("aria-label", `${devices.length} paired devices`);
  if (selected) {
    content.classList.add("confirming"); content.append(deviceCard(selected));
    const description = document.createElement("p"); description.className = "btd-confirm-copy";
    description.textContent = selected.connected ? "Disconnects this device and removes linked controller profiles. Pair again to reconnect." : "Removes linked controller profiles. Pair again to reconnect.";
    content.append(description);
    footer.append(action("Cancel", cancel, "cancel", "confirm"), action("Forget device", forget, "forget", "confirm", "danger"));
    return;
  }
  content.classList.remove("confirming");
  if (!devices.length) {
    const empty = document.createElement("div"); empty.className = "btd-empty";
    const icon = document.createElement("span"); icon.innerHTML = btIcon;
    const title = document.createElement("strong"); title.textContent = busy ? "Loading devices…" : listComplete ? "No paired devices" : "Some devices could not be loaded";
    const hint = document.createElement("p"); hint.textContent = busy ? "" : listComplete ? "Choose Pair device to add one." : "Choose Refresh to try again.";
    empty.append(icon,title,hint); content.append(empty);
  }
  for (const device of devices) content.append(deviceCard(device,true));
  const closeButton = action("Close", () => close(), "close", "footer"); closeButton.disabled = removing;
  footer.append(closeButton);
}
async function load() {
  if (!isOpen() || busy) return;
  const revision = generation, key = buttons()[index]?.dataset.focusKey;
  busy = true; selected = null; render(); message(devices.length ? "Refreshing…" : "");
  try {
    const next = await GetPairedBluetoothDevices();
    if (revision !== generation) return;
    applyList(next);
  } catch (error) { if (revision === generation) { listComplete = false; listWarning = errorText(error); message(listWarning,true); } }
  finally { if (revision === generation) { busy = false; render(); focusKey(key, devices.length ? 2 : 0); } }
}
function cancel() {
  if (busy) return;
  selected = null; render(); message(listWarning,!!listWarning); focusKey(returnDevice,devices.length ? 2 : 0);
}
async function forget() {
  if (busy || !selected) return;
  const device = selected, revision = generation;
  busy = removing = true; render(); message(`Forgetting ${device.name}…`);
  try {
    const warning = await ForgetBluetoothDevice(device.id);
    if (revision !== generation) return;
    selected = null;
    devices = devices.filter((item) => item.id !== device.id);
    const success = [`${device.name} forgotten.`,warning].filter(Boolean).join(" ");
    message(success,!!warning);
    try { const next = await GetPairedBluetoothDevices(); if (revision === generation) applyList(next,success,!!warning); }
    catch (error) { if (revision === generation) message(`Device forgotten. Refresh failed: ${errorText(error)}`,true); }
  } catch (error) { if (revision === generation) message(errorText(error),true); }
  finally { if (revision === generation) { busy = removing = false; render(); focusKey(selected ? "cancel" : "pair"); } }
}
async function startPairing() {
  if (busy) return;
  const revision = generation;
  busy = true; render(); message("Opening pairing…");
  try { await OpenBluetoothPairing(); if (revision === generation) close(true); }
  catch (error) { if (revision === generation) message(errorText(error),true); }
  finally { if (revision === generation) { busy = false; render(); focusKey("pair"); } }
}
export function openBluetoothDevices() {
  if (!overlay || isOpen() || suspended) return;
  generation++; selected = null; busy = removing = false; index = 0; returnDevice = ""; overlay.hidden = false; render(); load();
}
function close(force = false) {
  if (!isOpen() || (removing && !force)) return;
  generation++; overlay.hidden = true; busy = removing = false; selected = null; onClose();
}
function goBack() { if (selected && !busy) cancel(); else close(); }
export function handleBluetoothDevicesNav(direction) {
  if (!isOpen()) return false;
  if (busy) return true;
  const items = buttons(), current = items[index], group = current?.dataset.group;
  const backward = ["up","left"].includes(direction), horizontal = ["left","right"].includes(direction);
  if (selected || horizontal) {
    const peers = items.filter((button) => button.dataset.group === group);
    if (group === "devices" || peers.length < 2) return true;
    const next = (peers.indexOf(current) + (backward ? -1 : 1) + peers.length) % peers.length;
    focus(items.indexOf(peers[next]));
  } else if (group === "devices") {
    const next = items[index + (backward ? -1 : 1)];
    if (next?.dataset.group === "devices") focus(index + (backward ? -1 : 1));
    else focusKey(backward ? "pair" : "close");
  } else if (group === "toolbar") {
    if (backward) focusKey("close");
    else focusKey(devices[0]?.id || "close");
  } else {
    if (backward) focusKey(devices.at(-1)?.id || "pair");
    else focusKey("pair");
  }
  return true;
}
export function handleBluetoothDevicesPress(name) {
  if (!isOpen()) return false;
  if (name === "B") goBack();
  else if (!busy && name === "A") buttons()[index]?.click();
  return true;
}
export function initBluetoothDevices(closeCallback) {
  onClose = closeCallback || onClose;
  overlay = document.createElement("div"); overlay.className = "bluetooth-devices-overlay"; overlay.hidden = true;
  overlay.innerHTML = `<section class="btd-dialog" role="dialog" aria-modal="true" aria-labelledby="bluetooth-devices-heading"><header class="btd-header"><span class="btd-mark">${btIcon}</span><div class="btd-heading"><h2 id="bluetooth-devices-heading">Bluetooth devices</h2><p></p></div><span class="btd-count"></span><div class="btd-toolbar"><button type="button" class="btd-button primary" data-focus-key="pair" data-group="toolbar"><span aria-hidden="true">＋</span> Pair device</button><button type="button" class="btd-button" data-focus-key="refresh" data-group="toolbar">Refresh</button></div></header><div class="btd-content"></div><div class="btd-bottom"><p class="btd-status" role="status" hidden></p><footer class="btd-footer"><span class="btd-scope">Windows Bluetooth</span><div class="btd-footer-actions"></div></footer></div></section>`;
  (document.getElementById("stagebox-scale-root") || document.body).append(overlay);
  panel = overlay.querySelector("section"); content = panel.querySelector(".btd-content"); status = panel.querySelector(".btd-status");
  heading = panel.querySelector("h2"); subtitle = panel.querySelector(".btd-heading p"); count = panel.querySelector(".btd-count");
  refresh = panel.querySelector('[data-focus-key="refresh"]'); pair = panel.querySelector('[data-focus-key="pair"]'); footer = panel.querySelector(".btd-footer-actions");
  refresh.addEventListener("click", load); pair.addEventListener("click", startPairing);
  panel.addEventListener("focusin", (event) => {
    const at = buttons().indexOf(event.target);
    if (at < 0) return;
    index = at; panel.querySelectorAll(".gp-focus").forEach((button) => button.classList.toggle("gp-focus",button === event.target));
  });
  window.addEventListener("keydown", (event) => {
    if (!isOpen()) return;
    if (event.key === "Escape") { event.preventDefault(); goBack(); }
    else if (event.key === "Tab") {
      event.preventDefault(); const length = buttons().length;
      if (length) focus((index + (event.shiftKey ? -1 : 1) + length) % length);
    } else if (event.key.startsWith("Arrow")) { event.preventDefault(); handleBluetoothDevicesNav(event.key.slice(5).toLowerCase()); }
  });
  EventsOn("game:launch-begin", () => { suspended = true; close(true); });
  EventsOn("game:launch-blocked", () => { suspended = false; });
  EventsOn("game:end", () => { suspended = false; });
  EventsOn("pairing-ui:start", () => close(true));
}
