package main

import (
	"io"
	"launchpad.net/goyaml"
	"os"
)

func YAMLUnmarshalFile(filename string, i interface{}) error {
	yamlFile, err := os.Open(filename)
	if err != nil {
		return err
	}

	fi, err := yamlFile.Stat()
	if err != nil {
		return err
	}

	yml := make([]byte, fi.Size())
	io.ReadFull(yamlFile, yml)
	yamlFile.Close()
	err = goyaml.Unmarshal(yml, i)
	if err != nil {
		return err
	}

	return nil
}
