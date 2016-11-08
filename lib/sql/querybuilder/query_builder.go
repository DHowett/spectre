package querybuilder

type QueryBuilder interface {
	Build(Query) (string, error)
}

func New(dialect string) QueryBuilder {
	switch dialect {
	case "sqlite3", "sqlite":
		return &sqliteQueryBuilder{}
	case "postgres", "pgsql", "postgresql":
		return &postgresQueryBuilder{}
	default:
		return nil
	}
}
