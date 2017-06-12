package spectre

import (
	"time"

	ghtime "howett.net/spectre/internal/time"

	"github.com/Sirupsen/logrus"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	unmarshal(&s)
	parsed, err := ghtime.ParseDuration(s)
	*d = Duration(parsed)
	return err
}

type LogLevel struct {
	l *logrus.Level
}

func (l *LogLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	unmarshal(&s)
	lev, err := logrus.ParseLevel(s)
	l.l = &lev
	return err
}

func (l *LogLevel) LogrusLevel() logrus.Level {
	if l.l == nil {
		// default?
		return logrus.InfoLevel
	}
	return *l.l
}

type Configuration struct {
	Database *struct {
		Dialect    string
		Connection string
	}

	Web []struct {
		Bind string
		SSL  *struct {
			Certificate string `yaml:"cert"`
			Key         string `yaml:"key"`
		}
		Proxied bool
	}

	Logging struct {
		Components  map[string]bool
		Destination struct {
			Type string
			Path string
		}
		Level LogLevel
	}

	Application struct {
		ForceInsecureEncryption bool `yaml:"force_insecure_encryption"`
		Limits                  struct {
			PasteSize          int      `yaml:"paste_size"`
			PasteCache         int      `yaml:"paste_cache"`
			PasteMaxExpiration Duration `yaml:"paste_max_expiration"`
		}
	}
}

type ConfigurationService interface {
	LoadConfiguration() (*Configuration, error)
}
