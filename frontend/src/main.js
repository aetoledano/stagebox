import "./css/base.css";
import "./css/cards.css";
import "./css/topbar.css";
import { OpenSteam, Shutdown, Restart, ExitLauncher, OpenBluetoothPairing, LoadGames, Launch, Explorer, GetStatuses, Info, Warn, Error } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";
import { initBTIndicator } from "./features/bluetooth.js";
import { startClock } from "./features/clock.js";
import { controllerFocusGrid, controllerInit, controllerRefresh } from "./features/controller.js";
import { closeMenu, initTopbarMenu } from "./features/topbarMenu.js";

// build topbar once (or keep your existing HTML)
document.querySelector("#btn-reload")?.addEventListener("click", () => window.location.reload());
document.querySelector("#btn-debug-exit")?.addEventListener("click", () => ExitLauncher().catch(console.error));
document.querySelector("#btn-debug-bt-pairing")?.addEventListener("click", () => OpenBluetoothPairing().catch(console.error));
document.querySelector("#btn-restart")?.addEventListener("click", () => Restart());
document.querySelector("#btn-shutdown")?.addEventListener("click", () => Shutdown());
document.querySelector("#btn-explorer")?.addEventListener("click", () => Explorer());
document.querySelector("#btn-open-steam")?.addEventListener("click", () => OpenSteam().catch(console.error));


const container = document.querySelector(".content .scrollport") || document.querySelector(".content");
const grid = document.createElement("div");
grid.className = "grid";
container.replaceChildren(grid);

const cardsByID = new Map();
const pairingShield = document.getElementById("pairing-input-shield");
let pairingShieldActive = false;

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
    thumbnailUrl: g.thumbnailUrl ?? g.ThumbnailURL ?? "",
    imageHash: g.imageHash ?? g.ImageHash ?? "",
  };
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

    const body = document.createElement("div");
    body.className = "body";

    const title = document.createElement("div");
    title.className = "title";

    body.append(title);
    card.append(img, body);

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
  card.querySelector(".title").textContent = g.name || "";
  const img = card.querySelector("img.thumb");
  img.src = g.thumbnailUrl || "";
  img.dataset.hash = g.imageHash || "";

  insertSorted(card);
  controllerRefresh();
}

function mergeReturnedGames(raw) {
  const list = Array.isArray(raw)
    ? raw.map((g) => normalizeGame(g))
    : Object.entries(raw || {}).map(([k, g]) => normalizeGame(g, k));
  list.forEach(upsertCard);
}

controllerInit({
  getCards: () => [...document.querySelectorAll(".content .grid .card, .content .scrollport .grid .card")],
  getBarButtons: () => [...document.querySelectorAll(".topbar .btn")],
  onActivateCard: (card) => {
    const id = card?.dataset?.id;
    if (id) Launch(id).catch(console.error);
  },
});

EventsOn("game:loaded", upsertCard);
EventsOn("games:loading", setLoadStatus);
EventsOn("game:load-error", (payload) => {
  console.warn("Game load failed:", payload);
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
}

initTopbarMenu({ onClose: () => controllerFocusGrid() });
initBTIndicator();
GetStatuses().then((s) => setDebugToolsVisible(!!s.debug)).catch(() => setDebugToolsVisible(false));
loadAndShow();
startClock("America/Mexico_City");

// optional: disable context menu
window.addEventListener("contextmenu", (e) => { e.preventDefault(); e.stopPropagation(); }, true);
