import "./css/base.css";
import "./css/cards.css";
import "./css/topbar.css";
import "./css/sound.css";
import { OpenRetroBat, OpenSteam, Shutdown, Restart, ExitLauncher, OpenBluetoothPairing, LoadGames, Launch, SteamUpdateStates, Explorer, GetStatuses, Info, Warn, Error } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";
import { initBTIndicator } from "./features/bluetooth.js";
import { startClock } from "./features/clock.js";
import { controllerFocusGrid, controllerInit, controllerRefresh } from "./features/controller.js";
import { closeMenu, initTopbarMenu } from "./features/topbarMenu.js";
import { initWifiDialog, openWifiDialog } from "./features/wifi.js";
import { initSoundSettings, openSoundSettings } from "./features/sound.js";
import { initUIScale } from "./features/uiScale.js";
import controllerIconURL from "./assets/images/controller-ico-2.png";
import keyboardMouseIconURL from "./assets/images/kbm-ico.png";
import steamIconURL from "./assets/images/steam-ico.svg";
import updateIconURL from "./assets/images/update-ico.svg";
import { initLaunchFeedback } from "./features/launchFeedback.js";

// build topbar once (or keep your existing HTML)
document.querySelector("#btn-reload")?.addEventListener("click", () => window.location.reload());
document.querySelector("#btn-debug-exit")?.addEventListener("click", () => ExitLauncher().catch(console.error));
document.querySelector("#btn-debug-bt-pairing")?.addEventListener("click", () => OpenBluetoothPairing().catch(console.error));
document.querySelector("#btn-wifi-networks")?.addEventListener("click", () => {
  closeMenu();
  openWifiDialog();
});
document.querySelector("#btn-sound-settings")?.addEventListener("click", () => {
  closeMenu();
  openSoundSettings();
});
document.querySelector("#btn-restart")?.addEventListener("click", () => Restart());
document.querySelector("#btn-shutdown")?.addEventListener("click", () => Shutdown());
document.querySelector("#btn-explorer")?.addEventListener("click", () => Explorer());
document.querySelector("#btn-open-steam")?.addEventListener("click", () => OpenSteam().catch(console.error));
document.querySelector("#btn-open-retrobat")?.addEventListener("click", () => OpenRetroBat().catch(console.error));

initUIScale(() => controllerRefresh());

const container = document.querySelector(".content .scrollport") || document.querySelector(".content");
const grid = document.createElement("div");
grid.className = "grid";
container.replaceChildren(grid);

const cardsByID = new Map();
const pairingShield = document.getElementById("pairing-input-shield");
let pairingShieldActive = false;

Info("frontend boot").catch(() => {});

function setDebugToolsVisible(debug) {
  document.querySelectorAll("[data-debug-only]").forEach((el) => {
    el.hidden = !debug;
  });
  const exitButton = document.querySelector("#btn-debug-exit");
  const exitLabel = exitButton?.querySelector(".menu-label");
  if (exitButton && exitLabel) {
    exitButton.setAttribute("aria-label", debug ? "Reload Launcher" : "Exit Launcher");
    exitLabel.textContent = debug ? "Reload Launcher" : "Exit Launcher";
  }
  controllerRefresh();
}

function normalizeGame(g, fallbackID = "") {
  return {
    id: g.id || g.ID || fallbackID,
    name: g.name ?? g.Name ?? "",
    description: g.description ?? g.Description ?? "",
    controls: g.controls ?? g.Controls ?? 0,
    thumbnailUrl: g.thumbnailUrl ?? g.ThumbnailURL ?? "",
    imageHash: g.imageHash ?? g.ImageHash ?? "",
    steamAppId: g.steamAppId ?? g.SteamAppID ?? 0,
    steamNeedsUpdate: g.steamNeedsUpdate ?? g.SteamNeedsUpdate ?? false,
  };
}

function normalizeControls(value) {
  const mode = Number(value);
  if (mode === -1) return [];
  if (mode === 1) return ["keyboardMouse"];
  if (mode === 2) return ["keyboardMouse", "controller"];
  return ["controller"];
}

const controlIconURL = {
  controller: controllerIconURL,
  keyboardMouse: keyboardMouseIconURL,
};

