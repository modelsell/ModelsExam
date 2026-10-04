// The standalone app has no accounts. Every visitor is the same anonymous
// guest, so history and baselines are always enabled and shared platform-wide.
const state = { auth: { user: { id: 'guest', role: 0 } } }

export function useAuthStore<T>(selector: (value: typeof state) => T): T {
  return selector(state)
}
