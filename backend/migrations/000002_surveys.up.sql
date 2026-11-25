CREATE TABLE surveys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    description text,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'closed', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX surveys_status_period_idx ON surveys (status, starts_at, ends_at);

CREATE TABLE survey_questions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    survey_id uuid NOT NULL REFERENCES surveys(id) ON DELETE RESTRICT,
    label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 500),
    type text NOT NULL CHECK (type IN ('single_choice', 'multiple_choice', 'short_text', 'long_text', 'iso_proposal')),
    is_required boolean NOT NULL DEFAULT false,
    position integer NOT NULL CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (survey_id, position),
    UNIQUE (id, survey_id)
);

CREATE TABLE survey_options (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id uuid NOT NULL REFERENCES survey_questions(id) ON DELETE RESTRICT,
    label text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 200),
    slug text,
    upstream_url text,
    is_available boolean NOT NULL DEFAULT true,
    position integer NOT NULL CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (question_id, position),
    UNIQUE (id, question_id)
);

CREATE UNIQUE INDEX survey_options_slug_unique_idx ON survey_options (question_id, slug) WHERE slug IS NOT NULL;

CREATE TABLE survey_responses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    survey_id uuid NOT NULL REFERENCES surveys(id) ON DELETE RESTRICT,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    retention_expires_at timestamptz,
    UNIQUE (id, survey_id)
);

CREATE INDEX survey_responses_recent_idx ON survey_responses (survey_id, submitted_at DESC);

CREATE TABLE survey_response_contacts (
    response_id uuid PRIMARY KEY REFERENCES survey_responses(id) ON DELETE CASCADE,
    email text NOT NULL CHECK (char_length(email) <= 254),
    created_at timestamptz NOT NULL DEFAULT now(),
    retention_expires_at timestamptz
);

CREATE TABLE survey_response_answers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    survey_id uuid NOT NULL,
    response_id uuid NOT NULL,
    question_id uuid NOT NULL,
    text_value text,
    FOREIGN KEY (response_id, survey_id) REFERENCES survey_responses(id, survey_id) ON DELETE CASCADE,
    FOREIGN KEY (question_id, survey_id) REFERENCES survey_questions(id, survey_id) ON DELETE RESTRICT,
    UNIQUE (response_id, question_id),
    UNIQUE (id, question_id),
    UNIQUE (id, survey_id)
);

CREATE INDEX survey_response_answers_response_idx ON survey_response_answers (response_id);

CREATE TABLE survey_response_selections (
    answer_id uuid NOT NULL,
    question_id uuid NOT NULL,
    option_id uuid NOT NULL,
    PRIMARY KEY (answer_id, option_id),
    FOREIGN KEY (answer_id, question_id) REFERENCES survey_response_answers(id, question_id) ON DELETE CASCADE,
    FOREIGN KEY (option_id, question_id) REFERENCES survey_options(id, question_id) ON DELETE RESTRICT
);

CREATE INDEX survey_response_selections_option_idx ON survey_response_selections (option_id);

CREATE TABLE survey_idempotency_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    survey_id uuid NOT NULL REFERENCES surveys(id) ON DELETE CASCADE,
    key_hash char(64) NOT NULL,
    payload_hash char(64) NOT NULL,
    response_id uuid,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (survey_id, key_hash),
    FOREIGN KEY (response_id, survey_id) REFERENCES survey_responses(id, survey_id) ON DELETE CASCADE
);

CREATE INDEX survey_idempotency_expiry_idx ON survey_idempotency_keys (expires_at);

CREATE TABLE survey_proposals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    survey_id uuid NOT NULL,
    answer_id uuid NOT NULL,
    proposed_name text NOT NULL CHECK (char_length(proposed_name) BETWEEN 1 AND 200),
    normalized_name text NOT NULL,
    upstream_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (answer_id, survey_id) REFERENCES survey_response_answers(id, survey_id) ON DELETE CASCADE,
    UNIQUE (answer_id)
);

CREATE INDEX survey_proposals_rank_idx ON survey_proposals (survey_id, normalized_name);

CREATE TABLE mirror_fulfillments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    option_id uuid UNIQUE REFERENCES survey_options(id) ON DELETE RESTRICT,
    proposal_id uuid UNIQUE REFERENCES survey_proposals(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewing', 'prioritized', 'in_progress', 'fulfilled', 'rejected')),
    priority integer CHECK (priority IS NULL OR priority > 0),
    admin_note text,
    repository_id uuid REFERENCES repositories(id) ON DELETE RESTRICT,
    updated_by text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((option_id IS NOT NULL) <> (proposal_id IS NOT NULL))
);

CREATE INDEX mirror_fulfillments_status_priority_idx ON mirror_fulfillments (status, priority);
