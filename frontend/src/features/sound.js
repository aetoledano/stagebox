import { GetAudioStatus, GetSoundSettings, SetSoundOutput, AdjustAudioVolume, SetAudioVolume, ToggleAudioMute, SetSoundSettingsOpen } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";

let overlay, list, statusLine, slider, value, mute, refresh, close;
let onClose = null;
let current = { available: false, volume: 0, muted: false, deviceID: "", deviceName: "" };
let focusIndex = 0, generation = 0, refreshing = false, selecting = false;
let listRevision = 0;
let pollTimer = null, feedbackTimer = null;
let pendingVolume = null, writingVolume = false;
const speaker = "\uE767", mutedSpeaker = "\uE74F";
const message = (err) => String(err?.message || err || "Sound unavailable");
const isOpen = () => overlay && !overlay.hidden;

function setMessage(text, tone = "") {
  statusLine.textContent = text;
  statusLine.dataset.tone = tone;
}

function focusables() {
  return [refresh, close, mute, slider, ...list.querySelectorAll(".sound-device")].filter((el) => !el.disabled);
}

function focusAt(index = focusIndex) {
  if (!isOpen()) return;
  const items = focusables();
  overlay.querySelectorAll(".gp-focus").forEach((el) => el.classList.remove("gp-focus"));
  if (!items.length) return;
  focusIndex = ((index % items.length) + items.length) % items.length;
  const item = items[focusIndex];
  item.classList.add("gp-focus");
  if (document.activeElement !== item) item.focus({ preventScroll: true });
  item.scrollIntoView({ block: "nearest", inline: "nearest" });
}

function updateStatus(raw, feedback = false, error = "") {
  current = raw || { available: false, volume: 0, muted: false };
  const icon = current.muted || !current.available ? mutedSpeaker : speaker;
  const text = current.available ? `${current.volume}%` : "--";
  const indicator = document.getElementById("sound-indicator");
  indicator.classList.toggle("muted", !!current.muted);
  indicator.classList.toggle("unavailable", !current.available);
  indicator.querySelector(".sound-icon").textContent = icon;
  indicator.querySelector(".sound-percent").textContent = text;
  indicator.title = error || (current.available ? `${current.deviceName}: ${current.muted ? "Muted, " : ""}${text}` : "No sound output");
  indicator.setAttribute("aria-label", indicator.title);
  mute.disabled = slider.disabled = !current.available;
  mute.textContent = icon;
  mute.title = current.muted ? "Unmute" : "Mute";
  mute.setAttribute("aria-label", mute.title);
  mute.setAttribute("aria-pressed", String(!!current.muted));
  value.textContent = current.muted ? "Muted" : text;
  if (!writingVolume) slider.value = current.volume || 0;
  list.querySelectorAll(".sound-device").forEach((item) => {
    const selected = current.available && item.dataset.deviceID === current.deviceID;
    item.setAttribute("aria-checked", String(selected));
    item.querySelector(".sound-device-check").textContent = selected ? "\uE73E" : "";
    item.querySelector(".sound-device-current").textContent = selected ? "Current output" : "";
  });
  if (feedback) showFeedback(error);
}

function showFeedback(error = "") {
  const hud = document.getElementById("volume-feedback");
  clearTimeout(feedbackTimer);
  hud.hidden = false;
  hud.classList.toggle("muted", !!current.muted);
  document.getElementById("volume-feedback-icon").textContent = current.muted || !current.available ? mutedSpeaker : speaker;
  document.getElementById("volume-feedback-device").textContent = error || current.deviceName || "No sound output";
  document.getElementById("volume-feedback-value").textContent = !current.available ? "--" : current.muted ? "Muted" : `${current.volume}%`;
  document.getElementById("volume-feedback-bar").value = current.available ? current.volume : 0;
  feedbackTimer = setTimeout(() => { hud.hidden = true; }, 1800);
}

function renderDevices(devices) {
  const focused = document.activeElement;
  const focusedID = focused?.dataset?.deviceID;
  list.replaceChildren();
  for (const device of devices) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "sound-device";
    item.dataset.deviceID = device.id;
    item.setAttribute("role", "radio");
    const check = document.createElement("span");
    check.className = "sound-device-check mdl2";
    check.setAttribute("aria-hidden", "true");
    const name = document.createElement("span");
    name.className = "sound-device-name";
    name.textContent = device.name;
    const label = document.createElement("span");
    label.className = "sound-device-current";
    item.append(check, name, label);
    item.addEventListener("click", () => selectOutput(device.id));
    item.addEventListener("focus", () => focusAt(focusables().indexOf(item)));
    list.append(item);
  }
  if (!devices.length) {
    const empty = document.createElement("div");
    empty.className = "sound-empty";
    empty.textContent = "No sound outputs available";
    list.append(empty);
  }
  updateStatus(current);
  const desired = focusedID
    ? [...list.querySelectorAll(".sound-device")].find((item) => item.dataset.deviceID === focusedID)
    : focused;
  const index = focusables().indexOf(desired);
  if (index >= 0) focusAt(index);
}

