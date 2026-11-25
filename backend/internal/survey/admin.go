package survey

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validUUID(value string) bool { return uuidPattern.MatchString(value) }

type AdminAnswer struct {
	QuestionID string   `json:"question_id"`
	Text       *string  `json:"text,omitempty"`
	OptionIDs  []string `json:"option_ids"`
}
type AdminResponse struct {
	ID           string        `json:"id"`
	SubmittedAt  time.Time     `json:"submitted_at"`
	ContactEmail *string       `json:"contact_email"`
	Answers      []AdminAnswer `json:"answers"`
}
type FulfillmentInput struct {
	Status       string  `json:"status"`
	Priority     *int    `json:"priority"`
	Note         string  `json:"note"`
	RepositoryID *string `json:"repository_id"`
}
type Fulfillment struct {
	OptionID     string    `json:"option_id"`
	Status       string    `json:"status"`
	Priority     *int      `json:"priority"`
	Note         *string   `json:"note"`
	RepositoryID *string   `json:"repository_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type AuditEntry struct {
	ID           string          `json:"id"`
	Actor        string          `json:"actor_identifier"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   *string         `json:"resource_id"`
	Outcome      string          `json:"outcome"`
	RequestID    string          `json:"request_id"`
	SafeChanges  json.RawMessage `json:"safe_changes"`
	CreatedAt    time.Time       `json:"created_at"`
}

func validateDraft(input DraftInput) error {
	if n := len([]rune(strings.TrimSpace(input.Title))); n < 1 || n > 200 || len(input.Questions) < 1 || len(input.Questions) > 50 || !input.EndsAt.After(input.StartsAt) {
		return ErrInvalid
	}
	if input.Description != nil && len([]rune(*input.Description)) > 5000 {
		return ErrInvalid
	}
	valid := map[string]bool{"single_choice": true, "multiple_choice": true, "short_text": true, "long_text": true, "iso_proposal": true}
	for _, q := range input.Questions {
		if n := len([]rune(strings.TrimSpace(q.Label))); n < 1 || n > 500 || !valid[q.Type] || len(q.Options) > 100 {
			return ErrInvalid
		}
		if (q.Type == "single_choice" || q.Type == "multiple_choice") && len(q.Options) == 0 {
			return ErrInvalid
		}
		for _, o := range q.Options {
			if n := len([]rune(strings.TrimSpace(o.Label))); n < 1 || n > 200 {
				return ErrInvalid
			}
		}
	}
	return nil
}

func (s *Store) AdminList(ctx context.Context, status string, page, pageSize int) ([]AdminSurvey, int, error) {
	if status != "" && status != "draft" && status != "active" && status != "closed" && status != "archived" {
		return nil, 0, ErrInvalid
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM surveys WHERE ($1='' OR status=$1)`, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT s.id::text,s.title,s.description,s.starts_at,s.ends_at,s.status,s.created_at,s.updated_at,count(DISTINCT r.id) FROM surveys s LEFT JOIN survey_responses r ON r.survey_id=s.id WHERE ($1='' OR s.status=$1) GROUP BY s.id ORDER BY s.created_at DESC LIMIT $2 OFFSET $3`, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]AdminSurvey, 0)
	for rows.Next() {
		var x AdminSurvey
		if err := rows.Scan(&x.ID, &x.Title, &x.Description, &x.StartsAt, &x.EndsAt, &x.Status, &x.CreatedAt, &x.UpdatedAt, &x.ResponseCount); err != nil {
			return nil, 0, err
		}
		x.Questions = []Question{}
		items = append(items, x)
	}
	return items, total, rows.Err()
}

