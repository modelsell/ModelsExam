// Client-side copy of the server's password rules (internal/auth), for early
// feedback. The server also rejects common passwords; this only checks shape.
export const USERNAME_PATTERN = /^[A-Za-z0-9_]{3,32}$/

export function passwordProblems(username: string, password: string): string[] {
  const problems: string[] = []
  if ([...password].length < 10) problems.push('Password must be at least 10 characters')
  if (new TextEncoder().encode(password).length > 72) problems.push('Password must be at most 72 bytes')
  const classes = [/\p{Lu}/u, /\p{Ll}/u, /\p{Nd}/u, /[^\p{Lu}\p{Ll}\p{Nd}]/u].filter((re) => re.test(password)).length
  if (classes < 3) problems.push('Password must use at least 3 of: uppercase letters, lowercase letters, digits, symbols')
  if (username && password.toLowerCase().includes(username.toLowerCase()))
    problems.push('Password must not contain the username')
  return problems
}
