CREATE TABLE pastes (
	id character varying(256) NOT NULL,
	created_at timestamp with time zone,
	updated_at timestamp with time zone,
	expire_at timestamp with time zone,
	title text,
	language_name character varying(128) DEFAULT 'text',
	hmac bytea,
	encryption_salt bytea,
	encryption_method integer,
	PRIMARY KEY(id)
);

CREATE TABLE paste_bodies (
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	data bytea,
	PRIMARY KEY(paste_id)
);

CREATE TABLE paste_reports (
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	count integer DEFAULT 0,
	PRIMARY KEY(paste_id)
);

CREATE TABLE grants (
	id character varying(256) NOT NULL,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	PRIMARY KEY(id)
);

CREATE INDEX idx_grant_by_paste ON grants USING btree (paste_id);

CREATE TABLE users (
	id serial NOT NULL,
	updated_at timestamp with time zone,
	name character varying(512) NOT NULL,
	salt bytea,
	challenge bytea,
	source integer,
	permissions bigint,
	PRIMARY KEY(id)
);

CREATE UNIQUE INDEX uix_users_name ON users USING btree (name);

CREATE TABLE user_paste_permissions (
	user_id integer REFERENCES users(id) ON DELETE CASCADE,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	permissions bigint DEFAULT 0
);

CREATE UNIQUE INDEX uix_user_paste_perm ON user_paste_permissions USING btree (user_id, paste_id);
