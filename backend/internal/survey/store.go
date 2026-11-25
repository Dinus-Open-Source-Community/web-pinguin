package survey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("survey not found")
	ErrInvalid  = errors.New("invalid survey response")
	ErrConflict = errors.New("idempotency key conflict")
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Survey struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	Questions   []Question `json:"questions"`
}
type Question struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []Option `json:"options"`
}
type Option struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Submission struct {
	Answers []Answer `json:"answers"`
	Contact *Contact `json:"contact,omitempty"`
}
type Answer struct {
	QuestionID string   `json:"question_id"`
	OptionIDs  []string `json:"option_ids,omitempty"`
	Text       string   `json:"text,omitempty"`
}
type Contact struct {
	Email string `json:"email"`
}
type Receipt struct {
	ReceiptID   string    `json:"receipt_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type DraftInput struct {
	Title       string          `json:"title"`
	Description *string         `json:"description"`
	StartsAt    time.Time       `json:"starts_at"`
	EndsAt      time.Time       `json:"ends_at"`
	Questions   []DraftQuestion `json:"questions"`
}
type DraftQuestion struct {
	Label    string        `json:"label"`
	Type     string        `json:"type"`
	Required bool          `json:"required"`
	Options  []DraftOption `json:"options"`
}
type DraftOption struct {
	Label       string  `json:"label"`
	Slug        string  `json:"slug"`
	UpstreamURL *string `json:"upstream_url"`
}
type AdminSurvey struct {
	Survey
	Status        string    `json:"status"`
	ResponseCount int64     `json:"response_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type SurveyResults struct {
	ResponseCount int64          `json:"response_count"`
	Options       []OptionResult `json:"options"`
}
type OptionResult struct {
	OptionID     string `json:"option_id"`
	Label        string `json:"label"`
	RequestCount int64  `json:"request_count"`
	Rank         int    `json:"rank"`
}

