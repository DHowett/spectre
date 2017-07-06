package main

import (
	"fmt"

	gohttp "net/http"

	"github.com/Sirupsen/logrus"
	"howett.net/spectre"
)

// MOCK ONLY

type loggingPermitter struct{}
type loggingPermissionScope struct {
	c spectre.PermissionClass
	a []interface{}
}

func (b loggingPermitter) GetPermitterForRequest(r *gohttp.Request) spectre.Permitter {
	logrus.Infof("GetPermitterForRequest( ... %v ... )", r.URL)
	return b
}

func (b loggingPermitter) Permissions(class spectre.PermissionClass, args ...interface{}) spectre.PermissionScope {
	logrus.Infof("Permissions(%x, %v)", class, args)
	return &loggingPermissionScope{class, args}
}

func (bp *loggingPermissionScope) String() string {
	return fmt.Sprintf("%x(%v)", bp.c, bp.a)
}

func (bp *loggingPermissionScope) Has(p spectre.Permission) bool {
	logrus.Infof("Has(%x) in %v", p, bp)
	return true
}

func (bp *loggingPermissionScope) Grant(p spectre.Permission) error {
	logrus.Infof("Grant(%x) on %v", p, bp)
	return nil
}

func (bp *loggingPermissionScope) Revoke(p spectre.Permission) error {
	logrus.Infof("Revoke(%x) on %v", p, bp)
	return nil
}