async function loadOutputs(initial = false, quiet = false) {
  if (!isOpen() || refreshing || selecting) return;
  const token = generation;
  const revision = ++listRevision;
  refreshing = true;
  refresh.disabled = true;
  if (!quiet) setMessage("Loading sound outputs...");
  try {
    const settings = await GetSoundSettings();
    if (token !== generation || revision !== listRevision || !isOpen()) return;
    updateStatus(settings.status);
    renderDevices(settings.devices || []);
    if (!quiet || statusLine.dataset.tone === "error") setMessage("");
    if (initial) {
      const selected = list.querySelector('[aria-checked="true"]') || list.querySelector(".sound-device");
      focusAt(Math.max(0, focusables().indexOf(selected)));
    }
  } catch (err) {
    if (token === generation && revision === listRevision && isOpen()) setMessage(`Sound unavailable: ${message(err)}`, "error");
  } finally {
    if (token === generation) {
      refreshing = false;
      refresh.disabled = selecting;
      if (isOpen()) focusAt(Math.max(0, focusables().indexOf(document.activeElement)));
    }
  }
}

async function selectOutput(id) {
  if (selecting || id === current.deviceID) return;
  const token = generation;
  listRevision++;
  selecting = true;
  refresh.disabled = true;
  list.querySelectorAll("button").forEach((item) => { item.disabled = true; });
  setMessage("Switching output...");
  try {
    const settings = await SetSoundOutput(id);
    if (token !== generation || !isOpen()) return;
    updateStatus(settings.status);
    renderDevices(settings.devices || []);
    setMessage(`Using ${settings.status.deviceName}`, "ok");
  } catch (err) {
    if (token === generation && isOpen()) setMessage(`Output change failed: ${message(err)}`, "error");
  } finally {
    if (token === generation) {
      selecting = false;
      refresh.disabled = false;
      list.querySelectorAll("button").forEach((item) => { item.disabled = false; });
      const selected = [...list.querySelectorAll("button")].find((item) => item.dataset.deviceID === id);
      if (isOpen()) focusAt(Math.max(0, focusables().indexOf(selected)));
    }
  }
}

async function writeVolume(level) {
  pendingVolume = level;
  if (writingVolume) return;
  writingVolume = true;
  try {
    while (pendingVolume !== null) {
      const next = pendingVolume;
      pendingVolume = null;
      updateStatus(await SetAudioVolume(next));
    }
  } catch (err) {
    pendingVolume = null;
    if (isOpen()) setMessage(message(err), "error");
  } finally {
    writingVolume = false;
    slider.value = current.volume || 0;
  }
}

export function openSoundSettings() {
  if (!overlay || isOpen()) return;
  generation++;
  selecting = refreshing = false;
  overlay.hidden = false;
  document.body.classList.add("sound-open");
  SetSoundSettingsOpen(true).catch(console.error);
  focusAt(0);
  loadOutputs(true);
  pollTimer = setInterval(() => loadOutputs(false, true), 2000);
}

function closeSoundSettings() {
  if (!isOpen()) return;
  generation++;
  overlay.hidden = true;
  document.body.classList.remove("sound-open");
  clearInterval(pollTimer);
  SetSoundSettingsOpen(false).catch(console.error);
  onClose?.();
}

export function handleSoundControllerNav(dir) {
  if (!isOpen()) return false;
  const item = focusables()[focusIndex];
  if (item === slider && (dir === "left" || dir === "right")) {
    AdjustAudioVolume(dir === "right" ? 1 : -1).catch((err) => setMessage(message(err), "error"));
  } else if (dir === "up" || dir === "left") focusAt(focusIndex - 1);
  else if (dir === "down" || dir === "right") focusAt(focusIndex + 1);
  return true;
}

export function handleSoundControllerPress(name) {
  if (!isOpen() || name === "KILL") return false;
  if (name === "B") closeSoundSettings();
  else if (name === "A") focusables()[focusIndex]?.click();
  else if (name === "START") loadOutputs();
  return true;
}

export function initSoundSettings(opts = {}) {
  onClose = opts.onClose || null;
  overlay = document.getElementById("sound-overlay");
  list = document.getElementById("sound-devices");
  statusLine = document.getElementById("sound-status");
  slider = document.getElementById("sound-volume");
  value = document.getElementById("sound-volume-value");
  mute = document.getElementById("btn-sound-mute");
  refresh = document.getElementById("btn-sound-refresh");
  close = document.getElementById("btn-sound-close");
  refresh.addEventListener("click", () => loadOutputs());
  close.addEventListener("click", closeSoundSettings);
  mute.addEventListener("click", () => ToggleAudioMute().catch((err) => setMessage(message(err), "error")));
  slider.addEventListener("input", () => writeVolume(Number(slider.value)));
  [refresh, close, mute, slider].forEach((item) => item.addEventListener("focus", () => focusAt(focusables().indexOf(item))));
  overlay.addEventListener("click", (event) => { if (event.target === overlay) closeSoundSettings(); });
  window.addEventListener("keydown", (event) => {
    if (!isOpen()) return;
    if (event.key === "Tab") {
      event.preventDefault();
      focusAt(focusIndex + (event.shiftKey ? -1 : 1));
    } else if (event.key === "Escape") { event.preventDefault(); closeSoundSettings(); }
    else if (event.target !== slider && event.key.startsWith("Arrow")) {
      event.preventDefault();
      handleSoundControllerNav(event.key.slice(5).toLowerCase());
    }
  });
  EventsOn("sound:update", ({ status, feedback, error }) => {
    updateStatus(status, feedback, error);
    if (error && isOpen()) setMessage(error, "error");
  });
  EventsOn("game:start", () => { closeSoundSettings(); document.getElementById("volume-feedback").hidden = true; });
  GetAudioStatus().then((status) => updateStatus(status)).catch((err) => updateStatus(null, false, message(err)));
}
