// Hash-based navigation (see docs/decisions/0004-hash-router-instead-of-tanstack.md).
// Kept outside components so React's immutability rules don't flag writes to
// window.location from event handlers.
export function navigate(hash: string): void {
  window.location.hash = hash
}
