package account

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
)

type User struct {
	Name   string
	Values map[string]interface{}

	store             AccountStore
	challengeProvider ChallengeProvider
}

func (u *User) UpdateChallenge(password string) {
	s, ok := u.Values["_salt"]
	var salt []byte
	if !ok {
		salt = u.challengeProvider.RandomSalt()
		u.Values["_salt"] = salt
	} else {
		salt = s.([]byte)
	}
	key := u.challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	u.Values["_challenge"] = u.challengeProvider.Challenge(challengeMessage, key)
	u.Save()
}

func (u *User) Check(password string) bool {
	s, ok := u.Values["_salt"]
	if !ok {
		return false
	}
	salt := s.([]byte)
	key := u.challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	newChallenge := u.challengeProvider.Challenge(challengeMessage, key)
	return bytes.Equal(newChallenge, u.Values["_challenge"].([]byte))
}

func (u *User) Save() error {
	return u.store.Save(u)
}

type AccountStore interface {
	Get(string) *User
	Create(string) *User
	Save(*User) error
}

type ChallengeProvider interface {
	DeriveKey(string, []byte) []byte
	RandomSalt() []byte
	Challenge(message []byte, key []byte) []byte
}

type FilesystemStore struct {
	path              string
	challengeProvider ChallengeProvider
}

func (f *FilesystemStore) Get(name string) *User {
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
		} else {
			panic(err)
		}
	} else {
		if !os.IsNotExist(err) {
			panic(err)
		}
		return nil
	}

	if user == nil {
		return nil
	}
	user.store = f
	user.challengeProvider = f.challengeProvider
	return user
}

func (f *FilesystemStore) Create(name string) *User {
	userPath := filepath.Join(f.path, name)
	_, err := os.Stat(userPath)
	if !os.IsNotExist(err) {
		return nil
	}

	return &User{
		Name:              name,
		Values:            make(map[string]interface{}),
		store:             f,
		challengeProvider: f.challengeProvider,
	}
}

func (f *FilesystemStore) Save(user *User) error {
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

func NewFilesystemStore(path string, challengeProvider ChallengeProvider) *FilesystemStore {
	return &FilesystemStore{path, challengeProvider}
}