function setControlIcons(card, value) {
  const icons = normalizeControls(value);
  const el = card.querySelector(".control-icons");
  if (!el) return;

  el.replaceChildren();
  el.hidden = icons.length === 0;
  el.classList.toggle("has-controller", icons.includes("controller"));
  if (!icons.length) {
    el.removeAttribute("aria-label");
    el.removeAttribute("title");
    return;
  }

  const label = icons.length === 2 ? "Controller and keyboard/mouse" :
    icons[0] === "controller" ? "Controller" : "Keyboard and mouse";
  el.setAttribute("aria-label", label);
  el.title = label;

  icons.forEach((kind) => {
    const icon = document.createElement("span");
    icon.className = `control-icon ${kind}`;
    const img = document.createElement("img");
    img.src = controlIconURL[kind];
    img.alt = "";

    img.decoding = "async";
    icon.append(img);
    el.append(icon);
  });
}

function setLoadStatus(payload) {
  const el = document.getElementById("game-load-status");
  if (!el) return;

  const loading = !!payload?.loading;
  el.classList.toggle("idle", false);
  el.classList.toggle("loading", loading);
  el.classList.toggle("ready", !loading);
  if (payload && Number.isFinite(payload.loaded) && Number.isFinite(payload.total) && payload.total > 0) {
    el.title = loading
      ? `Loading games ${payload.loaded}/${payload.total}`
      : `Games ready ${payload.loaded}/${payload.total}`;
  } else {
    el.title = loading ? "Loading games" : "Games ready";
  }
}

function insertSorted(card) {
  const name = (card.dataset.name || "").toLocaleLowerCase();
  const cards = [...grid.children];
  const before = cards.find((el) => (el.dataset.name || "").toLocaleLowerCase() > name);
  grid.insertBefore(card, before || null);
}

function upsertCard(rawGame) {
  const g = normalizeGame(rawGame);
  if (!g.id) return;

  let card = cardsByID.get(g.id);
  if (!card) {
    card = document.createElement("div");
    card.className = "card";
    card.dataset.id = g.id;
    card.setAttribute("role", "button");
    card.setAttribute("tabindex", "0");

    const img = document.createElement("img");
    img.className = "thumb";
    img.loading = "lazy";
    img.decoding = "async";
    img.fetchPriority = "low";
    img.alt = "";

    const steamBadge = document.createElement("span");
    steamBadge.className = "steam-corner-badge";
    steamBadge.hidden = true;
    steamBadge.setAttribute("role", "img");
    steamBadge.setAttribute("aria-label", "Steam game");
    steamBadge.title = "Steam game";
    const steamLogo = document.createElement("img");
    steamLogo.src = steamIconURL;
    steamLogo.alt = "";
    steamLogo.decoding = "async";
    steamBadge.append(steamLogo);

    const body = document.createElement("div");
    body.className = "body";

    const titleRow = document.createElement("div");
    titleRow.className = "card-title-row";

    const title = document.createElement("div");
    title.className = "title";

    const update = document.createElement("span");
    update.className = "update-needed-icon";
    update.hidden = true;
    update.setAttribute("role", "img");
    update.setAttribute("aria-label", "Update needed");
    update.title = "Update needed — Steam will update this game before launch";
    const updateImage = document.createElement("img");
    updateImage.src = updateIconURL;
    updateImage.alt = "";
    update.append(updateImage);

    const controlIcons = document.createElement("div");
    controlIcons.className = "control-icons";
    controlIcons.hidden = true;

    titleRow.append(update, title, controlIcons);
    body.append(titleRow);
    card.append(img, body, steamBadge);

    card.addEventListener("click", () => Launch(g.id).catch(console.error));
    card.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        Launch(g.id).catch(console.error);
      }
    });

    cardsByID.set(g.id, card);
  }

  card.dataset.name = g.name || "";
  card.dataset.thumbnailUrl = g.thumbnailUrl || "";
  card.querySelector(".steam-corner-badge").hidden = !g.steamAppId;
  card.querySelector(".title").textContent = g.name || "";
  setUpdateNeeded(card, g.steamNeedsUpdate);
  setControlIcons(card, g.controls);
  const img = card.querySelector("img.thumb");
  img.src = g.thumbnailUrl || (g.steamAppId ? steamIconURL : "");
  img.alt = g.name || "";
  img.dataset.hash = g.imageHash || "";

  insertSorted(card);
  controllerRefresh();
}

function setUpdateNeeded(card, needed) {
  card.querySelector(".update-needed-icon").hidden = !needed;
  card.setAttribute("aria-label", `${card.dataset.name}${needed ? ". Update needed. Steam will update this game before launch." : ""}`);
}

