// The Vitest set-up (setupFiles) of shared and the four apps: jsdom has no ResizeObserver, which the components that measure
// themselves use (Table and ScrollX: a region that scrolls sideways takes the keyboard focus). This one never calls
// back and jsdom gives every box a width of 0, so nothing overflows; a test that needs sizes stubs its own (vi.stubGlobal).
const noop = () => { /* jsdom lays nothing out, so there is nothing to observe */ };

if (!("ResizeObserver" in globalThis)) {
  Object.defineProperty(globalThis, "ResizeObserver", {
    configurable: true,
    writable: true,
    value: class { observe = noop; unobserve = noop; disconnect = noop; },
  });
}
