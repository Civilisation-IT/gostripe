CREATE TABLE IF NOT EXISTS stripe_processed_sessions (
  id UUID PRIMARY KEY,
  session_id VARCHAR(255) NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  created_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_stripe_processed_sessions_session_id ON stripe_processed_sessions(session_id);
