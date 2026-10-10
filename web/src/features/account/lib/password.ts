// Client-side copy of the server's password rules (internal/auth), for early
// feedback. The server also rejects common passwords; this only checks shape.
export const USERNAME_PATTERN = /^[A-Za-z0-9_]{3,32}$/

export function passwordProblems(password: string): string[] {
  const problems: string[] = []
  if ([...password].length < 8) problems.push('Password must be at least 8 characters')
  if (new TextEncoder().encode(password).length > 72) problems.push('Password must be at most 72 bytes')
  if (!/\p{L}/u.test(password) || !/\p{Nd}/u.test(password)) problems.push('Password must contain both letters and digits')
  return problems
}
