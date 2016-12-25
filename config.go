package main

import (
	"bytes"
	"os"
	"text/template"
	"time"

	"github.com/Sirupsen/logrus"

	yaml "gopkg.in/yaml.v2"
)

type yamlDuration time.Duration

func (d *yamlDuration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	unmarshal(&s)
	parsed, err := ParseDuration(s)
	*d = yamlDuration(parsed)
	return err
}

type ConfigLogLevel struct {
	l *logrus.Level
}

func (l *ConfigLogLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	unmarshal(&s)
	lev, err := logrus.ParseLevel(s)
	l.l = &lev
	return err
}

func (l *ConfigLogLevel) LogrusLevel() logrus.Level {
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
			CA          string `yaml:"ca"`
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
		Level ConfigLogLevel
	}

	Application struct {
		ForceInsecureEncryption bool `yaml:"force_insecure_encryption"`
		Limits                  struct {
			PasteSize          uint         `yaml:"paste_size"`
			PasteCache         uint         `yaml:"paste_cache"`
			PasteMaxExpiration yamlDuration `yaml:"paste_max_expiration"`
		}
	}
}

func (c *Configuration) AppendFile(filename string) error {
	tmpl, err := template.ParseFiles(filename)
	if err != nil {
		return err
	}

	tmpl.Funcs(template.FuncMap{
		"env": func(key string) (string, error) {
			return os.Getenv(key), nil
		},
	})

	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, c)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(buf.Bytes(), c)
	if err != nil {
		return err
	}

	return nil
}
