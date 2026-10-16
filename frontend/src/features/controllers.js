import { EventsOn } from "/wailsjs/runtime";
import { GetControllers, SetControllerDefault, RemoveSavedController, SelectController, TestController, TestControllerRumble } from "/wailsjs/go/main/App.js";
import diagram from "../assets/images/controller-test.svg?raw";

let overlay, panel, title, list, detail, status, back;
let controllers = [], selected = "", view = "list", busy = false, suspended = false, pairing = false, index = 0, generation = 0;
let onClose = () => {};
let inventorySignature = "";
let testExitSince = 0;
let rumblePending = false;
let testCaptured = false;
let captureRevision = 0;
let recoveringDisconnect = false;
const dismissed = new Set();
export const controllersOpen = () => !!overlay && !overlay.hidden;
const current = () => controllers.find((pad) => pad.id === selected);
const controls = () => [...panel.querySelectorAll("button:not(:disabled), summary")].filter((button) => !button.closest("[hidden]"));

function focus(at = index) {
  if (!controllersOpen()) return;
  const buttons = controls();
  panel.querySelectorAll(".gp-focus").forEach((item) => item.classList.remove("gp-focus"));
  if (!buttons.length) return;
  index = ((at % buttons.length) + buttons.length) % buttons.length;
  buttons[index].classList.add("gp-focus");
  buttons[index].focus({ preventScroll: true });
  buttons[index].scrollIntoView({ block: "nearest", inline: "nearest" });
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
  if (!pad) { view = "list"; back.textContent = "Back"; renderList(); message("This controller profile is no longer available.", true); return; }
  title.textContent = view === "remove" ? "Remove saved controller?" : view === "prompt" ? "New controller connected" : "Controller settings";
  back.textContent = view === "remove" ? "Cancel" : view === "prompt" ? "Later" : "Back";
  list.hidden = true; detail.hidden = false; detail.replaceChildren();
  const name = document.createElement("h3"); controllerName(name,pad);
  const connection = document.createElement("p"); connection.className = "controller-meta"; connection.textContent = `${pad.connection} · ${pad.connected ? "Connected" : "Disconnected"}`;
  if (view === "remove") {
    const copy = document.createElement("p"); copy.textContent = "Remove this saved default from StageBox. Paired Bluetooth controllers must be forgotten from Bluetooth devices first.";
    const remove = document.createElement("button"); remove.className = "controller-remove-button"; remove.textContent = "Remove saved controller"; remove.disabled = busy || pad.connected;
    remove.addEventListener("click", removeSavedController);
    detail.append(name, connection, copy, remove);
    return;
  }
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
  if (!pad.connected && pad.known) {
    const remove = document.createElement("button"); remove.className = "controller-remove-button"; remove.textContent = "Remove saved controller"; remove.disabled = busy;
    remove.addEventListener("click", () => { view = "remove"; renderDetails(); message(""); focus(controls().length - 1); });
    detail.append(remove);
  }
}

