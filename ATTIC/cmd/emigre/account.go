package main

import (
	"encoding/gob"
	"os"
	"path/filepath"
)

type User struct {
	Name   string
	Values map[string]interface{}

	Salt      []byte
	Challenge []byte
	Persona   bool

	store *FilesystemUserStore
}

type FilesystemUserStore struct {
	path string
}

func (f *FilesystemUserStore) Get(name string) *User {
	userPath := filepath.Join(f.path, name)
	var user *User

	file, err := os.Open(userPath)
	if err == nil {
		defer file.Close()
		dec := gob.NewDecoder(file)
		var newuser User
		err = dec.Decode(&newuser)
		if err == nil {
			user = &newuser
		}
	}

	// Legacy: FS Store used to store Salt/Challenge in .Values.
	if val, ok := user.Values["_salt"].([]byte); ok {
		user.Salt = val
		delete(user.Values, "_salt")
	}

	if val, ok := user.Values["_challenge"].([]byte); ok {
		user.Challenge = val
		delete(user.Values, "_challenge")
	}

	if val, ok := user.Values["persona"].(bool); ok {
		user.Persona = val
	}

	if user == nil {
		return nil
	}
	user.store = f

	return user
}

func (f *FilesystemUserStore) Create(name string) *User {
	userPath := filepath.Join(f.path, name)
	_, err := os.Stat(userPath)
	if !os.IsNotExist(err) {
		return nil
	}

	return &User{
		Name:   name,
		Values: make(map[string]interface{}),
		store:  f,
	}
}

func (f *FilesystemUserStore) Save(user *User) error {
	userPath := filepath.Join(f.path, user.Name)
	tempPath := userPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}

	defer file.Close()
	enc := gob.NewEncoder(file)
	err = enc.Encode(user)
	if err != nil {
		return err
	}
	os.Remove(userPath)
	os.Rename(tempPath, userPath)
	return nil
}

func NewFilesystemUserStore(path string) *FilesystemUserStore {
	return &FilesystemUserStore{path}
}
