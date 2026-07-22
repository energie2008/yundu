-- +goose Up
-- +goose StatementBegin

CREATE TABLE subscription_short_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  short_code VARCHAR(16) NOT NULL UNIQUE,
  token_id UUID NOT NULL REFERENCES subscription_tokens(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ
);
CREATE INDEX idx_sub_short_codes_code ON subscription_short_codes(short_code);
CREATE INDEX idx_sub_short_codes_token ON subscription_short_codes(token_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS subscription_short_codes;
-- +goose StatementEnd