async function removeSavedController() {
  if (busy || view !== "remove" || current()?.connected) return;
  const id = selected, revision = generation;
  busy = true; renderDetails(); message("Removing saved controller…");
  try {
    const next = await RemoveSavedController(id);
    if (revision !== generation) return;
    controllers = next || []; dismissed.delete(id); selected = "";
    view = "list"; back.textContent = "Back"; index = 0; message("Saved controller removed.");
  } catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally {
    if (revision === generation && controllersOpen()) {
      busy = false;
      if (view === "list") renderList(); else renderDetails();
      focus(view === "remove" ? controls().length - 1 : 0);
    }
  }
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
  if (readout) readout.textContent = state.connected === false ? "Controller disconnected" : pressed.size ? [...pressed].map((button) => positions[button] || button).join(" · ") : "Move a control";
  const axes = detail.querySelector(".controller-axes");
  if (axes) axes.textContent = `L ${Number(state.leftX || 0).toFixed(2)}, ${Number(state.leftY || 0).toFixed(2)}    R ${Number(state.rightX || 0).toFixed(2)}, ${Number(state.rightY || 0).toFixed(2)}    LT ${Math.round((state.leftTrigger || 0)*100)}%    RT ${Math.round((state.rightTrigger || 0)*100)}%`;
  const source = (binding) => !binding || binding === "unmapped" ? "unmapped" : /^b\d+$/.test(binding) ? `digital button (${binding})` : /^[+-]?a\d+~?$/.test(binding) ? `axis (${binding})` : binding;
  const triggers = detail.querySelector(".controller-trigger-sources");
  if (triggers) triggers.textContent = `LT: ${source(state.leftTriggerSource)} · RT: ${source(state.rightTriggerSource)}`;
  const raw = detail.querySelector(".controller-raw-axes");
  if (raw) raw.textContent = `SDL triggers: ${state.sdlLeftTrigger || 0} / ${state.sdlRightTrigger || 0} (0–32767)\nRaw axes: ${(state.rawAxes || []).map((value,axis) => `a${axis}=${value}`).join("  ") || "unavailable"}`;
  const rumble = detail.querySelector(".controller-rumble-test");
  if (rumble) rumble.disabled = rumblePending || state.connected !== true;
}

async function testRumble() {
  if (rumblePending || busy || view !== "test" || !current()?.connected) return;
  const revision = generation, id = selected, button = detail.querySelector(".controller-rumble-test"), captureOperation = captureRevision;
  rumblePending = true; button.disabled = true;
  message("Testing rumble…");
  try {
    await TestControllerRumble(id);
    if (revision === generation && captureOperation === captureRevision && controllersOpen() && view === "test" && selected === id && detail.contains(button)) message("Rumble pulse sent.");
  } catch (error) {
    if (revision === generation && captureOperation === captureRevision && controllersOpen() && view === "test" && selected === id && detail.contains(button)) message(String(error?.message || error),true);
  } finally {
    // The button may have been replaced while its request was in flight.
    if (detail.contains(button)) { rumblePending = false; button.disabled = !current()?.connected; }
  }
}

async function toggleTestCapture() {
  if (busy || view !== "test") return;
  const revision = generation, id = selected, capture = !testCaptured, operation = ++captureRevision;
  busy = true;
  try {
    await TestController(capture ? id : "");
    if (operation !== captureRevision) return;
    if (revision !== generation || !controllersOpen() || view !== "test" || selected !== id || suspended) {
      if (capture) await TestController("");
      return;
    }
    testCaptured = capture; testExitSince = 0;
    const button = detail.querySelector(".controller-capture-test");
    button.textContent = capture ? "Release controls" : "Grab controls";
    button.setAttribute("aria-pressed", String(capture));
    if (capture) {
      message("Hold Select + Start (1s) to release.");
    } else {
      detail.querySelector(".controller-readout").textContent = "Input paused";
      detail.querySelector(".controller-rumble-test").disabled = rumblePending || !current()?.connected;
      message("Controls released · D-pad to move · A to select.");
      focus(controls().findIndex((item) => item.classList.contains("controller-rumble-test")));
    }
  } catch (error) { if (revision === generation && operation === captureRevision && view === "test") message(String(error?.message || error),true); }
  finally { if (revision === generation && operation === captureRevision) busy = false; }
}

