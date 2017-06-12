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
		SELECT * FROM pastes WHERE expire_at > now()
	);
