package main

import (
	"context"
	"crypto/subtle"
	"sync"

	"github.com/Sirupsen/logrus"

	"howett.net/spectre"
)

type mockUser struct {
	ID   uint
	Name string
	p    spectre.PassphraseMaterial
}

func (u *mockUser) GetID() uint {
	return u.ID
}

func (u *mockUser) GetName() string {
	return u.Name
}

func (u *mockUser) GetSource() spectre.UserSource {
	return 0
}

func (u *mockUser) SetSource(spectre.UserSource) {
	panic("not implemented")
}

func (u *mockUser) UpdateChallenge(p spectre.PassphraseMaterial) {
	u.p = p
}

func (u *mockUser) TestChallenge(p spectre.PassphraseMaterial) (bool, error) {
	return subtle.ConstantTimeCompare(p, u.p) == 1, nil
}

func (u *mockUser) GetPastes() ([]spectre.PasteID, error) {
	panic("not implemented")
}

func (u *mockUser) Commit() error {
	return nil // All changes are committed immediately in this implementation
}

func (u *mockUser) Erase() error {
	panic("not implemented")
}

func (u *mockUser) Permissions(class spectre.PermissionClass, args ...interface{}) spectre.PermissionScope {
	panic("not implemented")
}

type mockUserService struct {
	u map[string]*mockUser
	o sync.Once
}

func (m *mockUserService) init() {
	m.o.Do(func() {
		m.u = map[string]*mockUser{
			"test": &mockUser{
				ID:   1,
				Name: "test",
				p:    spectre.PassphraseMaterial("password"),
			},
		}
	})
}

func (m *mockUserService) GetUserNamed(_ context.Context, u string) (spectre.User, error) {
	m.init()
	logrus.Infof("GetUserNamed(%s)", u)
	if us, ok := m.u[u]; ok {
		return us, nil
	}
	return nil, spectre.ErrNotFound
}

func (m *mockUserService) GetUserByID(_ context.Context, id uint) (spectre.User, error) {
	logrus.Errorf("GetUserByID(%d)", id)
	panic("not implemented")
}

func (m *mockUserService) CreateUser(_ context.Context, u string) (spectre.User, error) {
	logrus.Errorf("CreateUser(%s)", u)
	panic("not implemented")
}
