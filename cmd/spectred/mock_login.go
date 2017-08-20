package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"

	"howett.net/spectre"
)

type mockLoginService struct {
	UserService spectre.UserService
}

func (m *mockLoginService) GetLoggedInUser(r *http.Request) spectre.User {
	logrus.Infof("GetLoggedInUser(... %v ...)", r.URL)
	cookie, _ := r.Cookie("uid")
	logrus.Infof("<- Cookies: %v", r.Cookies())
	uid, _ := strconv.ParseUint(cookie.Value, 10, 0)
	u, _ := m.UserService.GetUserByID(context.Background(), uint(uid))
	logrus.Infof("-> %+v", u)
	return u
}

func (m *mockLoginService) SetLoggedInUser(w http.ResponseWriter, r *http.Request, u spectre.User) {
	logrus.Infof("SetLoggedInUser(... %v ..., %v)", r.URL, u)
	if u != nil {
		http.SetCookie(w, &http.Cookie{
			Name:  "uid",
			Value: fmt.Sprintf("%d", u.GetID()),
		})
	} else {
		http.SetCookie(w, &http.Cookie{
			Name: "uid",
		})
	}
}
