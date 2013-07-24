package main

import (
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
	yamlFile.Read(yml)
	yamlFile.Close()
	err = goyaml.Unmarshal(yml, i)
	if err != nil {
		return err
	}

	return nil
}
