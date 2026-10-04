import axios from 'axios'

// Same-origin API client. There are no accounts: the server hands each browser
// an opaque owner cookie, which only gates editing a report's remark.
export const api = axios.create({
  baseURL: '',
  headers: { 'Content-Type': 'application/json' },
})

export function getCommonHeaders(): Record<string, string> {
  return { 'Content-Type': 'application/json' }
}
