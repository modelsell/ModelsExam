import type { Channel } from './types'

// Channel-based checks do not exist in the standalone app.
export async function getChannel(
  _id: number
): Promise<{ success: boolean; data?: Channel; message?: string }> {
  return { success: false, message: 'Channels are not available' }
}
