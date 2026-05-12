import { ConnectWifiNetwork, ScanWifiNetworks, SetLauncherKeyboardPaused } from "/wailsjs/go/main/App.js";

let overlay = null;
let list = null;
let status = null;
let form = null;
let selectedLabel = null;
let passwordInput = null;
let connectButton = null;
let refreshButton = null;
let closeButton = null;
let onClose = null;
let selected = null;
let focusIndex = 0;

function setStatus(message, tone = "") {
  if (!status) return;
  status.textContent = message || "";
  status.dataset.tone = tone;
}

function errorMessage(err) {
  return String(err?.message || err || "Unknown error");
}

function isOpen() {
  return !!overlay && !overlay.hidden;
}

function getFocusables() {
  if (!isOpen()) return [];
  return [
    refreshButton,
    closeButton,
    ...list.querySelectorAll(".wifi-network"),
    ...(!form.hidden ? [passwordInput, connectButton] : []),
  ].filter((el) => el && !el.hidden && !el.disabled);
}

function syncFocus(index = focusIndex) {
  const items = getFocusables();
  items.forEach((el) => el.classList.remove("gp-focus"));
  if (!items.length) return;

  focusIndex = Math.max(0, Math.min(index, items.length - 1));
  const el = items[focusIndex];
  el.classList.add("gp-focus");
  el.focus({ preventScroll: true });
  el.scrollIntoView?.({ behavior: "smooth", block: "nearest", inline: "nearest" });
}

function normalizeNetwork(raw) {
  return {
    ssid: raw.ssid || raw.SSID || "",
    authentication: raw.authentication || raw.Authentication || "",
    encryption: raw.encryption || raw.Encryption || "",
    signal: Number(raw.signal ?? raw.Signal ?? 0),
    secure: !!(raw.secure ?? raw.Secure),
    known: !!(raw.known ?? raw.Known),
  };
}

function renderNetworks(networks) {
  if (!list) return;
  list.replaceChildren();

  if (!networks.length) {
    const empty = document.createElement("div");
    empty.className = "wifi-empty";
    empty.textContent = "No networks found";
    list.append(empty);
    return;
  }

  networks
    .sort((a, b) => b.signal - a.signal || a.ssid.localeCompare(b.ssid))
    .forEach((network) => {
      const item = document.createElement("button");
      item.className = "wifi-network";
      item.type = "button";

      const name = document.createElement("span");
      name.className = "wifi-name";
      name.textContent = network.ssid;

      const meta = document.createElement("span");
      meta.className = "wifi-meta";
      meta.textContent = `${network.signal || 0}% ${network.known ? "Saved" : network.secure ? "Secured" : "Open"}`;

      item.append(name, meta);
      item.addEventListener("click", () => selectNetwork(network));
      list.append(item);
    });
}

function selectNetwork(network) {
  selected = network;
  if (!form || !selectedLabel || !passwordInput || !connectButton) return;

  selectedLabel.textContent = network.ssid;
  passwordInput.value = "";
  passwordInput.hidden = !network.secure || network.known;
  passwordInput.required = network.secure && !network.known;
  form.hidden = false;
  connectButton.disabled = false;

  syncFocus(getFocusables().indexOf(network.secure && !network.known ? passwordInput : connectButton));
}

async function scan() {
  if (!overlay || overlay.hidden) return;
  selected = null;
  if (form) form.hidden = true;
  if (refreshButton) refreshButton.disabled = true;
  setStatus("Scanning...");
  let desiredFocus = 0;

  try {
    const networks = (await ScanWifiNetworks()).map(normalizeNetwork).filter((n) => n.ssid);
    renderNetworks(networks);
    setStatus(`${networks.length} networks`);
    desiredFocus = networks.length ? 2 : 0;
  } catch (err) {
    console.error(err);
    renderNetworks([]);
    setStatus(`Scan failed: ${errorMessage(err)}`, "error");
  } finally {
    if (refreshButton) refreshButton.disabled = false;
    syncFocus(desiredFocus);
  }
}

async function connectSelected(e) {
  e.preventDefault();
  if (!selected || !passwordInput || !connectButton) return;

  connectButton.disabled = true;
  setStatus(`Connecting to ${selected.ssid}...`);
  try {
    await ConnectWifiNetwork(selected.ssid, selected.secure && !selected.known ? passwordInput.value : "");
    setStatus(`Connected to ${selected.ssid}`, "ok");
  } catch (err) {
    console.error(err);
    setStatus(`Connection failed: ${errorMessage(err)}`, "error");
  } finally {
    connectButton.disabled = false;
    syncFocus(getFocusables().indexOf(connectButton));
  }
}

export async function openWifiDialog() {
  if (!overlay) return;
  overlay.hidden = false;
  document.body.classList.add("wifi-open");
  focusIndex = 0;
  await SetLauncherKeyboardPaused(true).catch(console.error);
  syncFocus(0);
  scan();
}

export async function closeWifiDialog() {
  if (!overlay) return;
  overlay.hidden = true;
  document.body.classList.remove("wifi-open");
  selected = null;
  if (form) form.hidden = true;
  await SetLauncherKeyboardPaused(false).catch(console.error);
  onClose?.();
}

export function handleWifiControllerNav(dir) {
  if (!isOpen()) return false;
  const items = getFocusables();
  if (!items.length) return true;

  if (dir === "up" || dir === "left") syncFocus(focusIndex - 1);
  else if (dir === "down" || dir === "right") syncFocus(focusIndex + 1);
  return true;
}

export function handleWifiControllerPress(name) {
  if (!isOpen()) return false;

  if (name === "B") {
    closeWifiDialog();
    return true;
  }
  if (name === "START") {
    scan();
    return true;
  }
  if (name === "A") {
    const item = getFocusables()[focusIndex];
    item?.click();
    return true;
  }
  return true;
}

export function initWifiDialog(opts = {}) {
  onClose = opts.onClose || null;
  overlay = document.getElementById("wifi-overlay");
  list = document.getElementById("wifi-list");
  status = document.getElementById("wifi-status");
  form = document.getElementById("wifi-connect-form");
  selectedLabel = document.getElementById("wifi-selected");
  passwordInput = document.getElementById("wifi-password");
  connectButton = document.getElementById("btn-wifi-connect");
  refreshButton = document.getElementById("btn-wifi-refresh");
  closeButton = document.getElementById("btn-wifi-close");
  if (!overlay || !list || !status || !form) return;

  refreshButton?.addEventListener("click", scan);
  closeButton?.addEventListener("click", closeWifiDialog);
  form.addEventListener("submit", connectSelected);
  [refreshButton, closeButton, passwordInput, connectButton].forEach((el) => {
    el?.addEventListener("focus", () => {
      const i = getFocusables().indexOf(el);
      if (i >= 0) syncFocus(i);
    });
  });
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) closeWifiDialog();
  });
  window.addEventListener("keydown", (e) => {
    if (overlay.hidden || e.key !== "Escape") return;
    e.preventDefault();
    closeWifiDialog();
  });
}
