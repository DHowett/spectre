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
