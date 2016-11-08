package model

type GrantID string

type Grant interface {
	GetID() GrantID
	Realize()
}
