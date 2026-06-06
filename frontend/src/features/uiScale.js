const DESIGN_WIDTH = 1920;
const DESIGN_HEIGHT = 1080;

export function initUIScale(onChange = null) {
  const apply = () => {
    const width = Math.max(1, window.innerWidth || DESIGN_WIDTH);
    const height = Math.max(1, window.innerHeight || DESIGN_HEIGHT);
    const scale = Math.max(1, Math.min(width / DESIGN_WIDTH, height / DESIGN_HEIGHT));

    document.documentElement.style.setProperty("--stagebox-scale", scale.toFixed(4));
    document.documentElement.style.setProperty("--stagebox-width", `${width / scale}px`);
    document.documentElement.style.setProperty("--stagebox-height", `${height / scale}px`);
    onChange?.();
  };

  apply();
  window.addEventListener("resize", apply);
  window.visualViewport?.addEventListener("resize", apply);
}
