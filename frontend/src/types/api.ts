/**
 * Pinguin Dinus API Contracts
 * Strictly synchronized with backend/api/openapi.yaml
 */

export interface Pagination {
  page: number
  page_size: number
  total: number
  total_pages: number
}

export interface PaginatedResponse<T> {
  data: T[]
  pagination: Pagination
}

export interface ApiErrorDetail {
  code: string
  message: string
  request_id: string
}

export interface ErrorResponse {
  error: ApiErrorDetail
}

export interface HealthResponse {
  status: 'ok'
}

export interface ReadinessResponse {
  status: 'ok' | 'degraded' | 'unhealthy'
  dependencies?: {
    database?: string
    listing_dir?: string
    [key: string]: unknown
  }
}

export type RepositoryCategory = 'iso' | 'package' | 'tool'

export interface RepositorySummary {
  id: string
  slug: string
  path_prefix: string
  name: string
  distribution?: string
  category: RepositoryCategory
  description: string | null
  status: string
}

export interface ReleaseFile {
  path: string
  size_bytes: number
  checksum: string | null
  checksum_algorithm: string | null
  download_url: string
}

export interface Release {
  version: string
  released_at: string | null
  files: ReleaseFile[]
}

export interface RepositoryDetail extends RepositorySummary {
  releases?: Release[]
  mirror_status?: string
}

export type StatsPeriod = '24h' | '7d' | '30d'

export interface PopularFile {
  path: string
  requests: number
  bytes_sent: number
}

export interface StatsSummary {
  period: StatsPeriod
  generated_at: string
  download_requests: number
  completed_downloads: number
  bandwidth_bytes: number
  popular_files: PopularFile[]
}

export interface TimeseriesPoint {
  bucket_start: string
  download_requests: number
  completed_downloads: number
  bytes_sent: number
}

export interface RegionAggregate {
  region: string
  country: string
  latitude: number | null
  longitude: number | null
  downloads: number
  period: StatsPeriod
}

export type ListingEntryKind = 'directory' | 'file'

export interface ListingEntry {
  name: string
  path: string
  kind: ListingEntryKind
  size_bytes?: number
  modified_at?: string
  download_url?: string
}

export interface ListingResponse {
  path: string
  parent_path: string | null
  entries: ListingEntry[]
}

export interface MirrorTargetStatus {
  name: string
  url: string
  is_reachable: boolean
  status_code?: number
  latency_ms?: number
  last_checked_at: string
}

export type QuestionType =
  | 'single_choice'
  | 'multiple_choice'
  | 'short_text'
  | 'long_text'
  | 'iso_proposal'

export interface QuestionOption {
  id: string
  label: string
}

export interface PublicQuestion {
  id: string
  label: string
  type: QuestionType
  required: boolean
  options: QuestionOption[]
}

export interface PublicSurvey {
  id: string
  title: string
  description: string | null
  starts_at: string
  ends_at: string
  questions: PublicQuestion[]
}

export interface AnswerSubmission {
  question_id: string
  option_ids?: string[]
  text?: string
}

export interface SurveySubmission {
  answers: AnswerSubmission[]
  contact?: {
    email?: string
  }
}

export interface Receipt {
  receipt_id: string
  submitted_at: string
}

export type SurveyStatus = 'draft' | 'active' | 'closed' | 'archived'

export interface AdminSurvey extends PublicSurvey {
  status: SurveyStatus
  response_count: number
  created_at: string
  updated_at: string
}

export interface SurveyOptionResult {
  option_id: string
  label: string
  request_count: number
  rank: number
}

export interface SurveyResults {
  response_count: number
  options: SurveyOptionResult[]
}

export type FulfillmentStatus =
  | 'new'
  | 'reviewing'
  | 'prioritized'
  | 'in_progress'
  | 'fulfilled'
  | 'rejected'

export interface FulfillmentUpdate {
  status: FulfillmentStatus
  priority?: number
  note?: string
  repository_id?: string | null
}

export interface Fulfillment extends FulfillmentUpdate {
  option_id: string
  updated_at: string
}

export interface AuditLog {
  id: string
  admin_subject: string
  action: string
  target_type: string
  target_id: string
  details?: Record<string, unknown>
  created_at: string
}

export interface WorkerAnalytics {
  interval_seconds: number
  batch_size: number
  last_run_at?: string
  processed_events_count?: number
}
