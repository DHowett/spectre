CREATE FUNCTION proc_generic_set_updated_at()
	RETURNS trigger
	AS
	$$
	BEGIN
		NEW.updated_at := now();
		RETURN NEW;
	END
	$$
	LANGUAGE plpgsql;

UPDATE pastes SET encryption_method = 0 WHERE encryption_method IS NULL;

ALTER TABLE ONLY pastes
	ALTER COLUMN created_at SET DEFAULT now(),
	ALTER COLUMN created_at SET NOT NULL,
	ALTER COLUMN updated_at SET NOT NULL,
	ALTER COLUMN encryption_method SET DEFAULT 0,
	ALTER COLUMN encryption_method SET NOT NULL
;

CREATE FUNCTION proc_update_pastes_updated_at_by_body()
	RETURNS trigger
	AS
	$$
	BEGIN
		UPDATE pastes SET updated_at = now() WHERE id = NEW.paste_id;
		RETURN NEW;
	END
	$$
	LANGUAGE plpgsql;

CREATE TRIGGER paste_updated
	BEFORE INSERT OR UPDATE ON pastes
	FOR EACH ROW
	EXECUTE PROCEDURE proc_generic_set_updated_at();

CREATE TRIGGER paste_updated_by_body
	BEFORE INSERT OR UPDATE ON paste_bodies
	FOR EACH ROW
	EXECUTE PROCEDURE proc_update_pastes_updated_at_by_body();

CREATE TRIGGER user_updated
	BEFORE INSERT OR UPDATE ON users
	FOR EACH ROW
	EXECUTE PROCEDURE proc_generic_set_updated_at();