let checkingSteamUpdates = false;
async function refreshSteamUpdates() {
  if (checkingSteamUpdates) return;
  checkingSteamUpdates = true;
  try {
    const states = await SteamUpdateStates();
    Object.entries(states || {}).forEach(([id, needed]) => {
      const card = cardsByID.get(id);
      if (card) setUpdateNeeded(card, needed);
    });
  } catch (err) { console.warn("Steam update state unavailable:", err); }
  finally { checkingSteamUpdates = false; }
}

function mergeReturnedGames(raw) {
  const list = Array.isArray(raw)
    ? raw.map((g) => normalizeGame(g))
    : Object.entries(raw || {}).map(([k, g]) => normalizeGame(g, k));
  list.forEach(upsertCard);
}

function repaintGridOnce() {
  grid.classList.add("repaint");
  requestAnimationFrame(() => {
    grid.classList.remove("repaint");
  });
}

function nudgeScrollRepaint() {
  const scroller = container;
  const original = scroller.scrollTop;
  const max = scroller.scrollHeight - scroller.clientHeight;
  if (max <= 0) {
    repaintGridOnce();
    return;
  }

  const delta = original < max ? 1 : -1;
  scroller.scrollTop = original + delta;
  requestAnimationFrame(() => {
    scroller.scrollTop = original;
    repaintGridOnce();
  });
}

function repaintGridAfterRestore() {
  [0, 120, 350, 800].forEach((delay) => {
    window.setTimeout(nudgeScrollRepaint, delay);
  });
}

initLaunchFeedback(() => controllerFocusGrid(), (id) => ({
  thumbnailUrl: cardsByID.get(id)?.dataset.thumbnailUrl || "",
}));
controllerInit({
  getCards: () => [...document.querySelectorAll(".content .grid .card, .content .scrollport .grid .card")],
  getBarButtons: () => [...document.querySelectorAll(".topbar .btn")],
  onActivateCard: (card) => {
    const id = card?.dataset?.id;
    if (id) Launch(id).catch(console.error);
  },
});

EventsOn("game:loaded", upsertCard);
EventsOn("game:update-status", ({ id, needsUpdate }) => {
  const card = cardsByID.get(id);
  if (card) setUpdateNeeded(card, needsUpdate);
});
setInterval(refreshSteamUpdates, 15000);
EventsOn("games:loading", setLoadStatus);
EventsOn("game:load-error", (payload) => {
  console.warn("Game load failed:", payload);
});
EventsOn("game:launch-error", (payload) => {
  console.warn("Game launch failed:", payload);
});
EventsOn("game:end", () => {
  repaintGridAfterRestore();
  controllerFocusGrid();
  refreshSteamUpdates();
});
EventsOn("status:update", (payload) => {
  if (payload && "debug" in payload) setDebugToolsVisible(!!payload.debug);
});
EventsOn("pairing-ui:start", () => {
  pairingShieldActive = true;
  closeMenu();
  if (pairingShield) pairingShield.hidden = false;
});
EventsOn("pairing-ui:end", () => {
  pairingShieldActive = false;
  if (pairingShield) pairingShield.hidden = true;
  controllerFocusGrid();
});

["click", "dblclick", "pointerdown", "pointerup", "mousedown", "mouseup", "keydown"].forEach((type) => {
  window.addEventListener(type, (e) => {
    if (!pairingShieldActive) return;
    e.preventDefault();
    e.stopPropagation();
  }, true);
});

async function loadAndShow() {
  setLoadStatus({ loading: true });
  const raw = await LoadGames().catch((err) => {
    console.error("LoadGames failed:", err);
    setLoadStatus({ loading: false, loaded: 0, total: 0 });
    return {};
  });
  mergeReturnedGames(raw);
  Info("frontend games rendered").catch(() => {});
}

initTopbarMenu({ onClose: () => controllerFocusGrid() });
initWifiDialog({ onClose: () => controllerFocusGrid() });
initSoundSettings({ onClose: () => controllerFocusGrid() });
initBTIndicator();
GetStatuses().then((s) => setDebugToolsVisible(!!s.debug)).catch(() => setDebugToolsVisible(false));
loadAndShow();
startClock("America/Mexico_City");

// optional: disable context menu
window.addEventListener("contextmenu", (e) => { e.preventDefault(); e.stopPropagation(); }, true);
