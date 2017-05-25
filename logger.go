package gormrus

import (
	"database/sql/driver"
	"fmt"
	"github.com/Sirupsen/logrus"
	"reflect"
	"regexp"
	"time"
)

var sqlRegexp = regexp.MustCompile(`(\$\d+)|\?`)

type gormLogger struct {
	name   string
	logger *logrus.Logger
}

func (l *gormLogger) Print(values ...interface{}) {
	entry := l.logger.WithField("name", l.name)
	if len(values) > 1 {
		level := values[0]
		source := values[1]
		entry = l.logger.WithField("source", source)
		if level == "sql" {
			duration := values[2]
			// sql
			var formattedValues []interface{}
			for _, value := range values[4].([]interface{}) {
				indirectValue := reflect.Indirect(reflect.ValueOf(value))
				if indirectValue.IsValid() {
					value = indirectValue.Interface()
					if t, ok := value.(time.Time); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format(time.RFC3339)))
					} else if b, ok := value.([]byte); ok {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", string(b)))
					} else if r, ok := value.(driver.Valuer); ok {
						if value, err := r.Value(); err == nil && value != nil {
							formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
						} else {
							formattedValues = append(formattedValues, "NULL")
						}
					} else {
						formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
					}
				} else {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
				}
			}
			entry.WithField("took", duration).Debug(fmt.Sprintf(sqlRegexp.ReplaceAllString(values[3].(string), "%v"), formattedValues...))
		} else {
			entry.Error(values[2:]...)
		}
	} else {
		entry.Error(values...)
	}

}

// New Create new logger
func New() *gormLogger {
	return NewWithName("db")
}

// NewWithName Create new logger with custom name
func NewWithName(name string) *gormLogger {
	return NewWithNameAndLogger(name, logrus.StandardLogger())
}

// NewWithLogger Create new logger with custom logger
func NewWithLogger(logger *logrus.Logger) *gormLogger {
	return NewWithNameAndLogger("db", logger)
}

// NewWithNameAndLogger Create new logger with custom name and logger
func NewWithNameAndLogger(name string, logger *logrus.Logger) *gormLogger {
	return &gormLogger{name: name, logger: logger}
}
