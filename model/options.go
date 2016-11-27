package model

import "github.com/Sirupsen/logrus"

type OptionReceiver interface {
	setLoggerOption(logrus.FieldLogger)
	setDebugOption(bool)
}

type Option func(OptionReceiver) error

func FieldLoggingOption(f logrus.FieldLogger) Option {
	return func(o OptionReceiver) error {
		o.setLoggerOption(f)
		return nil
	}
}
