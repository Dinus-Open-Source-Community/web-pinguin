// retention-smoke verifies deletion boundaries against a dedicated test database.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/retention"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	fmt.Println("retention boundary smoke passed")
}

func run() error {
	db, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, db)
	if err != nil {
		return err
	}
	defer pool.Close()
	suffix, err := randomHex(12)
	if err != nil {
		return err
	}
	surveyID, err := randomUUID()
	if err != nil {
		return err
	}
	oldResponse, err := randomUUID()
	if err != nil {
		return err
	}
	freshResponse, err := randomUUID()
	if err != nil {
		return err
	}
	expiredOverrideResponse, err := randomUUID()
	if err != nil {
		return err
	}
	freshOverrideResponse, err := randomUUID()
	if err != nil {
		return err
	}
	fulfilledResponse, err := randomUUID()
	if err != nil {
		return err
	}
	questionID, err := randomUUID()
	if err != nil {
		return err
	}
	answerID, err := randomUUID()
	if err != nil {
		return err
	}
	proposalID, err := randomUUID()
	if err != nil {
		return err
	}
	expiredKey, err := randomUUID()
	if err != nil {
		return err
	}
	freshKey, err := randomUUID()
	if err != nil {
		return err
	}
	oldAudit, err := randomUUID()
	if err != nil {
		return err
	}
	freshAudit, err := randomUUID()
	if err != nil {
		return err
	}
	oldEvent, err := randomUUID()
	if err != nil {
		return err
	}
	pendingEvent, err := randomUUID()
	if err != nil {
		return err
	}
	bucket := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Hour)
	filePrefix := "retention-smoke/" + suffix
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `INSERT INTO surveys(id,title,starts_at,ends_at,status) VALUES($1,'Retention smoke',now()-interval '1 day',now()+interval '1 day','closed')`, surveyID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_responses(id,survey_id,submitted_at) VALUES($1,$6,now()-interval '2 days'),($2,$6,now()),($3,$6,now()),($4,$6,now()-interval '2 days'),($5,$6,now()-interval '2 days')`, oldResponse, freshResponse, expiredOverrideResponse, freshOverrideResponse, fulfilledResponse, surveyID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE survey_responses SET retention_expires_at=CASE WHEN id=$1 THEN now()-interval '1 second' WHEN id=$2 THEN now()+interval '1 day' END WHERE id IN ($1,$2)`, expiredOverrideResponse, freshOverrideResponse); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE survey_responses SET retention_expires_at=now()-interval '1 second' WHERE id=$1`, fulfilledResponse); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_response_contacts(response_id,email,created_at) VALUES($1,'retention-smoke@example.invalid',now()-interval '2 days'),($2,'retention-smoke@example.invalid',now()),($3,'retention-smoke@example.invalid',now()),($4,'retention-smoke@example.invalid',now()-interval '2 days'),($5,'retention-smoke@example.invalid',now()-interval '2 days')`, oldResponse, freshResponse, expiredOverrideResponse, freshOverrideResponse, fulfilledResponse); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE survey_response_contacts SET retention_expires_at=CASE WHEN response_id=$1 THEN now()-interval '1 second' WHEN response_id=$2 THEN now()+interval '1 day' END WHERE response_id IN ($1,$2)`, expiredOverrideResponse, freshOverrideResponse); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_questions(id,survey_id,label,type,position) VALUES($1,$2,'Requested ISO','short_text',0)`, questionID, surveyID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_response_answers(id,survey_id,response_id,question_id,text_value) VALUES($1,$2,$3,$4,'Old request')`, answerID, surveyID, fulfilledResponse, questionID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_proposals(id,survey_id,answer_id,proposed_name,normalized_name) VALUES($1,$2,$3,'Old requested ISO','old requested iso')`, proposalID, surveyID, answerID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mirror_fulfillments(proposal_id) VALUES($1)`, proposalID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO survey_idempotency_keys(id,survey_id,key_hash,payload_hash,expires_at) VALUES($1,$3,$4,$5,now()-interval '1 second'),($2,$3,$6,$5,now()+interval '1 day')`, expiredKey, freshKey, surveyID, hash(suffix+"a"), hash(suffix+"b"), hash(suffix+"c")); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(id,actor_identifier,action,resource_type,outcome,request_id,created_at) VALUES($1,'smoke','retention_test','smoke','success',$3,now()-interval '2 days'),($2,'smoke','retention_test','smoke','success',$3,now())`, oldAudit, freshAudit, "retention-"+suffix); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO download_events_raw(id,source_key,source,occurred_at,file_path,http_status,created_at,aggregated_at) VALUES($1,$3,'portal',now()-interval '2 days',$4,200,now()-interval '2 days',now()-interval '2 days'),($2,$5,'portal',now()-interval '2 days',$4,200,now()-interval '2 days',NULL)`, oldEvent, pendingEvent, "retention-aggregated-"+suffix, filePrefix+"file.iso", "retention-pending-"+suffix); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO download_aggregates(bucket_start,bucket_size,file_path_prefix,region_code,request_count,completed_count,bytes_sent) VALUES($1,'hour',$2,'ZZ/UNKNOWN',17,16,2048)`, bucket, filePrefix); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	job, err := retention.New(pool, 1)
	if err != nil {
		return err
	}
	counts, err := job.Run(ctx, retention.Policy{RawEventsDays: 1, SurveyAnswersDays: 1, SurveyContactsDays: 1, AuditLogsDays: 1})
	if err != nil {
		return err
	}
	if counts.RawEvents != 1 || counts.SurveyContacts != 3 || counts.IdempotencyKeys != 1 || counts.SurveyResponses != 2 || counts.SurveyResponsesBlockedByFulfillment != 1 || counts.AuditLogs != 1 {
		return fmt.Errorf("unexpected deletion counts: %+v", counts)
	}
	checks := []struct {
		name, query string
		args        []any
		want        int
	}{
		{"aggregated raw removed", `SELECT count(*) FROM download_events_raw WHERE id=$1`, []any{oldEvent}, 0},
		{"unaggregated raw retained", `SELECT count(*) FROM download_events_raw WHERE id=$1`, []any{pendingEvent}, 1},
		{"fresh response retained", `SELECT count(*) FROM survey_responses WHERE id=$1`, []any{freshResponse}, 1},
		{"expired response removed", `SELECT count(*) FROM survey_responses WHERE id=$1`, []any{oldResponse}, 0},
		{"expired response override removed", `SELECT count(*) FROM survey_responses WHERE id=$1`, []any{expiredOverrideResponse}, 0},
		{"fresh response override retained", `SELECT count(*) FROM survey_responses WHERE id=$1`, []any{freshOverrideResponse}, 1},
		{"fulfilled response retained", `SELECT count(*) FROM survey_responses WHERE id=$1`, []any{fulfilledResponse}, 1},
		{"fulfilled response contact expired separately", `SELECT count(*) FROM survey_response_contacts WHERE response_id=$1`, []any{fulfilledResponse}, 0},
		{"fresh contact retained", `SELECT count(*) FROM survey_response_contacts WHERE response_id=$1`, []any{freshResponse}, 1},
		{"fresh contact override retained", `SELECT count(*) FROM survey_response_contacts WHERE response_id=$1`, []any{freshOverrideResponse}, 1},
		{"expired idempotency key removed", `SELECT count(*) FROM survey_idempotency_keys WHERE id=$1`, []any{expiredKey}, 0},
		{"fresh idempotency key retained", `SELECT count(*) FROM survey_idempotency_keys WHERE id=$1`, []any{freshKey}, 1},
		{"fresh audit retained", `SELECT count(*) FROM audit_logs WHERE id=$1`, []any{freshAudit}, 1},
		{"old audit removed", `SELECT count(*) FROM audit_logs WHERE id=$1`, []any{oldAudit}, 0},
		{"aggregate retained", `SELECT count(*) FROM download_aggregates WHERE file_path_prefix=$1 AND request_count=17`, []any{filePrefix}, 1},
	}
	for _, check := range checks {
		var got int
		if err := pool.QueryRow(ctx, check.query, check.args...).Scan(&got); err != nil {
			return err
		}
		if got != check.want {
			return fmt.Errorf("retention check failed: %s", check.name)
		}
	}
	second, err := job.Run(ctx, retention.Policy{RawEventsDays: 1, SurveyAnswersDays: 1, SurveyContactsDays: 1, AuditLogsDays: 1})
	if err != nil {
		return err
	}
	if second.RawEvents != 0 || second.SurveyContacts != 0 || second.IdempotencyKeys != 0 || second.SurveyResponses != 0 || second.SurveyResponsesBlockedByFulfillment != 1 || second.AuditLogs != 0 {
		return fmt.Errorf("retention rerun was not idempotent: %+v", second)
	}
	return nil
}

func randomUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func hash(s string) string { value := sha256.Sum256([]byte(s)); return hex.EncodeToString(value[:]) }
