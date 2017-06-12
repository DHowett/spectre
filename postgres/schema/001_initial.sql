-- Pastes

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

CREATE FUNCTION proc_notify_paste_delete()
	RETURNS trigger
	IMMUTABLE
	AS
	$$
	BEGIN
		PERFORM pg_notify('paste.deleted', OLD.id);
		RETURN OLD;
	END;
	$$
	LANGUAGE plpgsql;

CREATE TRIGGER paste_deleted
	AFTER DELETE ON pastes
	FOR EACH ROW
	EXECUTE PROCEDURE proc_notify_paste_delete();

CREATE VIEW view_active_pastes
	AS (
		SELECT * FROM pastes WHERE expire_at > now() OR expire_at IS NULL
	);

CREATE FUNCTION proc_delete_invalid_paste()
	RETURNS trigger
	AS
	$$
	BEGIN
		IF NEW.expire_at < now() THEN
			DELETE FROM pastes WHERE id = OLD.id;
			RETURN NULL;
		END IF;
		RETURN NEW;
	END
	$$
	LANGUAGE plpgsql;

CREATE FUNCTION proc_delete_invalid_paste_by_body()
	RETURNS trigger
	AS
	$$
	BEGIN
		IF octet_length(NEW.data) = 0 THEN
			DELETE FROM pastes WHERE id = OLD.paste_id;
			RETURN NULL;
		END IF;
		RETURN NEW;
	END
	$$
	LANGUAGE plpgsql;

CREATE TRIGGER paste_update_cleanup
	BEFORE UPDATE ON pastes
	FOR EACH ROW
	EXECUTE PROCEDURE proc_delete_invalid_paste();

CREATE TRIGGER paste_body_update_cleanup
	BEFORE UPDATE ON paste_bodies
	FOR EACH ROW
	EXECUTE PROCEDURE proc_delete_invalid_paste_by_body();

-- Reports

CREATE TABLE paste_reports (
	id serial NOT NULL,
	created_at timestamp with time zone,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	PRIMARY KEY(id)
);

CREATE OR REPLACE VIEW view_paste_reports_aggregated AS 
	SELECT
		paste_id,
		min(created_at) AS created_at,
		max(created_at) AS updated_at,
		count(id) AS count
	FROM paste_reports
	GROUP BY paste_id;

-- Grants

CREATE TABLE grants (
	id character varying(256) NOT NULL,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	PRIMARY KEY(id)
);

CREATE INDEX idx_grant_by_paste ON grants USING btree (paste_id);

-- Users

CREATE TABLE users (
	id serial NOT NULL,
	updated_at timestamp with time zone,
	name character varying(512) NOT NULL,
	salt bytea,
	challenge bytea,
	source integer DEFAULT 0,
	permissions bigint DEFAULT 0,
	PRIMARY KEY(id)
);

CREATE UNIQUE INDEX uix_users_name ON users USING btree (name);

CREATE TABLE user_paste_permissions (
	user_id integer REFERENCES users(id) ON DELETE CASCADE,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	permissions bigint DEFAULT 0
);

CREATE UNIQUE INDEX uix_user_paste_perm ON user_paste_permissions USING btree (user_id, paste_id);