func (s *Store) Active(ctx context.Context) (*Survey, error) {
	var result Survey
	err := s.pool.QueryRow(ctx, `SELECT id, title, description, starts_at, ends_at FROM surveys
		WHERE status = 'active' AND starts_at <= now() AND ends_at > now()
		ORDER BY created_at DESC LIMIT 1`).Scan(&result.ID, &result.Title, &result.Description, &result.StartsAt, &result.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result.Questions = make([]Question, 0)
	rows, err := s.pool.Query(ctx, `SELECT id, label, type, is_required FROM survey_questions WHERE survey_id=$1 ORDER BY position`, result.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var question Question
		if err := rows.Scan(&question.ID, &question.Label, &question.Type, &question.Required); err != nil {
			rows.Close()
			return nil, err
		}
		question.Options = make([]Option, 0)
		optionRows, err := s.pool.Query(ctx, `SELECT id, label FROM survey_options WHERE question_id=$1 AND is_available ORDER BY position`, question.ID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		for optionRows.Next() {
			var option Option
			if err := optionRows.Scan(&option.ID, &option.Label); err != nil {
				optionRows.Close()
				rows.Close()
				return nil, err
			}
			question.Options = append(question.Options, option)
		}
		if err := optionRows.Err(); err != nil {
			optionRows.Close()
			rows.Close()
			return nil, err
		}
		optionRows.Close()
		result.Questions = append(result.Questions, question)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return &result, nil
}

func (s *Store) Submit(ctx context.Context, surveyID, key string, submission Submission) (Receipt, error) {
	if len(key) < 16 || len(key) > 128 || strings.ContainsAny(key, "\r\n") {
		return Receipt{}, ErrInvalid
	}
	if len(submission.Answers) == 0 || len(submission.Answers) > 100 {
		return Receipt{}, ErrInvalid
	}
	if submission.Contact != nil {
		address, err := mail.ParseAddress(strings.TrimSpace(submission.Contact.Email))
		if err != nil || address.Address != strings.TrimSpace(submission.Contact.Email) || len(address.Address) > 254 {
			return Receipt{}, ErrInvalid
		}
	}
	payload, err := json.Marshal(submission)
	if err != nil {
		return Receipt{}, err
	}
	keyHash := sha256.Sum256([]byte(key))
	payloadHash := sha256.Sum256(payload)
	keyDigest, payloadDigest := hex.EncodeToString(keyHash[:]), hex.EncodeToString(payloadHash[:])
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var start, end time.Time
	err = tx.QueryRow(ctx, `SELECT starts_at, ends_at FROM surveys WHERE id=$1 AND status='active' FOR SHARE`, surveyID).Scan(&start, &end)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, ErrNotFound
	}
	if err != nil {
		return Receipt{}, err
	}
	now := time.Now().UTC()
	if now.Before(start) || !now.Before(end) {
		return Receipt{}, ErrNotFound
	}
	var existingPayload *string
	var existingID *string
	var existingTime *time.Time
	err = tx.QueryRow(ctx, `SELECT payload_hash, response_id::text, (SELECT submitted_at FROM survey_responses WHERE id=response_id)
		FROM survey_idempotency_keys WHERE survey_id=$1 AND key_hash=$2 AND expires_at > now() FOR UPDATE`, surveyID, keyDigest).Scan(&existingPayload, &existingID, &existingTime)
	if err == nil {
		if existingPayload == nil || *existingPayload != payloadDigest {
			return Receipt{}, ErrConflict
		}
		if existingID == nil || existingTime == nil {
			return Receipt{}, fmt.Errorf("idempotency record is incomplete")
		}
		if err := tx.Commit(ctx); err != nil {
			return Receipt{}, err
		}
		return Receipt{ReceiptID: *existingID, SubmittedAt: *existingTime}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, err
	}
	// A transaction-scoped advisory lock serializes the first use of the same key.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, surveyID+":"+keyDigest); err != nil {
		return Receipt{}, err
	}
	// Recheck after acquiring the lock to cover concurrent submissions.
	err = tx.QueryRow(ctx, `SELECT payload_hash, response_id::text, (SELECT submitted_at FROM survey_responses WHERE id=response_id)
		FROM survey_idempotency_keys WHERE survey_id=$1 AND key_hash=$2 AND expires_at > now() FOR UPDATE`, surveyID, keyDigest).Scan(&existingPayload, &existingID, &existingTime)
	if err == nil {
		if existingPayload == nil || *existingPayload != payloadDigest {
			return Receipt{}, ErrConflict
		}
		if existingID == nil || existingTime == nil {
			return Receipt{}, fmt.Errorf("idempotency record is incomplete")
		}
		if err := tx.Commit(ctx); err != nil {
			return Receipt{}, err
		}
		return Receipt{ReceiptID: *existingID, SubmittedAt: *existingTime}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, err
	}
	questions, err := loadQuestions(ctx, tx, surveyID)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateSubmission(questions, submission); err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	err = tx.QueryRow(ctx, `INSERT INTO survey_responses (survey_id) VALUES ($1) RETURNING id::text, submitted_at`, surveyID).Scan(&receipt.ReceiptID, &receipt.SubmittedAt)
	if err != nil {
		return Receipt{}, err
	}
	if submission.Contact != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO survey_response_contacts (response_id,email) VALUES ($1,$2)`, receipt.ReceiptID, strings.TrimSpace(submission.Contact.Email)); err != nil {
			return Receipt{}, err
		}
	}
	for _, answer := range submission.Answers {
		question := questions[answer.QuestionID]
		var answerID string
		textValue := strings.TrimSpace(answer.Text)
		var textArg any
		if textValue != "" {
			textArg = textValue
		}
		if err := tx.QueryRow(ctx, `INSERT INTO survey_response_answers (survey_id,response_id,question_id,text_value) VALUES ($1,$2,$3,$4) RETURNING id::text`, surveyID, receipt.ReceiptID, answer.QuestionID, textArg).Scan(&answerID); err != nil {
			return Receipt{}, err
		}
		for _, optionID := range answer.OptionIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO survey_response_selections (answer_id,question_id,option_id) VALUES ($1,$2,$3)`, answerID, answer.QuestionID, optionID); err != nil {
				return Receipt{}, err
			}
		}
		if question.kind == "iso_proposal" && textValue != "" {
			upstream := normalizeUpstream(answer.Text)
			if _, err := tx.Exec(ctx, `INSERT INTO survey_proposals (survey_id,answer_id,proposed_name,normalized_name,upstream_url) VALUES ($1,$2,$3,$4,$5)`, surveyID, answerID, textValue, strings.ToLower(textValue), upstream); err != nil {
				return Receipt{}, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM survey_idempotency_keys WHERE survey_id=$1 AND key_hash=$2 AND expires_at <= now()`, surveyID, keyDigest); err != nil {
		return Receipt{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO survey_idempotency_keys (survey_id,key_hash,payload_hash,response_id,expires_at) VALUES ($1,$2,$3,$4,now()+interval '24 hours')`, surveyID, keyDigest, payloadDigest, receipt.ReceiptID); err != nil {
		return Receipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

type questionRule struct {
	kind     string
	required bool
	options  map[string]bool
}

func loadQuestions(ctx context.Context, tx pgx.Tx, surveyID string) (map[string]questionRule, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,type,is_required FROM survey_questions WHERE survey_id=$1`, surveyID)
	if err != nil {
		return nil, err
	}
	questions := make(map[string]questionRule)
	for rows.Next() {
		var id, kind string
		var required bool
		if err := rows.Scan(&id, &kind, &required); err != nil {
			rows.Close()
			return nil, err
		}
		questions[id] = questionRule{kind: kind, required: required, options: make(map[string]bool)}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	optionRows, err := tx.Query(ctx, `SELECT o.id::text,o.question_id::text FROM survey_options o JOIN survey_questions q ON q.id=o.question_id WHERE q.survey_id=$1 AND o.is_available`, surveyID)
	if err != nil {
		return nil, err
	}
	for optionRows.Next() {
		var id, questionID string
		if err := optionRows.Scan(&id, &questionID); err != nil {
			optionRows.Close()
			return nil, err
		}
		rule, ok := questions[questionID]
		if ok {
			rule.options[id] = true
			questions[questionID] = rule
		}
	}
	if err := optionRows.Err(); err != nil {
		optionRows.Close()
		return nil, err
	}
	optionRows.Close()
	return questions, nil
}

func validateSubmission(questions map[string]questionRule, submission Submission) error {
	answers := make(map[string]bool)
	for _, answer := range submission.Answers {
		rule, ok := questions[answer.QuestionID]
		if !ok || answers[answer.QuestionID] {
			return ErrInvalid
		}
		answers[answer.QuestionID] = true
		text := strings.TrimSpace(answer.Text)
		if len(text) > 5000 {
			return ErrInvalid
		}
		if rule.kind == "single_choice" && len(answer.OptionIDs) > 1 {
			return ErrInvalid
		}
		if (rule.kind == "single_choice" || rule.kind == "multiple_choice") && text != "" {
			return ErrInvalid
		}
		if rule.kind != "single_choice" && rule.kind != "multiple_choice" && len(answer.OptionIDs) > 0 {
			return ErrInvalid
		}
		if (rule.kind == "short_text" || rule.kind == "long_text" || rule.kind == "iso_proposal") && text == "" && rule.required {
			return ErrInvalid
		}
		if (rule.kind == "short_text" || rule.kind == "iso_proposal") && len([]rune(text)) > 200 {
			return ErrInvalid
		}
		if rule.kind == "single_choice" && rule.required && len(answer.OptionIDs) != 1 {
			return ErrInvalid
		}
		if rule.kind == "multiple_choice" && rule.required && len(answer.OptionIDs) == 0 {
			return ErrInvalid
		}
		seen := make(map[string]bool)
		for _, optionID := range answer.OptionIDs {
			if !rule.options[optionID] || seen[optionID] {
				return ErrInvalid
			}
			seen[optionID] = true
		}
	}
	for id, question := range questions {
		if question.required && !answers[id] {
			return ErrInvalid
		}
	}
	return nil
}

func normalizeUpstream(value string) *string {
	for _, field := range strings.Fields(value) {
		parsed, err := url.Parse(field)
		if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
			normalized := parsed.String()
			return &normalized
		}
	}
	return nil
}
