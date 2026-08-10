import { TIME_MS } from './time'

export const REALTIME_CONFIG = {
  websocket: {
    reconnectInterval: 3 * TIME_MS.second,
    maxReconnectAttempts: 5,
    healthCheckTimeout: 5 * TIME_MS.second,
    healthCheckAttempts: 3,
    healthCheckRetryInterval: TIME_MS.second,
  },
} as const
