export type RealtimeEventKind = 'status' | 'history' | 'ping' | 'metadata'

export interface RealtimeEvent {
  kind: RealtimeEventKind
  uuid: string
  task_id?: number
  time?: string
  value?: number
}

type RealtimeListener = (event: RealtimeEvent) => void

const EVENT_KINDS: RealtimeEventKind[] = ['status', 'history', 'ping', 'metadata']
const listeners = new Set<RealtimeListener>()
let source: EventSource | null = null

function dispatch(raw: string): void {
  try {
    const event = JSON.parse(raw) as RealtimeEvent
    if (!event || typeof event.kind !== 'string' || typeof event.uuid !== 'string' || !event.uuid)
      return
    for (const listener of listeners)
      listener(event)
  }
  catch {
    // Ignore malformed or stale events; the next valid event will refresh the
    // corresponding portion of the UI.
  }
}

function start(): void {
  if (source || typeof window === 'undefined' || typeof EventSource === 'undefined')
    return

  source = new EventSource('/api/realtime', { withCredentials: true })
  for (const kind of EVENT_KINDS) {
    source.addEventListener(kind, (event) => {
      dispatch((event as MessageEvent<string>).data)
    })
  }
}

function stop(): void {
  source?.close()
  source = null
}

export function subscribeRealtimeEvents(listener: RealtimeListener): () => void {
  listeners.add(listener)
  start()

  let released = false
  return () => {
    if (released)
      return
    released = true
    listeners.delete(listener)
    if (!listeners.size)
      stop()
  }
}
