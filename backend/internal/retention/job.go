package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Policy uses explicit day counts so operators can set periods to match the
// approved privacy policy. Aggregates are deliberately never removed here.
type Policy struct {
	RawEventsDays      int
	SurveyAnswersDays  int
	SurveyContactsDays int
	AuditLogsDays      int
}

type Counts struct {
	RawEvents, SurveyResponses, SurveyContacts, IdempotencyKeys, AuditLogs, SurveyResponsesBlockedByFulfillment int64
}
type Job struct {
	pool      *pgxpool.Pool
	batchSize int
}

func New(pool *pgxpool.Pool, batchSize int) (*Job, error) {
	if pool == nil || batchSize < 1 || batchSize > 100000 {
		return nil, fmt.Errorf("pool and batch size between 1 and 100000 are required")
	}
	return &Job{pool: pool, batchSize: batchSize}, nil
}

func (j *Job) Run(ctx context.Context, policy Policy) (Counts, error) {
	if policy.RawEventsDays < 1 || policy.SurveyAnswersDays < 1 || policy.SurveyContactsDays < 1 || policy.AuditLogsDays < 1 {
		return Counts{}, fmt.Errorf("all retention periods must be at least one day")
	}
	var out Counts
	var err error
	if out.RawEvents, err = j.deleteBatches(ctx, `DELETE FROM download_events_raw WHERE id IN (SELECT id FROM download_events_raw WHERE aggregated_at IS NOT NULL AND created_at < now() - make_interval(days => $2) ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED)`, policy.RawEventsDays); err != nil {
		return out, err
	}
	if out.SurveyContacts, err = j.deleteBatches(ctx, `DELETE FROM survey_response_contacts WHERE response_id IN (SELECT response_id FROM survey_response_contacts WHERE (retention_expires_at IS NOT NULL AND retention_expires_at < now()) OR (retention_expires_at IS NULL AND created_at < now() - make_interval(days => $2)) ORDER BY COALESCE(retention_expires_at, created_at),response_id LIMIT $1 FOR UPDATE SKIP LOCKED)`, policy.SurveyContactsDays); err != nil {
		return out, err
	}
	// Contact retention is independent: an old address may be removed while
	// its answer remains, and deleting a response also cascades to its contact.
	if out.IdempotencyKeys, err = j.deleteBatches(ctx, `DELETE FROM survey_idempotency_keys WHERE id IN (SELECT id FROM survey_idempotency_keys WHERE expires_at < now() ORDER BY expires_at,id LIMIT $1 FOR UPDATE SKIP LOCKED)`); err != nil {
		return out, err
	}
	if out.SurveyResponses, err = j.deleteBatches(ctx, `DELETE FROM survey_responses r WHERE r.id IN (SELECT r2.id FROM survey_responses r2 WHERE ((r2.retention_expires_at IS NOT NULL AND r2.retention_expires_at < now()) OR (r2.retention_expires_at IS NULL AND r2.submitted_at < now() - make_interval(days => $2))) AND NOT EXISTS (SELECT 1 FROM survey_response_answers a JOIN survey_proposals p ON p.answer_id=a.id JOIN mirror_fulfillments f ON f.proposal_id=p.id WHERE a.response_id=r2.id) ORDER BY COALESCE(r2.retention_expires_at, r2.submitted_at),r2.id LIMIT $1 FOR UPDATE SKIP LOCKED)`, policy.SurveyAnswersDays); err != nil {
		return out, err
	}
	if err = j.pool.QueryRow(ctx, `SELECT count(*) FROM survey_responses r WHERE ((r.retention_expires_at IS NOT NULL AND r.retention_expires_at < now()) OR (r.retention_expires_at IS NULL AND r.submitted_at < now() - make_interval(days => $1))) AND EXISTS (SELECT 1 FROM survey_response_answers a JOIN survey_proposals p ON p.answer_id=a.id JOIN mirror_fulfillments f ON f.proposal_id=p.id WHERE a.response_id=r.id)`, policy.SurveyAnswersDays).Scan(&out.SurveyResponsesBlockedByFulfillment); err != nil {
		return out, err
	}
	if out.AuditLogs, err = j.deleteBatches(ctx, `DELETE FROM audit_logs WHERE id IN (SELECT id FROM audit_logs WHERE created_at < now() - make_interval(days => $2) ORDER BY created_at,id LIMIT $1 FOR UPDATE SKIP LOCKED)`, policy.AuditLogsDays); err != nil {
		return out, err
	}
	return out, nil
}

func (j *Job) deleteBatches(ctx context.Context, query string, days ...int) (int64, error) {
	var total int64
	for {
		args := []any{j.batchSize}
		for _, day := range days {
			args = append(args, day)
		}
		result, err := j.pool.Exec(ctx, query, args...)
		if err != nil {
			return total, err
		}
		n := result.RowsAffected()
		total += n
		if n < int64(j.batchSize) {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

func Cutoff(days int) time.Time { return time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour) }
