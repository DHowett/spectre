CREATE TABLE paste_reports_v2 (
	id serial NOT NULL,
	created_at timestamp with time zone,
	paste_id character varying(256) NOT NULL REFERENCES pastes(id) ON DELETE CASCADE,
	PRIMARY KEY(id)
);

CREATE OR REPLACE VIEW view_paste_reports_v2_aggregated AS 
	SELECT
		paste_id,
		min(created_at) AS created_at,
		max(created_at) AS updated_at,
		count(id) AS count
	FROM paste_reports_v2
	GROUP BY paste_id;
