package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"time"

	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"golang.org/x/crypto/scrypt"
)

const USER_CACHE_MAX_ENTRIES int = 1000

type authReply struct {
	Status        string            `json:"status,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Type          string            `json:"type,omitempty"`
	ExtraData     map[string]string `json:"extra,omitempty"`
	InvalidFields []string          `json:"invalid_fields,omitempty"`
}

type AuthController struct {
	App   Application    `inject:""`
	Model model.Provider `inject:""`
}

func (ac *AuthController) loginPostHandler(w http.ResponseWriter, r *http.Request) {
	reply := &authReply{
		Status:    "invalid",
		ExtraData: make(map[string]string),
	}

	defer func() {
		enc := json.NewEncoder(w)
		enc.Encode(reply)
	}()

	var user model.User

	loginType := r.FormValue("type")
	if loginType == "username" {
		// We don't have an assertion, hope we have a username/password
		reply.Type = "username"

		username, password, confirm := r.FormValue("username"), r.FormValue("password"), r.FormValue("confirm_password")
		if username == "" || password == "" {
			reply.Reason = "invalid username or password"
			reply.InvalidFields = []string{"username", "password"}
			return
		}

		// errors here are non-fatal.
		newuser, _ := ac.Model.GetUserNamed(username)
		if newuser == nil {
			if confirm == "" {
				reply.Status = "moreinfo"
				reply.InvalidFields = []string{"confirm_password"}
				return
			}
			if password != confirm {
				reply.Reason = "passwords don't match"
				reply.InvalidFields = []string{"password", "confirm_password"}
				return
			}
			newuser, err := ac.Model.CreateUser(username)
			if err != nil {
				// TODO(DH): propagate.
				log.Error(err)
				return
			}
			newuser.UpdateChallenge(password)
			user = newuser
		} else {
			if newuser.Check(password) {
				user = newuser
			} else {
				reply.Reason = "invalid username or password"
				reply.InvalidFields = []string{"username", "password"}
			}
		}
	} else if loginType == "token" {
		// Authentication Token
		reply.Type = "token"

		token := r.FormValue("token")
		if token == "" {
			reply.Reason = "authtoken login requested but no token provided"
			reply.InvalidFields = []string{"token"}
			return
		}

		u, ok := ephStore.Get("A|U|" + token)
		if !ok {
			w.WriteHeader(http.StatusTeapot) // I'm a teapot.
			reply.Reason = "that authenticated token isn't"
			reply.InvalidFields = []string{"token"}
			return
		}
		user = u.(model.User)
	} else {
		reply.Reason = "invalid login type"
		reply.InvalidFields = []string{"type"}
		return
	}

	if user != nil {
		SetLoggedInUser(r, user)

		MigrateLegacyPermissionsForRequest(w, r)

		reply.Status = "valid"
		reply.ExtraData["username"] = user.GetName()

		if token := r.FormValue("requested_auth_token"); token != "" {
			ephStore.Put("A|U|"+token, user, 30*time.Minute)
		}
	}

	// reply serialized in defer above. just for fun.
}

func (ac *AuthController) logoutPostHandler(w http.ResponseWriter, r *http.Request) {
	SetLoggedInUser(r, nil)
}

func (ac *AuthController) tokenHandler(w http.ResponseWriter, r *http.Request) {
	authToken, _ := generateRandomBase32String(20, 32)
	ephStore.Put("A|"+authToken, true, 30*time.Minute)
	url := ac.App.GenerateURL(URLTypeAuthToken, "token", authToken)
	w.Header().Set("Location", url.String())
	w.WriteHeader(http.StatusSeeOther)
}

func (ac *AuthController) tokenPageHandler(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]
	_, ok := ephStore.Get("A|" + token)
	if !ok {
		return
	}

	user := GetLoggedInUser(r)
	if user != nil {
		ephStore.Put("A|U|"+token, user, 30*time.Minute)
	}
	templatePack.ExecutePage(w, r, "authtoken", map[string]string{"token": token})
}

func (ac *AuthController) InitRoutes(router *mux.Router) {
	router.Methods("POST").Path("/login").HandlerFunc(ac.loginPostHandler)
	router.Methods("POST").Path("/logout").HandlerFunc(ac.logoutPostHandler)
	router.Methods("GET").Path("/token").HandlerFunc(ac.tokenHandler)
	authTokenRoute :=
		router.Methods("GET").Path("/token/{token}").HandlerFunc(ac.tokenPageHandler)

	ac.App.RegisterRouteForURLType(URLTypeAuthToken, authTokenRoute)
}

func (ac *AuthController) BindViews(viewModel *views.Model) error {
	return nil
}

type AuthChallengeProvider struct{}

func (a *AuthChallengeProvider) DeriveKey(password string, salt []byte) []byte {
	if password == "" {
		return nil
	}

	key, err := scrypt.Key([]byte(password), salt, 16384, 8, 1, 32)
	if err != nil {
		panic(err)
	}

	return key
}

func (a *AuthChallengeProvider) RandomSalt() []byte {
	b, err := generateRandomBytes(32)
	if err != nil {
		panic(err)
	}
	return b
}

func (a *AuthChallengeProvider) Challenge(message []byte, key []byte) []byte {
	shaHmac := hmac.New(sha256.New, key)
	shaHmac.Write(message)
	challenge := shaHmac.Sum(nil)
	return challenge
}

type ManglingUserStore struct {
	model.Provider
}

func (m *ManglingUserStore) mangle(name string) string {
	if len(name) > 2 && name[:2] == "1$" {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return "1$" + base32Encoder.EncodeToString(sum[:])
}

func (m *ManglingUserStore) GetUserNamed(name string) (model.User, error) {
	return m.Provider.GetUserNamed(m.mangle(name))
}

func (m *ManglingUserStore) CreateUser(name string) (model.User, error) {
	return m.Provider.CreateUser(m.mangle(name))
}

/*
type CachingUserStore struct {
	account.AccountStore
	mu    sync.RWMutex
	cache *lru.Cache
}

func (c *CachingUserStore) fromCache(name string) *account.User {
	c.mu.RLock()
	var user *account.User
	if c.cache != nil {
		if u, ok := c.cache.Get(name); ok {
			user = u.(*account.User)
		}
	}
	c.mu.RUnlock()
	return user
}

func (c *CachingUserStore) putCache(name string, user *account.User) {
	c.mu.Lock()
	if c.cache == nil {
		c.cache = &lru.Cache{
			MaxEntries: USER_CACHE_MAX_ENTRIES,
		}
	}
	c.cache.Add(name, user)
	c.mu.Unlock()
}

func (c *CachingUserStore) Get(name string) *account.User {
	user := c.fromCache(name)
	if user == nil {
		user = c.AccountStore.Get(name)
		if user != nil {
			c.putCache(user.Name, user)
		}
	}
	return user
}

func (c *CachingUserStore) Create(name string) *account.User {
	user := c.fromCache(name)
	if user == nil {
		user = c.AccountStore.Create(name)
		if user != nil {
			c.putCache(user.Name, user)
		}
	}
	return user
}
*/

type PromoteFirstUserToAdminStore struct {
	model.Provider
}

func (c *PromoteFirstUserToAdminStore) CreateUser(name string) (model.User, error) {
	u, err := c.Provider.CreateUser(name)
	if err != nil {
		return u, err
	}

	if u.GetID() == 1 {
		err = u.Permissions(model.PermissionClassUser).Grant(model.UserPermissionAdmin)
		if err != nil {
			return nil, err
		}
	}
	return u, nil
}
