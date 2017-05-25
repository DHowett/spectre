package rayman

import (
	"context"

	"github.com/Sirupsen/logrus"
)

type nullLogger struct{}

func (n *nullLogger) WithField(key string, value interface{}) logrus.FieldLogger {
	return n
}

func (n *nullLogger) WithFields(fields logrus.Fields) logrus.FieldLogger {
	return n
}

func (n *nullLogger) WithError(err error) logrus.FieldLogger {
	return n
}

func (*nullLogger) Debugf(format string, args ...interface{})   {}
func (*nullLogger) Infof(format string, args ...interface{})    {}
func (*nullLogger) Printf(format string, args ...interface{})   {}
func (*nullLogger) Warnf(format string, args ...interface{})    {}
func (*nullLogger) Warningf(format string, args ...interface{}) {}
func (*nullLogger) Errorf(format string, args ...interface{})   {}
func (*nullLogger) Fatalf(format string, args ...interface{})   {}
func (*nullLogger) Panicf(format string, args ...interface{})   {}
func (*nullLogger) Debug(args ...interface{})                   {}
func (*nullLogger) Info(args ...interface{})                    {}
func (*nullLogger) Print(args ...interface{})                   {}
func (*nullLogger) Warn(args ...interface{})                    {}
func (*nullLogger) Warning(args ...interface{})                 {}
func (*nullLogger) Error(args ...interface{})                   {}
func (*nullLogger) Fatal(args ...interface{})                   {}
func (*nullLogger) Panic(args ...interface{})                   {}
func (*nullLogger) Debugln(args ...interface{})                 {}
func (*nullLogger) Infoln(args ...interface{})                  {}
func (*nullLogger) Println(args ...interface{})                 {}
func (*nullLogger) Warnln(args ...interface{})                  {}
func (*nullLogger) Warningln(args ...interface{})               {}
func (*nullLogger) Errorln(args ...interface{})                 {}
func (*nullLogger) Fatalln(args ...interface{})                 {}
func (*nullLogger) Panicln(args ...interface{})                 {}

func contextWithLogger(ctx context.Context, logger logrus.FieldLogger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func ContextLogger(ctx context.Context) logrus.FieldLogger {
	logger, _ := ctx.Value(loggerKey).(logrus.FieldLogger)
	if logger == nil {
		return &nullLogger{}
	}
	return logger
}
