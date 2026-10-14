import { EventsOn } from "/wailsjs/runtime";
import { GetControllers, SetControllerDefault, SelectController, TestController } from "/wailsjs/go/main/App.js";
import diagram from "../assets/images/controller-test.svg?raw";

let overlay, panel, title, list, detail, status, back;
let controllers = [], selected = "", view = "list", busy = false, suspended = false, pairing = false, index = 0, generation = 0;
let onClose = () => {};
let inventorySignature = "";
let testExitSince = 0;
const dismissed = new Set();
export const controllersOpen = () => !!overlay && !overlay.hidden;
const current = () => controllers.find((pad) => pad.id === selected);
const controls = () => [...panel.querySelectorAll("button:not(:disabled)")].filter((button) => !button.closest("[hidden]"));

function focus(at = index) {
  if (!controllersOpen()) return;
  const buttons = controls();
  panel.querySelectorAll(".gp-focus").forEach((item) => item.classList.remove("gp-focus"));
  if (!buttons.length) return;
  index = ((at % buttons.length) + buttons.length) % buttons.length;
  buttons[index].classList.add("gp-focus");
  buttons[index].focus({ preventScroll: true });
}
function message(text, error = false) { status.textContent = text; status.dataset.error = String(error); }
function controllerTag(pad) {
  const id = pad?.id || "";
  let length = 6;
  while (length < id.length && controllers.some((other) => other.id !== id && other.id.startsWith(id.slice(0,length)))) length += 2;
  return `#${id.slice(0,length).toUpperCase()}`;
}
function controllerName(element, pad) {
  element.textContent = pad.name || "Controller";
  const tag = document.createElement("span"); tag.className = "controller-id";
  tag.textContent = controllerTag(pad); tag.title = `Controller ID: ${pad.id}`;
  element.append(" ",tag);
}

function renderList() {
  title.textContent = "Controllers";
  list.hidden = false; detail.hidden = true;
  const focused = document.activeElement?.dataset.controllerId;
  list.replaceChildren();
  controllers.forEach((pad) => {
    const button = document.createElement("button");
    button.className = "controller-row"; button.dataset.controllerId = pad.id;
    const name = document.createElement("strong"); controllerName(name,pad);
    const meta = document.createElement("small"); meta.textContent = `${pad.connected ? pad.connection : "Disconnected"} · ${pad.nativeXInput ? "Native Xbox input" : pad.known ? pad.mode === "virtual" ? "Virtual Xbox default" : "Native default" : "Choose a default"}`;
    button.append(name, meta); button.addEventListener("click", () => showDetails(pad.id)); list.append(button);
  });
  if (!controllers.length) {
    const empty = document.createElement("p"); empty.textContent = "Connect a controller recognized by SDL to choose its default or test its mapping."; list.append(empty);
  }
  if (focused) { const at = controls().findIndex((button) => button.dataset.controllerId === focused); if (at >= 0) index = at; }
}

function renderDetails() {
  const pad = current();
  if (!pad) { view = "list"; renderList(); message("The controller was disconnected before a default was saved.", true); return; }
  title.textContent = view === "prompt" ? "New controller connected" : "Controller settings";
  list.hidden = true; detail.hidden = false; detail.replaceChildren();
  const name = document.createElement("h3"); controllerName(name,pad);
  const connection = document.createElement("p"); connection.className = "controller-meta"; connection.textContent = `${pad.connection} · ${pad.connected ? "Connected" : "Disconnected"}`;
  const description = document.createElement("p"); description.textContent = pad.nativeXInput ? "This controller already provides Xbox input and stays native." : "How should StageBox use this controller in games, Steam and RetroBat? Your choice is remembered when it reconnects.";
  detail.append(name, connection, description);
  [ ["native", "Native", "Let the game read the physical controller."], ["virtual", "Virtual Xbox", "Use an Xbox controller and hide the physical pad in games, Steam and RetroBat."] ].forEach(([mode,label,description]) => {
    const button = document.createElement("button"); button.className = "controller-mode"; button.dataset.mode = mode;
    button.setAttribute("aria-pressed", String(pad.known && pad.mode === mode));
    const strong = document.createElement("strong"); strong.textContent = label;
    const small = document.createElement("small"); small.textContent = description;
    button.append(strong,small); button.disabled = busy || (pad.nativeXInput && mode === "virtual");
    button.addEventListener("click", () => saveMode(mode)); detail.append(button);
  });
  const test = document.createElement("button"); test.className = "controller-test-button"; test.textContent = "Test controller mapping"; test.disabled = busy || !pad.connected;
  test.addEventListener("click", beginTest); detail.append(test);
}

