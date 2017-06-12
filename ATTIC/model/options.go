package model

import "github.com/Sirupsen/logrus"

type OptionReceiver interface {
	SetLoggerOption(logrus.FieldLogger)
	SetDebugOption(bool)
}

type Option func(OptionReceiver) error

func FieldLoggingOption(f logrus.FieldLogger) Option {
	return func(o OptionReceiver) error {
		o.SetLoggerOption(f)
		return nil
	}
}
