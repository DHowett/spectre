package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/DHowett/ghostbin/lib/templatepack"
	"github.com/DHowett/ghostbin/model"
	"github.com/golang/glog"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/scrypt"
)

const USER_CACHE_MAX_ENTRIES int = 1000

type contextKey int

const userContextKey contextKey = 0

type authReply struct {
	Status        string            `json:"status,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	Type          string            `json:"type,omitempty"`
	ExtraData     map[string]string `json:"extra,omitempty"`
	InvalidFields []string          `json:"invalid_fields,omitempty"`
}

func authLoginPostHandler(w http.ResponseWriter, r *http.Request) {
	clientSession, err := clientLongtermSessionStore.Get(r, "authentication")
	if err != nil {
		glog.Errorln(err)
	}
	serverSession, err := sessionStore.Get(r, "session")
	if err != nil {
		glog.Errorln(err)
	}

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
		newuser, _ := userStore.GetUserNamed(username)
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
			newuser, err = userStore.CreateUser(username)
			if err != nil {
				// TODO(DH): propagate.
				glog.Error(err)
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
	} else if loginType == "persona" {
		// BrowserID Assertion
		reply.Type = "persona"

		assertion := r.FormValue("assertion")
		if assertion == "" {
			reply.Reason = "persona login requested without an assertion"
			reply.InvalidFields = []string{"assertion"}
			return
		}

		audience := "https://ghostbin.com"
		if !RequestIsHTTPS(r) {
			audience = "http://localhost:8080"
		}
		verifyResponse, err := http.PostForm("https://verifier.login.persona.org/verify", url.Values{
			"assertion": {assertion},
			"audience":  {audience},
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			glog.Error("Persona Verify Request Failed: ", err)
			reply.Reason = "persona verification failed"
			reply.ExtraData["error"] = err.Error()
			return
		}
		defer verifyResponse.Body.Close()
		dec := json.NewDecoder(verifyResponse.Body)

		var verifyResponseJSON map[string]interface{}
		err = dec.Decode(&verifyResponseJSON)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			glog.Error("Persona Verify JSON Decode Failed: ", err)
			reply.Reason = "persona verification failed"
			reply.ExtraData["error"] = err.Error()
			return
		}

		if verifyResponseJSON["status"].(string) == "okay" {
			email := verifyResponseJSON["email"].(string)
			user, _ = userStore.GetUserNamed(email)
			if user == nil {
				user, err = userStore.CreateUser(email)
				if err != nil {
					glog.Error(err)
				}
			}
			user.SetPersona(true)
			reply.ExtraData["persona"] = email
		} else {
			reply.Reason = verifyResponseJSON["reason"].(string)
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
		context.Set(r, userContextKey, user)

		// TODO(DH) paste perms
		_ = serverSession
		// Attempt to aggregate user, session, and old perms.
		//pastePerms := GetPastePermissions(r)
		//user.Values["permissions"] = pastePerms
		//delete(serverSession.Values, "pastes")      // delete old perms
		//delete(serverSession.Values, "permissions") // delete new session perms

		//err := userStore.SaveUser(user)
		//if err != nil {
		//reply.Reason = "failed to save user"
		//reply.ExtraData["error"] = err.Error()
		//} else {
		reply.Status = "valid"
		reply.ExtraData["username"] = user.GetName()
		//}
		clientSession.Values["acct_id"] = user.GetID()
		err = sessions.Save(r, w)
		if err != nil {
			glog.Errorln(err)
		}

		if token := r.FormValue("requested_auth_token"); token != "" {
			ephStore.Put("A|U|"+token, user, 30*time.Minute)
		}
	}

	// reply serialized in defer above. just for fun.
}

func authLogoutPostHandler(w http.ResponseWriter, r *http.Request) {
	ses, _ := clientLongtermSessionStore.Get(r, "authentication")
	delete(ses.Values, "acct_id")
	err := sessions.Save(r, w)
	if err != nil {
		glog.Errorln(err)
	}
}

func authTokenHandler(w http.ResponseWriter, r *http.Request) {
	authToken, _ := generateRandomBase32String(20, 32)
	ephStore.Put("A|"+authToken, true, 30*time.Minute)
	url, _ := router.Get("auth_token_login").URL("token", authToken)
	w.Header().Set("Location", url.String())
	w.WriteHeader(http.StatusSeeOther)
}

func authTokenPageHandler(w http.ResponseWriter, r *http.Request) {
	token := mux.Vars(r)["token"]
	_, ok := ephStore.Get("A|" + token)
	if !ok {
		return
	}

	user := GetUser(r)
	if user != nil {
		ephStore.Put("A|U|"+token, user, 30*time.Minute)
	}
	templatePack.ExecutePage(w, r, "authtoken", map[string]string{"token": token})
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

func GetUser(r *http.Request) model.User {
	user, present := context.Get(r, userContextKey).(model.User)
	if user == nil || !present {
		ses, _ := clientLongtermSessionStore.Get(r, "authentication")
		uid, ok := ses.Values["acct_id"].(uint)
		if ok {
			var err error
			user, err = userStore.GetUserByID(uid)
			if user != nil && err == nil {
				context.Set(r, userContextKey, user)
			}
		}
	}
	return user
}

type ManglingUserStore struct {
	model.Broker
}

func (m *ManglingUserStore) mangle(name string) string {
	if len(name) > 2 && name[:2] == "1$" {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return "1$" + base32Encoder.EncodeToString(sum[:])
}

func (m *ManglingUserStore) GetUserNamed(name string) (model.User, error) {
	return m.Broker.GetUserNamed(m.mangle(name))
}

func (m *ManglingUserStore) CreateUser(name string) (model.User, error) {
	return m.Broker.CreateUser(m.mangle(name))
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
	model.Broker
}

func (c *PromoteFirstUserToAdminStore) CreateUser(name string) (model.User, error) {
	u, err := c.Broker.CreateUser(name)
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

func adminPromoteHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	user, _ := userStore.GetUserNamed(username)
	if user != nil {
		err := user.Permissions(model.PermissionClassUser).Grant(model.UserPermissionAdmin)
		if err != nil {
			SetFlash(w, "success", "Promoted "+username+".")
		} else {
			SetFlash(w, "error", "Failed to promote "+username+".")
		}
	} else {
		SetFlash(w, "error", "Couldn't find "+username+" to promote.")
	}
	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusSeeOther)
}

func init() {
	globalInit.Add(&InitHandler{
		Priority: 10,
		Name:     "auth",
		Do: func() error {
			templatePack.AddFunction("user", func(r *templatepack.Context) model.User {
				return GetUser(r.Request)
			})
			return nil
		},
	})
}
