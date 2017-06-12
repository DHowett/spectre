CREATE OR REPLACE VIEW view_active_pastes
	AS (
		SELECT * FROM pastes WHERE expire_at > now() OR expire_at IS NULL
	);
