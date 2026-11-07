-- administrator_login_limits: consolidated schema through historical migration 000049.

-- migrate:structure

CREATE TABLE administrator_login_limits (
    key_hash character(64) NOT NULL,
    window_start timestamp with time zone NOT NULL,
    attempts integer NOT NULL
);

-- migrate:constraints

ALTER TABLE ONLY administrator_login_limits
    ADD CONSTRAINT administrator_login_limits_pkey PRIMARY KEY (key_hash);

CREATE INDEX idx_administrator_login_limit_expiry ON administrator_login_limits USING btree (window_start);

-- migrate:references


