// Shared types mirroring the mongosync-ui backend JSON contracts.

export type BinaryState =
  | 'absent'
  | 'downloading'
  | 'extracting'
  | 'installed'
  | 'error'

export interface BinaryStatus {
  state: BinaryState
  version?: string
  progress: number
  error?: string
  path?: string
}

export interface MigrationConfig {
  sourceUri: string
  destinationUri: string
  port: number
  version: string
  enableVerifierPersistence: boolean
}

export type SessionMode = 'local' | 'remote'
export type SessionStatus = 'active' | 'committed' | 'stopped' | 'failed'

// SessionSummary is the peak measurable progress observed for a session.
export interface SessionSummary {
  phase?: string
  copiedBytes?: number
  totalBytes?: number
  eventsApplied?: number
  indexesBuilt?: number
  totalIndexes?: number
  verifiedDocuments?: number
  estimatedDocuments?: number
  verifiedCollections?: number
  totalCollections?: number
}

// SessionRecord is one migration session, current or historical.
export interface SessionRecord {
  id: string
  mode: SessionMode
  apiBaseUrl: string
  port?: number
  source: string
  destination: string
  mongosyncVersion?: string
  startedAt: string
  endedAt?: string
  status: SessionStatus
  lastState?: string
  outcome?: string
  summary?: SessionSummary
}

// ActiveView is the active session enriched with live detail.
export interface ActiveView {
  record: SessionRecord
  initHint?: string
  initHintProblem?: boolean
}

// SessionResponse is returned by GET /api/session.
export interface SessionResponse {
  active: ActiveView | null
  binary: BinaryStatus
}

export interface VerificationPhase {
  phase?: string
  estimatedDocumentCount?: number
  hashedDocumentCount?: number
  totalCollectionCount?: number
  scannedCollectionCount?: number
  lagTimeSeconds?: number
}

export interface Progress {
  state?: string
  canCommit?: boolean
  canWrite?: boolean
  info?: string
  lagTimeSeconds?: number | null
  totalEventsApplied?: number
  collectionCopy?: {
    estimatedTotalBytes?: number
    estimatedCopiedBytes?: number
  }
  indexBuilding?: {
    indexesBuilt?: number
    totalIndexesToBuild?: number
    collectionsFinished?: number
    collectionsTotal?: number
  }
  directionMapping?: { Source?: string; Destination?: string }
  source?: { pingLatencyMs?: number }
  destination?: { pingLatencyMs?: number }
  verification?: {
    source?: VerificationPhase
    destination?: VerificationPhase
  }
  warnings?: string[]
  estimatedOplogTimeRemaining?: string
  estimatedSecondsToCEACatchup?: number
  mongosyncID?: string
}

export interface ProgressResponse {
  mode?: string
  success?: boolean
  progress?: Progress
  error?: string
  errorDescription?: string
}

export interface Namespace {
  database: string
  collections?: string[]
}

export interface StartOptions {
  reversible: boolean
  buildIndexes: string
  verification: { enabled: boolean }
  includeNamespaces?: Namespace[]
  excludeNamespaces?: Namespace[]
}

export interface LogsResponse {
  available: boolean
  lines: string[]
}
