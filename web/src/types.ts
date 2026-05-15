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
}

export type SessionMode = 'none' | 'local' | 'remote'

export interface SessionView {
  mode: SessionMode
  apiBaseUrl?: string
  pid?: number
  running: boolean
  startedAt?: string
  config: MigrationConfig
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