async function showDetails(id, prompt = false) {
  const revision = generation;
  selected = id; view = prompt ? "prompt" : "detail"; index = 0;
  message(prompt ? "Choose Native for a supported controller, or Virtual Xbox for a generic controller that needs Xbox input." : "");
  renderDetails(); focus();
  if (current()?.connected) { try { await SelectController(id); } catch (error) { if (revision === generation && selected === id) message(String(error),true); } }
}
async function saveMode(mode) {
  if (busy) return;
  const id = selected, revision = generation;
  busy = true; renderDetails(); message("Saving controller default…");
  try {
    controllers = await SetControllerDefault(id,mode);
    if (revision !== generation) return;
    dismissed.delete(id);
    if (view === "prompt") { busy = false; await closeControllers(); return; }
    message(`Default saved: ${mode === "virtual" ? "Virtual Xbox" : "Native"}.`);
  } catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally { if (revision === generation && controllersOpen()) { busy = false; renderDetails(); focus(); } }
}
function resetDiagram(state = {}) {
  const pressed = new Set(state.buttons || []);
  detail.querySelectorAll("[data-button]").forEach((part) => part.classList.toggle("pressed", pressed.has(part.dataset.button)));
  for (const side of ["left","right"]) {
    const stick = detail.querySelector(`[data-stick="${side}"]`);
    const x = Number(stick?.dataset.centerX || 0), y = Number(stick?.dataset.centerY || 0), travel = Number(stick?.dataset.travel || 0);
    const dx = state[`${side}X`] || 0, dy = state[`${side}Y`] || 0, magnitude = Math.max(1,Math.hypot(dx,dy));
    stick?.setAttribute("cx", String(x + dx * travel / magnitude));
    stick?.setAttribute("cy", String(y - dy * travel / magnitude));
    detail.querySelector(`[data-trigger="${side}"]`)?.setAttribute("width", String((state[`${side}Trigger`] || 0) * 69));
  }
  const readout = detail.querySelector(".controller-readout");
  const positions = { A: "South", B: "East", X: "West", Y: "North" };
  if (readout) readout.textContent = state.connected === false ? "Controller disconnected" : pressed.size ? [...pressed].map((button) => positions[button] || button).join(" · ") : "Press any button or move a stick";
  const axes = detail.querySelector(".controller-axes");
  if (axes) axes.textContent = `L ${Number(state.leftX || 0).toFixed(2)}, ${Number(state.leftY || 0).toFixed(2)}    R ${Number(state.rightX || 0).toFixed(2)}, ${Number(state.rightY || 0).toFixed(2)}    LT ${Math.round((state.leftTrigger || 0)*100)}%    RT ${Math.round((state.rightTrigger || 0)*100)}%`;
}
async function beginTest() {
  if (busy || !current()?.connected) return;
  const revision = generation, id = selected;
  busy = true;
  try {
    await TestController(id);
    if (revision !== generation || !controllersOpen() || suspended) { await TestController(""); return; }
    view = "test"; title.textContent = `${current()?.name || "Controller"} ${controllerTag(current())} · mapping test`;
    list.hidden = true; detail.hidden = false;
    detail.innerHTML = `<div class="controller-diagram">${diagram}</div><p class="controller-readout" role="status"></p><p class="controller-axes"></p><p class="controller-meta">Highlights show StageBox's mapped buttons, regardless of the printed labels. Launcher actions are paused while testing.</p><button class="controller-stop-test">Back to controller settings</button>`;
    detail.querySelector("button").addEventListener("click", stopTest);
    testExitSince = 0;
    back.textContent = "Close"; resetDiagram(); message("Hold Start + Back for one second to leave, or use the mouse Back button or keyboard Escape. Individual controller buttons stay available for testing."); focus(0);
  } catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally { if (revision === generation) busy = false; }
}
async function stopTest() {
  if (view !== "test" || busy) return;
  const revision = generation;
  busy = true;
  try { await TestController(""); if (revision !== generation) return; view = "detail"; back.textContent = "Back"; renderDetails(); message(""); focus(0); }
  catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally { if (revision === generation) busy = false; }
}
export async function openControllers() {
  if (!overlay || controllersOpen() || suspended) return;
  generation++; view = "list"; index = 0; overlay.hidden = false; message(""); renderList(); focus();
  const revision = generation;
  try { const result = await GetControllers(); if (revision === generation) { controllers = result; renderList(); focus(); } }
  catch (error) { if (revision === generation) message(String(error),true); }
}
export async function closeControllers(force = false) {
  if (!controllersOpen() || (busy && !force)) return;
  if (view === "test") {
    try { await TestController(""); }
    catch (error) { if (!force) { message(String(error),true); return; } }
  }
  if (view === "prompt") dismissed.add(selected);
  generation++; overlay.hidden = true; busy = false; back.textContent = "Back"; onClose();
}
async function goBack() {
  if (busy) return;
  if (view === "test") { await stopTest(); return; }
  if (view === "detail") { view = "list"; renderList(); message(""); focus(0); return; }
  await closeControllers();
}
export function handleControllersNav(direction) {
  if (!controllersOpen()) return false;
  if (!busy && view !== "test") focus(index + (["up","left"].includes(direction) ? -1 : 1));
  return true;
}
export function handleControllersPress(name) {
  if (!controllersOpen()) return false;
  if (view === "test" || busy) return true;
  if (name === "B") goBack();
  else if (name === "A") controls()[index]?.click();
  return true;
}
function maybePrompt() {
  if (!overlay || suspended || pairing || controllersOpen() || !document.hasFocus()) return;
  if (document.querySelector("#tools-menu-overlay:not([hidden]), #sound-overlay:not([hidden]), #wifi-overlay:not([hidden]), .game-options-overlay:not([hidden]), .launch-feedback:not([hidden])")) return;
  const unknown = controllers.find((pad) => pad.connected && !pad.known && !dismissed.has(pad.id));
  if (!unknown) return;
  generation++; overlay.hidden = false; busy = false; back.textContent = "Later"; showDetails(unknown.id,true);
}
export function initControllers(closeCallback) {
  onClose = closeCallback || onClose;
  overlay = document.createElement("div"); overlay.className = "controllers-overlay"; overlay.hidden = true;
  overlay.innerHTML = `<section class="controllers-panel" role="dialog" aria-modal="true" aria-labelledby="controllers-heading"><header><span>STAGEBOX INPUT</span><h2 id="controllers-heading">Controllers</h2></header><div class="controllers-list"></div><div class="controllers-detail" hidden></div><p class="controllers-status" role="status"></p><button class="controllers-back">Back</button></section>`;
  (document.getElementById("stagebox-scale-root") || document.body).append(overlay);
  panel = overlay.querySelector("section"); title = panel.querySelector("h2"); list = panel.querySelector(".controllers-list"); detail = panel.querySelector(".controllers-detail"); status = panel.querySelector(".controllers-status"); back = panel.querySelector(".controllers-back");
  back.addEventListener("click", () => view === "test" ? closeControllers() : goBack());
  window.addEventListener("keydown", (event) => {
    if (!controllersOpen()) return;
    if (event.key === "Escape") { event.preventDefault(); goBack(); }
    else if (event.key === "Tab") { event.preventDefault(); focus(index + (event.shiftKey ? -1 : 1)); }
    else if (view !== "test" && event.key.startsWith("Arrow")) { event.preventDefault(); handleControllersNav(event.key.slice(5).toLowerCase()); }
  });
  EventsOn("controllers:update", (next) => {
    const signature = JSON.stringify(next || []), changed = signature !== inventorySignature;
    inventorySignature = signature;
    controllers = next || [];
    for (const id of dismissed) if (!controllers.some((pad) => pad.id === id && pad.connected)) dismissed.delete(id);
    if (changed && controllersOpen() && view === "list" && !busy) { renderList(); focus(); }
    else if (changed && controllersOpen() && view !== "test" && !busy) { renderDetails(); focus(); }
    maybePrompt();
  });
  EventsOn("controller:test-state", (state) => {
    if (!controllersOpen() || view !== "test" || state.id !== selected) return;
    resetDiagram(state);
    const held = state.connected && state.buttons?.includes("START") && state.buttons.includes("BACK");
    if (!held) testExitSince = 0;
    else if (!testExitSince) testExitSince = performance.now();
    else if (performance.now() - testExitSince >= 1000) stopTest();
  });
  EventsOn("game:launch-begin", () => { suspended = true; closeControllers(true); });
  EventsOn("game:launch-blocked", () => { suspended = false; });
  EventsOn("game:end", () => { suspended = false; });
  EventsOn("pairing-ui:start", () => { pairing = true; });
  EventsOn("pairing-ui:end", () => { pairing = false; });
  GetControllers().then((next) => { controllers = next || []; maybePrompt(); }).catch(console.error);
}
