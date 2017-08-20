package main

import (
	"context"

	"github.com/Sirupsen/logrus"

	"howett.net/spectre"
)

type mockUser struct {
	ID   uint
	Name string
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
	panic("not implemented")
}

func (u *mockUser) TestChallenge(p spectre.PassphraseMaterial) (bool, error) {
	return true, nil
}

func (u *mockUser) GetPastes() ([]spectre.PasteID, error) {
	panic("not implemented")
}

func (u *mockUser) Commit() error {
	panic("not implemented")
}

func (u *mockUser) Erase() error {
	panic("not implemented")
}

func (u *mockUser) Permissions(class spectre.PermissionClass, args ...interface{}) spectre.PermissionScope {
	panic("not implemented")
}

type mockUserService struct {
	u map[string]*mockUser
}

func (m *mockUserService) GetUserNamed(_ context.Context, u string) (spectre.User, error) {
	logrus.Infof("GetUserNamed(%s)", u)
	return &mockUser{
		ID:   1,
		Name: u,
	}, nil
}

func (m *mockUserService) GetUserByID(_ context.Context, id uint) (spectre.User, error) {
	logrus.Errorf("GetUserByID(%d)", id)
	panic("not implemented")
}

func (m *mockUserService) CreateUser(_ context.Context, u string) (spectre.User, error) {
	logrus.Errorf("CreateUser(%s)", u)
	panic("not implemented")
}
