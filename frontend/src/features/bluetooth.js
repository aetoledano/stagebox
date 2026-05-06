import { GetStatuses } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";

const BT_GLYPH = "\uE702"; // Segoe MDL2 Bluetooth

let lastState = "unknown";
let isPairing = false;
let pairingStatus = "off";

function applyBT(el) {
  el.textContent = BT_GLYPH;
  el.classList.remove("on", "off", "disabled", "unknown", "pairing");
  el.classList.add(lastState);
  if (isPairing) el.classList.add("pairing");
  el.title = `Bluetooth: ${lastState}${isPairing ? ` (${pairingStatus})` : ""}`;
}

function paintBTStatus(payload) {
  const el = document.getElementById("bt-indicator");
  if (!el) return;

  if (payload && payload.bt) lastState = String(payload.bt).toLowerCase();
  if (payload && "autopair" in payload) isPairing = !!payload.autopair;
  if (payload && payload.autopairStatus) pairingStatus = String(payload.autopairStatus).toLowerCase();

  applyBT(el);
}

export function initBTIndicator() {
  // initial BT status
  GetStatuses()
    .then((s) => paintBTStatus(s))
    .catch(() => {});

  // watch for Go-side updates (bt and/or autopair)
  EventsOn("status:update", paintBTStatus);
}

// handy refresher you can call after toggling pairing from UI
export async function refreshBTIndicator() {
  try { paintBTStatus(await GetStatuses()); } catch {}
}
