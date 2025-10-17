import "./css/base.css";
import "./css/cards.css";
import "./css/topbar.css";
import { Shutdown, Restart, LoadGames, Launch, Explorer, AutoPairStart, AutoPairStop, AutoPairRunning } from "/wailsjs/go/main/App.js";
import { controllerInit } from "./controller.js";
import { initBTIndicator, refreshBTIndicator } from "./bluetooth.js"; 
import { startClock } from "./clock.js";

// build topbar once (or keep your existing HTML)
document.querySelector("#btn-reload")?.addEventListener("click", () => window.location.reload());
document.querySelector("#btn-restart")?.addEventListener("click", () => Restart());
document.querySelector("#btn-shutdown")?.addEventListener("click", () => Shutdown());
document.querySelector("#btn-explorer")?.addEventListener("click", () => Explorer());

document.querySelector("#btn-auto-bt-pair")?.addEventListener("click", async () => {
  try {
    const on = await AutoPairRunning();
    if (on) {
      await AutoPairStop();
    } else {
      const WL = "8bitdo,dualshock,xbox,pro"; // tweak your whitelist here
      await AutoPairStart(WL);
    }
    // Refresh visual state (will also be updated by Go via status:update)
    await refreshBTIndicator();

    // Optional: toggle the button “active” look
    const btn = document.getElementById("btn-auto-bt-pair");
    if (btn) btn.classList.toggle("active", await AutoPairRunning());
  } catch (e) {
    console.error("AutoPair toggle failed:", e);
  }
});


const container = document.querySelector(".content .scrollport") || document.querySelector(".content");

function renderGrid(games) {
  const grid = document.createElement("div");
  grid.className = "grid";

  games.forEach((g) => {
    const card = document.createElement("div");
    card.className = "card";
    card.dataset.id = g.id;
    card.setAttribute("role", "button");
    card.setAttribute("tabindex", "0");

    const img = document.createElement("img");
    img.className = "thumb";
    img.loading = "lazy";
    img.decoding = "async";
    img.fetchPriority = "low";
    img.src = g.b64 ? `data:image/*;base64,${g.b64}` : (g.image || "");
    img.alt = "";

    const body = document.createElement("div");
    body.className = "body";

    const title = document.createElement("div");
    title.className = "title";
    title.textContent = g.name || "";

    body.append(title);
    card.append(img, body);

    // Mouse/keyboard activation still works
    card.addEventListener("click", () => Launch(g.id).catch(console.error));
    card.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        Launch(g.id).catch(console.error);
      }
    });

    grid.appendChild(card);
  });

  container.replaceChildren(grid);
}

async function loadAndShow() {
  const raw = await LoadGames().catch(() => ({}));
  const list = Array.isArray(raw)
    ? raw
    : Object.entries(raw).map(([k, g]) => ({
      id: g.id || k,
      name: g.name ?? g.Name,
      description: g.description ?? g.Description,
      b64: g.b64 ?? g.ImageB64,
      image: g.image ?? g.Image,
    }));
  list.sort((a, b) => (a.name || "").localeCompare(b.name || ""));
  renderGrid(list);

  // NOW init controller (after grid is in the DOM)
  controllerInit({
    getCards: () => [...document.querySelectorAll(".content .grid .card, .content .scrollport .grid .card")],
    getBarButtons: () => [...document.querySelectorAll(".topbar .btn")],
    onActivateCard: (card) => {
      const id = card?.dataset?.id;
      if (id) Launch(id).catch(console.error);
    },
  });

  initBTIndicator();
}

loadAndShow();
startClock("America/Mexico_City");

// optional: disable context menu
window.addEventListener("contextmenu", (e) => { e.preventDefault(); e.stopPropagation(); }, true);