async function releaseDisconnectedTest(id) {
  if (!controllersOpen() || view !== "test" || selected !== id || recoveringDisconnect) return;
  const revision = generation, operation = ++captureRevision;
  recoveringDisconnect = true; testCaptured = false; testExitSince = 0; busy = true;
  const capture = detail.querySelector(".controller-capture-test");
  capture.textContent = "Grab controls"; capture.setAttribute("aria-pressed", "false"); capture.disabled = true;
  detail.querySelector(".controller-rumble-test").disabled = true;
  detail.querySelector(".controller-readout").textContent = "Controller disconnected";
  message("Controls released. Use another controller or keyboard.");
  try { await TestController(""); await TestControllerRumble(""); }
  catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally {
    if (revision === generation && operation === captureRevision) {
      recoveringDisconnect = false; busy = false;
      if (controllersOpen() && view === "test") focus(controls().findIndex((button) => button.classList.contains("controller-stop-test")));
    }
  }
}
async function beginTest() {
  if (busy || !current()?.connected) return;
  const revision = generation, id = selected;
  busy = true;
  try {
    await TestController(id);
    if (revision !== generation || !controllersOpen() || suspended) { await TestController(""); return; }
    view = "test"; testCaptured = true; recoveringDisconnect = false; captureRevision++; title.textContent = `${current()?.name || "Controller"} ${controllerTag(current())} · test`;
    list.hidden = true; detail.hidden = false;
    detail.innerHTML = `<div class="controller-diagram">${diagram}</div><p class="controller-readout" role="status"></p><p class="controller-axes"></p><div class="controller-test-actions"><button class="controller-capture-test" aria-pressed="true">Release controls</button><button class="controller-rumble-test" title="Test physical controller rumble via SDL">Test rumble</button><button class="controller-stop-test">Back to settings</button></div><details class="controller-input-details"><summary>Input details</summary><p class="controller-meta controller-trigger-sources"></p><p class="controller-axes controller-raw-axes"></p></details>`;
    rumblePending = false;
    detail.querySelector(".controller-rumble-test").addEventListener("click", testRumble);
    detail.querySelector(".controller-capture-test").addEventListener("click", toggleTestCapture);
    detail.querySelector(".controller-stop-test").addEventListener("click", stopTest);
    testExitSince = 0;
    back.textContent = "Close"; resetDiagram(); message("Hold Select + Start (1s) to release."); focus(0);
  } catch (error) { if (revision === generation) message(String(error?.message || error),true); }
  finally { if (revision === generation) busy = false; }
}
async function stopTest() {
  if (view !== "test" || busy) return;
  const revision = generation;
  busy = true;
  try { await TestController(""); await TestControllerRumble(""); if (revision !== generation) return; testCaptured = false; busy = false; view = "detail"; back.textContent = "Back"; renderDetails(); message(""); focus(0); }
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
    try { await TestController(""); await TestControllerRumble(""); }
    catch (error) { if (!force) { message(String(error),true); return; } }
  }
  if (view === "prompt") dismissed.add(selected);
  generation++; captureRevision++; overlay.hidden = true; busy = false; testCaptured = false; recoveringDisconnect = false; back.textContent = "Back"; onClose();
}
async function goBack() {
  if (busy) return;
  if (view === "test") { await stopTest(); return; }
  if (view === "remove") { view = "detail"; renderDetails(); message(""); focus(0); return; }
  if (view === "detail") { view = "list"; renderList(); message(""); focus(0); return; }
  await closeControllers();
}
export function handleControllersNav(direction) {
  if (!controllersOpen()) return false;
  if (!busy && (view !== "test" || !testCaptured)) focus(index + (["up","left"].includes(direction) ? -1 : 1));
  return true;
}
export function handleControllersPress(name) {
  if (!controllersOpen()) return false;
  if ((view === "test" && testCaptured) || busy) return true;
  if (name === "B") goBack();
  else if (name === "A") controls()[index]?.click();
  return true;
}
function maybePrompt(foregroundID = "") {
  if (!overlay || suspended || pairing || controllersOpen() || document.hidden || (!foregroundID && !document.hasFocus())) return;
  if (document.querySelector("#tools-menu-overlay:not([hidden]), #sound-overlay:not([hidden]), #wifi-overlay:not([hidden]), .bluetooth-devices-overlay:not([hidden]), .game-options-overlay:not([hidden]), .launch-feedback:not([hidden])")) return;
  const unknown = controllers.find((pad) => pad.connected && !pad.known && !dismissed.has(pad.id) && (!foregroundID || pad.id === foregroundID));
  if (!unknown) return;
  generation++; overlay.hidden = false; busy = false; back.textContent = "Later"; showDetails(unknown.id,true);
}
export function initControllers(closeCallback) {
  onClose = closeCallback || onClose;
  overlay = document.createElement("div"); overlay.className = "controllers-overlay"; overlay.hidden = true;
  overlay.innerHTML = `<section class="controllers-panel" role="dialog" aria-modal="true" aria-labelledby="controllers-heading"><header><span>STAGEBOX INPUT</span><h2 id="controllers-heading">Controllers</h2></header><div class="controllers-content"><div class="controllers-list"></div><div class="controllers-detail" hidden></div></div><footer><p class="controllers-status" role="status"></p><button class="controllers-back">Back</button></footer></section>`;
  (document.getElementById("stagebox-scale-root") || document.body).append(overlay);
  panel = overlay.querySelector("section"); title = panel.querySelector("h2"); list = panel.querySelector(".controllers-list"); detail = panel.querySelector(".controllers-detail"); status = panel.querySelector(".controllers-status"); back = panel.querySelector(".controllers-back");
  back.addEventListener("click", () => view === "test" ? closeControllers() : goBack());
  panel.addEventListener("focusin", (event) => {
    const at = controls().indexOf(event.target);
    if (at < 0) return;
    index = at;
    panel.querySelectorAll(".gp-focus").forEach((item) => item.classList.toggle("gp-focus", item === event.target));
  });
  window.addEventListener("keydown", (event) => {
    if (!controllersOpen()) return;
    if (event.key === "Escape") { event.preventDefault(); goBack(); }
    else if (event.key === "Tab") { event.preventDefault(); focus(index + (event.shiftKey ? -1 : 1)); }
    else if ((view !== "test" || !testCaptured) && event.key.startsWith("Arrow")) { event.preventDefault(); handleControllersNav(event.key.slice(5).toLowerCase()); }
  });
  EventsOn("controllers:update", (next) => {
    const signature = JSON.stringify(next || []), changed = signature !== inventorySignature;
    inventorySignature = signature;
    controllers = next || [];
    for (const id of dismissed) if (!controllers.some((pad) => pad.id === id && pad.connected)) dismissed.delete(id);
    if (changed && controllersOpen() && view === "list" && !busy) { renderList(); focus(); }
    else if (changed && controllersOpen() && view !== "test" && !busy) { renderDetails(); focus(); }
    else if (controllersOpen() && view === "test") {
      const pad = current();
      detail.querySelector(".controller-rumble-test").disabled = rumblePending || !pad?.connected;
      detail.querySelector(".controller-capture-test").disabled = !testCaptured && !pad?.connected;
      if (testCaptured && !pad?.connected) releaseDisconnectedTest(selected);
    }
    maybePrompt();
  });
  EventsOn("controller:test-state", (state) => {
    if (!controllersOpen() || view !== "test" || !testCaptured || state.id !== selected) return;
    resetDiagram(state);
    if (!state.connected) { releaseDisconnectedTest(state.id); return; }
    const held = state.connected && state.buttons?.includes("START") && state.buttons.includes("BACK");
    if (!held) testExitSince = 0;
    else if (!testExitSince) testExitSince = performance.now();
    else if (performance.now() - testExitSince >= 1000) toggleTestCapture();
  });
  EventsOn("controller:test-released", ({id, reason}) => {
    if (reason === "disconnected") releaseDisconnectedTest(id);
  });
  EventsOn("controller:prompt-ready", ({id}) => maybePrompt(id));
  window.addEventListener("focus", () => maybePrompt());
  document.addEventListener("visibilitychange", () => maybePrompt());
  EventsOn("game:launch-begin", () => { suspended = true; closeControllers(true); });
  EventsOn("game:launch-blocked", () => { suspended = false; });
  EventsOn("game:end", () => { suspended = false; });
  EventsOn("pairing-ui:start", () => { pairing = true; });
  EventsOn("pairing-ui:end", () => { pairing = false; maybePrompt(); });
  GetControllers().then((next) => { controllers = next || []; maybePrompt(); }).catch(console.error);
}
