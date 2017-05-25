# Gormrus
[![GoDoc](https://godoc.org/github.com/o1egl/gormrus?status.svg)](https://godoc.org/github.com/o1egl/gormrus)

## Overview

Gormrus is a library for integrating [Logrus](https://github.com/Sirupsen/logrus) logger with [Gorm](https://github.com/jinzhu/gorm)

## Usage

```go
    package main
    import (
        "github.com/jinzhu/gorm"
        _ "github.com/go-sql-driver/mysql"
        "github.com/o1egl/gormrus"
    )

    db, err = gorm.Open("mysql", "user:password@/dbname?charset=utf8&parseTime=True&loc=Local")
    db.LogMode(true)
    db.SetLogger(gormrus.New())
````

## Copyright, License & Contributors

Gormrus is released under the MIT license. See [LICENSE](LICENSE)