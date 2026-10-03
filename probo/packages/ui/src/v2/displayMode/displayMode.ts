// Copyright (c) 2026 Probo Inc <hello@probo.com>.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

export type DisplayMode = "light" | "dark";

type Listener = () => void;

// Black Fortress ships a single dark theme. The display-mode API is kept so
// existing callers (theme toggles in menus) keep compiling, but every mode
// resolves to "dark" and toggling is a no-op.
const FORCED_DISPLAY_MODE: DisplayMode = "dark";

const listeners = new Set<Listener>();

export function getDisplayMode(): DisplayMode {
  return FORCED_DISPLAY_MODE;
}

function applyDisplayMode(): void {
  if (typeof document === "undefined") {
    return;
  }
  document.documentElement.classList.add("dark");
}

export function setDisplayMode(_mode: DisplayMode): void {
  applyDisplayMode();
  for (const listener of listeners) {
    listener();
  }
}

export function toggleDisplayMode(): void {
  setDisplayMode(FORCED_DISPLAY_MODE);
}

export function subscribeDisplayMode(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function initDisplayMode(): void {
  applyDisplayMode();
}