func (s *Store) AdminDetail(ctx context.Context, id string) (AdminSurvey, error) {
	if !validUUID(id) {
		return AdminSurvey{}, ErrNotFound
	}
	var x AdminSurvey
	err := s.pool.QueryRow(ctx, `SELECT s.id::text,s.title,s.description,s.starts_at,s.ends_at,s.status,s.created_at,s.updated_at,count(DISTINCT r.id) FROM surveys s LEFT JOIN survey_responses r ON r.survey_id=s.id WHERE s.id=$1 GROUP BY s.id`, id).Scan(&x.ID, &x.Title, &x.Description, &x.StartsAt, &x.EndsAt, &x.Status, &x.CreatedAt, &x.UpdatedAt, &x.ResponseCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, ErrNotFound
	}
	if err != nil {
		return x, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,label,type,is_required FROM survey_questions WHERE survey_id=$1 ORDER BY position`, id)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	x.Questions = make([]Question, 0)
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.Label, &q.Type, &q.Required); err != nil {
			return x, err
		}
		q.Options = make([]Option, 0)
		orows, err := s.pool.Query(ctx, `SELECT id::text,label FROM survey_options WHERE question_id=$1 ORDER BY position`, q.ID)
		if err != nil {
			return x, err
		}
		for orows.Next() {
			var o Option
			if err := orows.Scan(&o.ID, &o.Label); err != nil {
				orows.Close()
				return x, err
			}
			q.Options = append(q.Options, o)
		}
		if err := orows.Err(); err != nil {
			orows.Close()
			return x, err
		}
		orows.Close()
		x.Questions = append(x.Questions, q)
	}
	return x, rows.Err()
}

func insertQuestions(ctx context.Context, tx pgx.Tx, id string, questions []DraftQuestion) error {
	for i, q := range questions {
		var qid string
		if err := tx.QueryRow(ctx, `INSERT INTO survey_questions(survey_id,label,type,is_required,position) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, id, strings.TrimSpace(q.Label), q.Type, q.Required, i).Scan(&qid); err != nil {
			return err
		}
		for j, o := range q.Options {
			var slug any
			if strings.TrimSpace(o.Slug) != "" {
				slug = strings.TrimSpace(o.Slug)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO survey_options(question_id,label,slug,upstream_url,position) VALUES($1,$2,$3,$4,$5)`, qid, strings.TrimSpace(o.Label), slug, o.UpstreamURL, j); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) CreateDraft(ctx context.Context, input DraftInput) (AdminSurvey, error) {
	if err := validateDraft(input); err != nil {
		return AdminSurvey{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AdminSurvey{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO surveys(title,description,starts_at,ends_at) VALUES($1,$2,$3,$4) RETURNING id::text`, strings.TrimSpace(input.Title), input.Description, input.StartsAt, input.EndsAt).Scan(&id); err != nil {
		return AdminSurvey{}, err
	}
	if err := insertQuestions(ctx, tx, id, input.Questions); err != nil {
		return AdminSurvey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminSurvey{}, err
	}
	return s.AdminDetail(ctx, id)
}

func (s *Store) UpdateDraft(ctx context.Context, id string, input DraftInput) (AdminSurvey, error) {
	if !validUUID(id) {
		return AdminSurvey{}, ErrNotFound
	}
	if err := validateDraft(input); err != nil {
		return AdminSurvey{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AdminSurvey{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var status string
	var count int
	err = tx.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM survey_responses WHERE survey_id=surveys.id) FROM surveys WHERE id=$1 FOR UPDATE`, id).Scan(&status, &count)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminSurvey{}, ErrNotFound
	}
	if err != nil {
		return AdminSurvey{}, err
	}
	if status != "draft" || count > 0 {
		return AdminSurvey{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `DELETE FROM survey_options WHERE question_id IN(SELECT id FROM survey_questions WHERE survey_id=$1)`, id); err != nil {
		return AdminSurvey{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM survey_questions WHERE survey_id=$1`, id); err != nil {
		return AdminSurvey{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE surveys SET title=$2,description=$3,starts_at=$4,ends_at=$5,updated_at=now() WHERE id=$1`, id, strings.TrimSpace(input.Title), input.Description, input.StartsAt, input.EndsAt); err != nil {
		return AdminSurvey{}, err
	}
	if err := insertQuestions(ctx, tx, id, input.Questions); err != nil {
		return AdminSurvey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminSurvey{}, err
	}
	return s.AdminDetail(ctx, id)
}

func (s *Store) Transition(ctx context.Context, id, action string) (AdminSurvey, error) {
	if !validUUID(id) {
		return AdminSurvey{}, ErrNotFound
	}
	from, to := "", ""
	switch action {
	case "publish":
		from, to = "draft", "active"
	case "close":
		from, to = "active", "closed"
	case "archive":
		from, to = "closed", "archived"
	default:
		return AdminSurvey{}, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AdminSurvey{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var status string
	var start, end time.Time
	var qcount int
	err = tx.QueryRow(ctx, `SELECT status,starts_at,ends_at,(SELECT count(*) FROM survey_questions WHERE survey_id=surveys.id) FROM surveys WHERE id=$1 FOR UPDATE`, id).Scan(&status, &start, &end, &qcount)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminSurvey{}, ErrNotFound
	}
	if err != nil {
		return AdminSurvey{}, err
	}
	if status != from {
		return AdminSurvey{}, ErrConflict
	}
	if action == "publish" && (qcount == 0 || !end.After(start)) {
		return AdminSurvey{}, ErrInvalid
	}
	if _, err := tx.Exec(ctx, `UPDATE surveys SET status=$2,updated_at=now() WHERE id=$1`, id, to); err != nil {
		return AdminSurvey{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AdminSurvey{}, err
	}
	return s.AdminDetail(ctx, id)
}

func (s *Store) Results(ctx context.Context, id string) (SurveyResults, error) {
	if !validUUID(id) {
		return SurveyResults{}, ErrNotFound
	}
	var x SurveyResults
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM survey_responses WHERE survey_id=$1`, id).Scan(&x.ResponseCount); err != nil {
		return x, err
	}
	rows, err := s.pool.Query(ctx, `SELECT o.id::text,o.label,count(sel.option_id)::bigint FROM survey_options o JOIN survey_questions q ON q.id=o.question_id LEFT JOIN survey_response_selections sel ON sel.option_id=o.id WHERE q.survey_id=$1 GROUP BY o.id,o.label ORDER BY count(sel.option_id) DESC,o.label`, id)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	x.Options = make([]OptionResult, 0)
	for rows.Next() {
		var o OptionResult
		if err := rows.Scan(&o.OptionID, &o.Label, &o.RequestCount); err != nil {
			return x, err
		}
		o.Rank = len(x.Options) + 1
		x.Options = append(x.Options, o)
	}
	if err := rows.Err(); err != nil {
		return x, err
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM surveys WHERE id=$1)`, id).Scan(&exists); err != nil {
		return x, err
	}
	if !exists {
		return x, ErrNotFound
	}
	return x, nil
}

func (s *Store) Responses(ctx context.Context, id string, page, size int) ([]AdminResponse, int, error) {
	if !validUUID(id) {
		return nil, 0, ErrNotFound
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM survey_responses WHERE survey_id=$1`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.id::text,r.submitted_at,c.email FROM survey_responses r LEFT JOIN survey_response_contacts c ON c.response_id=r.id WHERE r.survey_id=$1 ORDER BY r.submitted_at DESC LIMIT $2 OFFSET $3`, id, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]AdminResponse, 0)
	for rows.Next() {
		var x AdminResponse
		var email *string
		if err := rows.Scan(&x.ID, &x.SubmittedAt, &email); err != nil {
			return nil, 0, err
		}
		if email != nil {
			masked := maskEmail(*email)
			x.ContactEmail = &masked
		}
		x.Answers = make([]AdminAnswer, 0)
		arows, err := s.pool.Query(ctx, `SELECT a.question_id::text,a.text_value FROM survey_response_answers a WHERE a.response_id=$1 ORDER BY a.question_id`, x.ID)
		if err != nil {
			return nil, 0, err
		}
		for arows.Next() {
			var a AdminAnswer
			if err := arows.Scan(&a.QuestionID, &a.Text); err != nil {
				arows.Close()
				return nil, 0, err
			}
			a.OptionIDs = make([]string, 0)
			orows, err := s.pool.Query(ctx, `SELECT option_id::text FROM survey_response_selections WHERE answer_id=(SELECT id FROM survey_response_answers WHERE response_id=$1 AND question_id=$2) ORDER BY option_id`, x.ID, a.QuestionID)
			if err != nil {
				arows.Close()
				return nil, 0, err
			}
			for orows.Next() {
				var oid string
				if err := orows.Scan(&oid); err != nil {
					orows.Close()
					arows.Close()
					return nil, 0, err
				}
				a.OptionIDs = append(a.OptionIDs, oid)
			}
			if err := orows.Err(); err != nil {
				orows.Close()
				arows.Close()
				return nil, 0, err
			}
			orows.Close()
			x.Answers = append(x.Answers, a)
		}
		if err := arows.Err(); err != nil {
			arows.Close()
			return nil, 0, err
		}
		arows.Close()
		items = append(items, x)
	}
	return items, total, rows.Err()
}

func maskEmail(value string) string {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return "***"
	}
	parts := strings.SplitN(address.Address, "@", 2)
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) > 1 {
		local = local[:1] + strings.Repeat("*", min(8, len(local)-1))
	} else {
		local = "*"
	}
	return local + "@" + parts[1]
}

func (s *Store) UpdateFulfillment(ctx context.Context, id string, input FulfillmentInput, actor string) (Fulfillment, error) {
	if !validUUID(id) || input.RepositoryID != nil && !validUUID(*input.RepositoryID) {
		return Fulfillment{}, ErrInvalid
	}
	valid := map[string]bool{"new": true, "reviewing": true, "prioritized": true, "in_progress": true, "fulfilled": true, "rejected": true}
	if !valid[input.Status] || input.Priority != nil && *input.Priority < 1 || len(input.Note) > 5000 {
		return Fulfillment{}, ErrInvalid
	}
	var out Fulfillment
	err := s.pool.QueryRow(ctx, `INSERT INTO mirror_fulfillments(option_id,status,priority,admin_note,repository_id,updated_by) SELECT id,$2,$3,NULLIF($4,''),$5,$6 FROM survey_options WHERE id=$1 ON CONFLICT(option_id) DO UPDATE SET status=EXCLUDED.status,priority=EXCLUDED.priority,admin_note=EXCLUDED.admin_note,repository_id=EXCLUDED.repository_id,updated_by=EXCLUDED.updated_by,updated_at=now() RETURNING option_id::text,status,priority,admin_note,repository_id::text,updated_at`, id, input.Status, input.Priority, input.Note, input.RepositoryID, actor).Scan(&out.OptionID, &out.Status, &out.Priority, &out.Note, &out.RepositoryID, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) Audit(ctx context.Context, actor, action, resourceType, resourceID, requestID string, changes any) error {
	var data any
	if changes != nil {
		encoded, err := json.Marshal(changes)
		if err != nil {
			return err
		}
		data = encoded
	}
	var rid any
	if resourceID != "" {
		rid = resourceID
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO audit_logs(actor_identifier,action,resource_type,resource_id,outcome,request_id,safe_changes) VALUES($1,$2,$3,$4,'success',$5,$6)`, actor, action, resourceType, rid, requestID, data)
	return err
}

func (s *Store) AuditList(ctx context.Context, actor, action, resourceID, from, to string, page, size int) ([]AuditEntry, int, error) {
	var start, end any
	if from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			return nil, 0, ErrInvalid
		}
		start = t
	}
	if to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			return nil, 0, ErrInvalid
		}
		end = t
	}
	if start != nil && end != nil && start.(time.Time).After(end.(time.Time)) {
		return nil, 0, ErrInvalid
	}
	args := []any{actor, action, resourceID, start, end}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE ($1='' OR actor_identifier=$1) AND ($2='' OR action=$2) AND ($3='' OR resource_id::text=$3) AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at <= $5)`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, (page-1)*size)
	rows, err := s.pool.Query(ctx, `SELECT id::text,COALESCE(actor_identifier,''),action,resource_type,resource_id::text,outcome,request_id,safe_changes,created_at FROM audit_logs WHERE ($1='' OR actor_identifier=$1) AND ($2='' OR action=$2) AND ($3='' OR resource_id::text=$3) AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at <= $5) ORDER BY created_at DESC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]AuditEntry, 0)
	for rows.Next() {
		var x AuditEntry
		if err := rows.Scan(&x.ID, &x.Actor, &x.Action, &x.ResourceType, &x.ResourceID, &x.Outcome, &x.RequestID, &x.SafeChanges, &x.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, x)
	}
	return items, total, rows.Err()
}
