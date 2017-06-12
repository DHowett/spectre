package querybuilder

type Query interface{}

type UpsertQuery struct {
	Table        string
	ConflictKeys []string
	Fields       []string
}
