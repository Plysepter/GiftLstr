export function confetti(x: number, y: number, colors?: string[]): void {
  if (typeof window === "undefined" || typeof document === "undefined") return;
  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  if (document.documentElement.dataset.reduceVfx) return;
  const palette = colors ?? ["#818CF8", "#E879F9", "#22D3EE", "#FBBF24"];
  for (let i = 0; i < 20; i++) {
    const c = document.createElement("i");
    c.className = "confetti";
    const ang = Math.random() * Math.PI * 2;
    const dist = 50 + Math.random() * 110;
    c.style.background = palette[i % palette.length]!;
    c.style.left = `${x}px`;
    c.style.top = `${y}px`;
    c.style.setProperty("--dx", `${Math.cos(ang) * dist}px`);
    c.style.setProperty("--dy", `${Math.sin(ang) * dist + 120}px`);
    c.style.setProperty("--rot", `${Math.random() * 540 - 270}deg`);
    c.style.setProperty("--dur", `${800 + Math.random() * 600}ms`);
    document.body.appendChild(c);
    setTimeout(() => c.remove(), 1500);
  }
}
