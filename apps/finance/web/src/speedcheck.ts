// Speed check (Settings): times what feels slow on a given computer — tab
// switches, the theme switch, opening a sheet, the API — and describes the
// machine (screen scale, graphics). The report holds no financial data.
import { applyThemeClass } from '@hub/app-sdk';
import { go, type Page } from './router';

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const frame = () => new Promise((r) => requestAnimationFrame(r));

/** Long frames (≥ 50 ms) seen while fn runs: count and total time. */
async function longFrames<T>(fn: () => Promise<T>): Promise<{ value: T; count: number; total: number }> {
  const seen: PerformanceEntry[] = [];
  let obs: PerformanceObserver | null = null;
  try {
    obs = new PerformanceObserver((l) => seen.push(...l.getEntries()));
    obs.observe({ type: 'long-animation-frame' });
  } catch {
    // Not supported: report without long frames.
  }
  const value = await fn();
  await sleep(100);
  obs?.disconnect();
  return { value, count: seen.length, total: seen.reduce((s, e) => s + e.duration, 0) };
}

async function until(test: () => boolean, timeout = 3000) {
  const t0 = performance.now();
  while (!test() && performance.now() - t0 < timeout) await frame();
}

const pageShown = (p: Page) => {
  const el = document.querySelector<HTMLElement>(`main > [data-page="${p}"]`);
  return !!el && el.checkVisibility() && el.getBoundingClientRect().height > 0;
};

const median = (xs: number[]) => [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)] ?? 0;
const ms = (x: number) => `${Math.round(x)} ms`;

function machine(): string[] {
  let gpu = 'unknown';
  try {
    const gl = document.createElement('canvas').getContext('webgl');
    const ext = gl?.getExtension('WEBGL_debug_renderer_info');
    gpu = String(gl ? gl.getParameter(ext ? ext.UNMASKED_RENDERER_WEBGL : gl.RENDERER) : 'no WebGL');
  } catch {
    gpu = 'no WebGL';
  }
  const nav = navigator as Navigator & { deviceMemory?: number };
  return [
    `Browser: ${navigator.userAgent}`,
    `Graphics: ${gpu}`,
    `Screen: ${window.screen.width}×${window.screen.height} at ${window.devicePixelRatio}x, window ${window.innerWidth}×${window.innerHeight}`,
    `CPU cores: ${navigator.hardwareConcurrency ?? '?'}${nav.deviceMemory ? `, memory ≥ ${nav.deviceMemory} GB` : ''}`,
  ];
}

/** Runs the check and returns the report. It ends back on Settings. */
export async function runSpeedCheck(progress: (s: string) => void): Promise<string> {
  const lines = ['Finance speed check', new Date().toISOString(), ...machine(), ''];

  progress('Timing the app’s data…');
  lines.push('Data (median of 5):');
  for (const path of ['/api/status', '/api/overview', '/api/goals', '/api/transactions?limit=500']) {
    const times: number[] = [];
    for (let i = 0; i < 5; i++) {
      const t0 = performance.now();
      await (await fetch(path, { cache: 'no-store' })).arrayBuffer();
      times.push(performance.now() - t0);
    }
    lines.push(`  ${path}: ${ms(median(times))}`);
  }

  lines.push('', 'Tab switches (click → new page on screen):');
  for (const p of ['overview', 'goals', 'transactions', 'import', 'overview', 'transactions', 'settings'] as Page[]) {
    progress(`Switching to ${p}…`);
    const r = await longFrames(async () => {
      const t0 = performance.now();
      go(p);
      await until(() => pageShown(p));
      await frame();
      return performance.now() - t0;
    });
    lines.push(`  → ${p}: ${ms(r.value)}${r.count ? `, ${r.count} long frame(s) ${ms(r.total)}` : ''}`);
    await sleep(300);
  }

  progress('Switching the theme…');
  const html = document.documentElement;
  const was = html.classList.contains('dark') ? 'dark' : 'light';
  lines.push('', 'Theme switch (to new colours on screen):');
  for (const t of [was === 'dark' ? 'light' : 'dark', was] as const) {
    const r = await longFrames(async () => {
      const t0 = performance.now();
      applyThemeClass(t);
      await frame();
      await frame();
      return performance.now() - t0;
    });
    lines.push(`  → ${t}: ${ms(r.value)}${r.count ? `, ${r.count} long frame(s) ${ms(r.total)}` : ''}`);
    await sleep(300);
  }

  progress('Opening a sheet…');
  go('transactions');
  await until(() => pageShown('transactions'));
  await sleep(300);
  const add = [...document.querySelectorAll<HTMLButtonElement>('main > [data-page="transactions"] button')].find(
    (b) => b.textContent?.trim() === 'Add',
  );
  if (add) {
    const r = await longFrames(async () => {
      const t0 = performance.now();
      add.click();
      await until(() => !!document.querySelector('[role="dialog"]'));
      const shown = performance.now() - t0;
      const dlg = document.querySelector('[role="dialog"]');
      await Promise.all((dlg?.getAnimations() ?? []).map((a) => a.finished.catch(() => undefined)));
      return [shown, performance.now() - t0];
    });
    lines.push('', `Sheet: starts ${ms(r.value[0])}, fully open ${ms(r.value[1])}${r.count ? `, ${r.count} long frame(s) ${ms(r.total)}` : ''}`);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await sleep(400);
  }

  go('settings');
  await until(() => pageShown('settings'));
  return lines.join('\n');
}
