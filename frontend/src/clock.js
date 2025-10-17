let clockTimer = null;

export function startClock(tz /* optional e.g. "America/Mexico_City" */) {
  const el = document.getElementById("clock");
  if (!el) return;

  const tick = () => {
    const d = new Date();
    let h = d.getHours();
    const mer = h >= 12 ? "PM" : "AM";
    h = h % 12 || 12;                       // 0 → 12
    const m = String(d.getMinutes()).padStart(2, "0");
    el.textContent = `${h}:${m} ${mer}`;    // e.g., 3:07 PM
  };

  if (clockTimer) clearInterval(clockTimer);
  tick();
  clockTimer = setInterval(tick, 1000);
}