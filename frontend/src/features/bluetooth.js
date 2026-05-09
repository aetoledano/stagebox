import { GetStatuses } from "/wailsjs/go/main/App.js";
import { EventsOn } from "/wailsjs/runtime";

const BT_GLYPH = "\uE702"; // Segoe MDL2 Bluetooth

let btState = "unknown";
let controllerCount = 0;
let bluetoothControllerConnected = false;

function applyBT(el) {
  el.textContent = BT_GLYPH;
  el.classList.remove("on", "idle", "off", "disabled", "unknown", "pairing");

  if (btState !== "on") {
    const cls = btState === "disabled" || btState === "unknown" ? btState : "off";
    el.classList.add(cls);
    el.title = btState === "disabled"
      ? "Bluetooth disabled"
      : btState === "unknown"
        ? "Bluetooth unavailable"
        : "Bluetooth off";
    return;
  }

  const hasController = bluetoothControllerConnected;
  el.classList.add(hasController ? "on" : "idle");
  el.title = hasController ? "Bluetooth controller connected" : "Bluetooth on, no controller connected";
}

function applyControllerIndicator(count) {
  const el = document.getElementById("controller-indicator");
  if (!el) return;

  const countEl = el.querySelector(".cmi-count");
  if (countEl) {
    countEl.textContent = String(count);
  } else {
    el.textContent = String(count);
  }
  el.classList.toggle("active", count > 0);
  el.classList.toggle("inactive", count <= 0);
  el.title = count === 1 ? "1 controller connected" : `${count} controllers connected`;
}

function paintBTStatus(payload) {
  const el = document.getElementById("bt-indicator");

  if (payload && "bt" in payload) btState = String(payload.bt || "unknown");
  if (payload && "btConnected" in payload) bluetoothControllerConnected = !!payload.btConnected;
  if (payload && "controllerCount" in payload) {
    const nextCount = Number(payload.controllerCount);
    controllerCount = Number.isFinite(nextCount) ? Math.max(0, nextCount) : 0;
  } else if (payload && "btConnected" in payload) {
    controllerCount = payload.btConnected ? 1 : 0;
  }

  if (el) applyBT(el);
  applyControllerIndicator(controllerCount);
}

export function initBTIndicator() {
  // initial BT status
  GetStatuses()
    .then((s) => paintBTStatus(s))
    .catch(() => {});

  // watch for Go-side Bluetooth updates
  EventsOn("status:update", paintBTStatus);
}
