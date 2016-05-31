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

	Salt      []byte
	Challenge []byte

	store             AccountStore
	challengeProvider ChallengeProvider
}

func (u *User) UpdateChallenge(password string) {
	salt := u.Salt
	if salt == nil {
		salt = u.challengeProvider.RandomSalt()
		u.Salt = salt
	}
	key := u.challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	u.Challenge = u.challengeProvider.Challenge(challengeMessage, key)
	u.Save()
}

func (u *User) Check(password string) bool {
	salt := u.Salt
	if salt == nil {
		return false
	}
	key := u.challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	newChallenge := u.challengeProvider.Challenge(challengeMessage, key)
	return bytes.Equal(newChallenge, u.Challenge)
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
