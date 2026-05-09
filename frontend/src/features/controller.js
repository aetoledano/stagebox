import { EventsOn } from "/wailsjs/runtime";

import { KillRunning } from "/wailsjs/go/main/App.js";
import { handleControllerNav, handleControllerPress } from "./topbarMenu.js";


let getCards = () => [];
let getBarButtons = () => [];
let onActivateCard = null;

const state = { mode: "grid", i: 0 };

function updateClasses() {
  const cards = getCards();
  const btns  = getBarButtons();

  cards.forEach((c) => c.classList.remove("gp-focus"));
  btns.forEach((b) => b.classList.remove("gp-focus"));

  if (state.mode === "grid" && cards.length) {
    const i = Math.max(0, Math.min(state.i, cards.length - 1));
    state.i = i;
    const el = cards[i];
    el.classList.add("gp-focus");
    // ensure visible
    const scroller = document.querySelector(".content .scrollport") || document.querySelector(".content");
    el.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "nearest" });
  } else if (state.mode === "bar" && btns.length) {
    const i = Math.max(0, Math.min(state.i, btns.length - 1));
    state.i = i;
    btns[i].classList.add("gp-focus");
  }
}

export function controllerFocusGrid(index = state.i) {
  state.mode = "grid";
  state.i = index;
  updateClasses();
}

function cols() {
  const cards = getCards();
  if (cards.length < 2) return 1;
  const top0 = cards[0].offsetTop;
  const idx = cards.findIndex((c) => c.offsetTop !== top0);
  return idx > 0 ? idx : cards.length;
}

function nearestBarIndexFromCard(card) {
  const btns = getBarButtons();
  if (!btns.length) return 0;
  const r = card.getBoundingClientRect(), cx = r.left + r.width/2;
  let best = 0, dist = Infinity;
  btns.forEach((b,i) => {
    const rb = b.getBoundingClientRect(), cb = rb.left + rb.width/2;
    const d = Math.abs(cb - cx);
    if (d < dist) { dist = d; best = i; }
  });
  return best;
}

function nearestCardIndexFromBar(btn) {
  const cards = getCards();
  if (!cards.length) return 0;
  const c = cols(), firstRow = Math.min(c, cards.length);
  const rb = btn.getBoundingClientRect(), cbx = rb.left + rb.width/2;
  let best = 0, dist = Infinity;
  for (let i=0;i<firstRow;i++) {
    const rc = cards[i].getBoundingClientRect(), ccx = rc.left + rc.width/2;
    const d = Math.abs(ccx - cbx);
    if (d < dist) { dist = d; best = i; }
  }
  return best;
}

function move(dir) {
  if (handleControllerNav(dir)) return;

  if (state.mode === "grid") {
    const cards = getCards();
    if (!cards.length) return;
    const c = cols();
    const inTopRow = state.i < c;

    if (dir === "up" && inTopRow) {
      state.mode = "bar";
      state.i = nearestBarIndexFromCard(cards[state.i]);
    } else if (dir === "down") {
      state.i = Math.min(state.i + c, cards.length - 1);
    } else if (dir === "up") {
      state.i = Math.max(state.i - c, 0);
    } else if (dir === "left") {
      const rowStart = Math.floor(state.i / c) * c;
      state.i = Math.max(rowStart, state.i - 1);
    } else if (dir === "right") {
      const rowEnd = Math.min(Math.floor(state.i / c) * c + c - 1, cards.length - 1);
      state.i = Math.min(rowEnd, state.i + 1);
    }
  } else {
    const btns = getBarButtons();
    if (!btns.length) return;
    if (dir === "down") {
      state.mode = "grid";
      state.i = nearestCardIndexFromBar(btns[state.i]);
    } else if (dir === "left") {
      state.i = Math.max(0, state.i - 1);
    } else if (dir === "right") {
      state.i = Math.min(btns.length - 1, state.i + 1);
    }
  }
  updateClasses();
}

function activate() {
  if (state.mode === "grid") {
    const cards = getCards();
    if (!cards.length) return;
    onActivateCard?.(cards[state.i]);
  } else {
    const btns = getBarButtons();
    if (!btns.length) return;
    btns[state.i].click();
  }
}

function isEditableTarget(target) {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || target.isContentEditable;
}

function isPairingShieldActive() {
  const shield = document.getElementById("pairing-input-shield");
  return !!shield && !shield.hidden;
}

function handleKeyboard(e) {
  if (isPairingShieldActive()) return;
  if (isEditableTarget(e.target)) return;

  const dirs = {
    ArrowUp: "up",
    ArrowDown: "down",
    ArrowLeft: "left",
    ArrowRight: "right",
  };
  const dir = dirs[e.key];
  if (dir) {
    e.preventDefault();
    e.stopImmediatePropagation();
    move(dir);
    return;
  }

  if (e.key === "Enter") {
    e.preventDefault();
    e.stopImmediatePropagation();
    if (handleControllerPress("A")) return;
    activate();
    return;
  }

  if (e.key === " ") {
    e.preventDefault();
    e.stopImmediatePropagation();
    handleControllerPress("START");
  }
}

export function controllerInit(opts) {
  getCards        = opts.getCards;
  getBarButtons   = opts.getBarButtons || (()=>[]);
  onActivateCard  = opts.onActivateCard || null;

  // Start selected at first card
  state.mode = "grid";
  state.i = 0;
  updateClasses();

  // Wire Go -> JS events
  EventsOn("pad:nav",   ({dir}) => move(dir));
  EventsOn("pad:press", (name) => { 
    if (handleControllerPress(name)) return;
    if (name === "A") activate() 
    else if (name === "KILL") KillRunning()
  });
  window.addEventListener("keydown", handleKeyboard, true);
}

export function controllerRefresh() {
  updateClasses();
}
