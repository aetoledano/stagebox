import { EventsOn } from "/wailsjs/runtime";

// Highest visual layer first. A modal owns all activation until it closes;
// aria-modal alone does not disable the library beneath it.
const overlays = [
  ".launch-feedback", ".controllers-overlay", ".bluetooth-devices-overlay",
  "#pairing-input-shield", ".game-options-overlay", "#sound-overlay",
  "#wifi-overlay", "#tools-menu-overlay",
];
let sessionActive = false, pairing = false;

function activeOverlay() {
  return overlays.map((selector) => document.querySelector(selector))
    .find((element) => element && !element.hidden && !element.closest("[hidden]"));
}

export function canActivateLauncher() {
  return !sessionActive && !pairing && !document.hidden && !activeOverlay();
}

export function initLauncherInputGuard() {
  const root = document.getElementById("stagebox-scale-root") || document.body;
  const watched = new WeakSet();
  const update = () => {
    const blocked = !canActivateLauncher();
    root.querySelectorAll(":scope > .content, :scope > .topbar").forEach((element) => {
      element.inert = blocked;
    });
  };
  const visibility = new MutationObserver(update);
  const watchOverlays = () => {
    overlays.forEach((selector) => root.querySelectorAll(selector).forEach((element) => {
      if (watched.has(element)) return;
      watched.add(element);
      visibility.observe(element, { attributes: true, attributeFilter: ["hidden"] });
    }));
    update();
  };
  // Dynamically created dialogs are siblings of the library. Do not observe
  // diagram/readout mutations on every controller sample.
  new MutationObserver(watchOverlays).observe(root, { childList: true });
  watchOverlays();

  ["pointerdown", "pointerup", "mousedown", "mouseup", "click", "dblclick", "contextmenu", "keydown"].forEach((type) => {
    window.addEventListener(type, (event) => {
      const modal = activeOverlay();
      // Modal keyboard handlers live on window and must still receive Escape
      // or Tab if opening a dialog left DOM focus on the body. Inert plus
      // activation checks prevent those keys from operating covered controls.
      if (type === "keydown" && modal && !pairing && !document.hidden) return;
      if (!document.hidden && (!modal || modal.contains(event.target)) &&
          (!sessionActive || modal) && (!pairing || modal?.id === "pairing-input-shield")) return;
      event.preventDefault();
      event.stopImmediatePropagation();
    }, true);
  });
  ["game:launch-begin", "game:start"].forEach((event) => EventsOn(event, () => { sessionActive = true; update(); }));
  ["game:end", "game:launch-blocked"].forEach((event) => EventsOn(event, () => { sessionActive = false; update(); }));
  EventsOn("pairing-ui:start", () => { pairing = true; update(); });
  EventsOn("pairing-ui:end", () => { pairing = false; update(); });
  document.addEventListener("visibilitychange", update);
}
