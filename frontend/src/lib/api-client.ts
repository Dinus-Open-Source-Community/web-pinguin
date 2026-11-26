import type {
  AdminSurvey,
  AuditLog,
  ErrorResponse,
  Fulfillment,
  FulfillmentUpdate,
  HealthResponse,
  ListingResponse,
  MirrorTargetStatus,
  PaginatedResponse,
  PublicSurvey,
  ReadinessResponse,
  Receipt,
  RegionAggregate,
  RepositoryDetail,
  RepositorySummary,
  StatsPeriod,
  StatsSummary,
  SurveyResults,
  SurveyStatus,
  SurveySubmission,
  TimeseriesPoint,
  WorkerAnalytics,
} from '#/types/api'

export class ApiError extends Error {
  public status: number
  public code?: string
  public requestId?: string

  constructor(status: number, message: string, code?: string, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

const ADMIN_TOKEN_KEY = 'pinguin_admin_jwt'

export function getAdminToken(): string | null {
  if (typeof window === 'undefined') return null
  return sessionStorage.getItem(ADMIN_TOKEN_KEY) || localStorage.getItem(ADMIN_TOKEN_KEY)
}

export function setAdminToken(token: string, remember = false): void {
  if (typeof window === 'undefined') return
  if (remember) {
    localStorage.setItem(ADMIN_TOKEN_KEY, token)
  } else {
    sessionStorage.setItem(ADMIN_TOKEN_KEY, token)
  }
}

export function clearAdminToken(): void {
  if (typeof window === 'undefined') return
  sessionStorage.removeItem(ADMIN_TOKEN_KEY)
  localStorage.removeItem(ADMIN_TOKEN_KEY)
}

interface RequestOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined | null>
  requireAdmin?: boolean
}

async function request<T>(endpoint: string, options: RequestOptions = {}): Promise<T> {
  const { params, requireAdmin = false, headers: customHeaders, ...init } = options

  // Build full URL. In browser, relative URL /api/v1/... routes cleanly through Caddy Gateway or Vite proxy.
  let url = endpoint
  if (params) {
    const searchParams = new URLSearchParams()
    for (const [key, val] of Object.entries(params)) {
      if (val !== undefined && val !== null && val !== '') {
        searchParams.append(key, String(val))
      }
    }
    const query = searchParams.toString()
    if (query) {
      url += (url.includes('?') ? '&' : '?') + query
    }
  }

  const headers = new Headers(customHeaders)
  if (!headers.has('Accept')) {
    headers.set('Accept', 'application/json')
  }

  if (requireAdmin) {
    const token = getAdminToken()
    if (!token) {
      throw new ApiError(401, 'Admin token required', 'UNAUTHORIZED')
    }
    headers.set('Authorization', `Bearer ${token}`)
  }

  if (init.body && typeof init.body === 'string' && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(url, { ...init, headers })

  if (!response.ok) {
    let errorCode: string | undefined
    let errorMessage = `HTTP Error ${response.status}: ${response.statusText}`
    let requestId: string | undefined

    try {
      const errorJson = (await response.json()) as ErrorResponse
      if (errorJson?.error) {
        errorCode = errorJson.error.code
        errorMessage = errorJson.error.message || errorMessage
        requestId = errorJson.error.request_id
      }
    } catch {
      // Non-JSON error body fallback
    }

    throw new ApiError(response.status, errorMessage, errorCode, requestId)
  }

  // Handle empty responses (204 No Content)
  if (response.status === 204) {
    return null as unknown as T
  }

  return (await response.json()) as T
}

export const api = {
  // Health
  health: {
    live: () => request<HealthResponse>('/health/live'),
    ready: () => request<ReadinessResponse>('/health/ready'),
  },

  // Repositories & Catalog
  repositories: {
    list: (params?: {
      page?: number
      page_size?: number
      category?: string
      search?: string
      sort?: string
    }) => request<PaginatedResponse<RepositorySummary>>('/api/v1/repositories', { params }),
    getById: (id: string) => request<RepositoryDetail>(`/api/v1/repositories/${id}`),
  },

  // File Listing & Streaming Download
  listing: {
    get: (path?: string) => request<ListingResponse>('/api/v1/listing', { params: { path } }),
    getDownloadUrl: (path: string) => `/api/v1/download?path=${encodeURIComponent(path)}`,
  },

  // Statistics
  stats: {
    summary: (period: StatsPeriod = '24h') =>
      request<StatsSummary>('/api/v1/stats/summary', { params: { period } }),
    timeseries: (period: StatsPeriod = '24h') =>
      request<TimeseriesPoint[]>('/api/v1/stats/timeseries', { params: { period } }),
    regions: (params?: { period?: StatsPeriod; repository_id?: string }) =>
      request<RegionAggregate[]>('/api/v1/stats/regions', { params }),
  },

  // Mirror Health
  mirrors: {
    status: () => request<MirrorTargetStatus[]>('/api/v1/mirrors/status'),
  },

  // Surveys (Public)
  surveys: {
    getActive: () => request<PublicSurvey | null>('/api/v1/surveys/active'),
    submitResponse: (surveyId: string, submission: SurveySubmission) =>
      request<Receipt>(`/api/v1/surveys/${surveyId}/responses`, {
        method: 'POST',
        body: JSON.stringify(submission),
      }),
  },

  // Admin Module
  admin: {
    surveys: {
      list: (params?: { page?: number; page_size?: number; status?: SurveyStatus }) =>
        request<PaginatedResponse<AdminSurvey>>('/api/v1/admin/surveys', {
          params,
          requireAdmin: true,
        }),
      create: (data: unknown) =>
        request<AdminSurvey>('/api/v1/admin/surveys', {
          method: 'POST',
          body: JSON.stringify(data),
          requireAdmin: true,
        }),
      getById: (id: string) =>
        request<AdminSurvey>(`/api/v1/admin/surveys/${id}`, { requireAdmin: true }),
      updateStatus: (id: string, status: SurveyStatus) =>
        request<AdminSurvey>(`/api/v1/admin/surveys/${id}/status`, {
          method: 'PATCH',
          body: JSON.stringify({ status }),
          requireAdmin: true,
        }),
      getResponses: (id: string, params?: { page?: number; page_size?: number }) =>
        request<PaginatedResponse<unknown>>(`/api/v1/admin/surveys/${id}/responses`, {
          params,
          requireAdmin: true,
        }),
      getResults: (id: string) =>
        request<SurveyResults>(`/api/v1/admin/surveys/${id}/results`, { requireAdmin: true }),
    },
    fulfillment: {
      update: (optionId: string, update: FulfillmentUpdate) =>
        request<Fulfillment>(`/api/v1/admin/survey-options/${optionId}/fulfillment`, {
          method: 'PATCH',
          body: JSON.stringify(update),
          requireAdmin: true,
        }),
    },
    auditLogs: (params?: { page?: number; page_size?: number; action?: string }) =>
      request<PaginatedResponse<AuditLog>>('/api/v1/admin/audit-logs', {
        params,
        requireAdmin: true,
      }),
    workerAnalytics: () =>
      request<WorkerAnalytics>('/api/v1/admin/workers/analytics', { requireAdmin: true }),
  },
}
