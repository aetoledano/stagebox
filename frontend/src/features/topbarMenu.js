const state = { open: false, i: 0 };

let overlay = null;
let toggleButton = null;
let menu = null;
let onClose = null;

function getItems() {
  return [...document.querySelectorAll("#tools-menu-overlay .menu-item")];
}

function updateFocus() {
  const items = getItems();
  items.forEach((item) => item.classList.remove("gp-focus"));
  if (!state.open || !items.length) return;

  state.i = Math.max(0, Math.min(state.i, items.length - 1));
  items[state.i].classList.add("gp-focus");
  items[state.i].focus({ preventScroll: true });
}

export function isMenuOpen() {
  return state.open;
}

export function openMenu(index = 0) {
  if (!overlay || !toggleButton) return;
  document.querySelectorAll(".topbar .btn.gp-focus, .content .card.gp-focus")
    .forEach((el) => el.classList.remove("gp-focus"));
  state.open = true;
  state.i = index;
  overlay.hidden = false;
  document.body.classList.add("tools-menu-open");
  toggleButton.setAttribute("aria-expanded", "true");
  toggleButton.classList.add("active");
  updateFocus();
}

export function closeMenu() {
  if (!overlay || !toggleButton) return;
  state.open = false;
  overlay.hidden = true;
  document.body.classList.remove("tools-menu-open");
  toggleButton.setAttribute("aria-expanded", "false");
  toggleButton.classList.remove("active");
  getItems().forEach((item) => item.classList.remove("gp-focus"));
  toggleButton.blur();
  onClose?.();
}

export function toggleMenu() {
  if (state.open) closeMenu();
  else openMenu();
}

export function handleControllerNav(dir) {
  if (!state.open) return false;
  const items = getItems();
  if (!items.length) return true;

  if (dir === "up" || dir === "left") {
    state.i = Math.max(0, state.i - 1);
  } else if (dir === "down" || dir === "right") {
    state.i = Math.min(items.length - 1, state.i + 1);
  }
  updateFocus();
  return true;
}

export function handleControllerPress(name) {
  if (state.open) {
    if (name === "B") {
      closeMenu();
      return true;
    }
    if (name === "A") {
      const item = getItems()[state.i];
      item?.click();
      return true;
    }
    return false;
  }

  if (name === "B") return false;
  return false;
}

export function initTopbarMenu(opts = {}) {
  onClose = opts.onClose || null;
  overlay = document.getElementById("tools-menu-overlay");
  toggleButton = document.getElementById("btn-tools-menu");
  menu = overlay?.querySelector(".tools-menu") || null;
  if (!overlay || !toggleButton || !menu) return;

  toggleButton.addEventListener("click", toggleMenu);
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) closeMenu();
  });

  getItems().forEach((item, i) => {
    item.addEventListener("mouseenter", () => {
      state.i = i;
      if (state.open) updateFocus();
    });
    item.addEventListener("click", () => {
      window.setTimeout(closeMenu, 0);
    });
  });

  window.addEventListener("keydown", (e) => {
    if (!state.open) return;
    if (e.key === "Escape") {
      e.preventDefault();
      closeMenu();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      handleControllerNav("up");
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      handleControllerNav("down");
    }
  });
}
