package main

import (
	"bytes"
	"html/template"
	"os"

	yaml "gopkg.in/yaml.v2"
)

var _ ConfigurationService = &fileConfigurationService{}

type fileConfigurationService struct {
	files []string
}

func (fc *fileConfigurationService) LoadConfiguration() (*Configuration, error) {
	var c Configuration
	for _, file := range fc.files {
		err := fc.appendFileToConfiguration(&c, file)
		if err != nil {
			return nil, err
		}
	}
	return &c, nil
}

func (fc *fileConfigurationService) appendFileToConfiguration(c *Configuration, filename string) error {
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

func NewFileConfigurationService(files []string) ConfigurationService {
	return &fileConfigurationService{
		files: files,
	}
}
