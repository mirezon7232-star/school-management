
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	CookieName  = "school_session"
	RoleAdmin   = "admin"   
	RoleStudent = "student"

	
	OwnerUsername = "rezon"

	SessionTTL = 8 * time.Hour
)

var (
	ErrInvalidLogin = errors.New("Incorrect username or password")
	ErrLocked       = errors.New("Too many failed attempts. Please try again in 15 minutes")
	ErrExists       = errors.New("This username is already taken")
	ErrNotFound     = errors.New("Account not found")
	ErrProtected    = errors.New("This account cannot be changed here")
)

type InputError struct{ Msg string }

func (e InputError) Error() string { return e.Msg }

type Session struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	StudentID int    `json:"studentId,omitempty"`
	Owner     bool   `json:"owner,omitempty"`
}

type TeacherInfo struct {
	Name     string
	Subject  string
	Username string
}

type account struct {
	Session
	subject string
	hash    []byte
}

type sessionEntry struct {
	Session
	expires time.Time
}

var (
	mu        sync.RWMutex
	accounts  = map[string]*account{}
	sessions  = map[string]sessionEntry{}
	dummyHash []byte

	usernameRe = regexp.MustCompile(`^[a-z0-9._-]{3,30}$`)
	attempts   = &limiter{m: map[string]*attempt{}}
)

func Init() error {
	pw := os.Getenv("ADMIN_PASSWORD")
	if pw == "" {
		pw = "73966"
	}

	hash, err := hashPassword(pw)
	if err != nil {
		return err
	}
	dummy, err := hashPassword("not-a-real-password")
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	dummyHash = dummy
	accounts[OwnerUsername] = &account{
		Session: Session{Username: OwnerUsername, Name: "Rezon", Role: RoleAdmin, Owner: true},
		hash:    hash,
	}
	return nil
}

func Login(username, password, ip string) (string, Session, error) {
	username = normalize(username)

	if attempts.blocked("ip:"+ip) || attempts.blocked("user:"+username) {
		return "", Session{}, ErrLocked
	}

	mu.RLock()
	acc := accounts[username]
	hash := dummyHash 
	var sess Session
	if acc != nil {
		hash = acc.hash
		sess = acc.Session
	}
	mu.RUnlock()

	err := bcrypt.CompareHashAndPassword(hash, []byte(password))
	if acc == nil || err != nil {
		attempts.fail("ip:" + ip)
		if acc != nil {
			attempts.fail("user:" + username)
		}
		return "", Session{}, ErrInvalidLogin
	}

	attempts.reset("user:" + username)

	token, err := newToken()
	if err != nil {
		return "", Session{}, err
	}

	mu.Lock()
	sessions[token] = sessionEntry{Session: sess, expires: time.Now().Add(SessionTTL)}
	purgeExpiredLocked()
	mu.Unlock()

	return token, sess, nil
}

func Logout(token string) {
	mu.Lock()
	delete(sessions, token)
	mu.Unlock()
}


func CurrentSession(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return Session{}, false
	}

	mu.RLock()
	defer mu.RUnlock()

	entry, ok := sessions[cookie.Value]
	if !ok || time.Now().After(entry.expires) {
		return Session{}, false
	}
	return entry.Session, true
}

func RequireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := CurrentSession(r); !ok {
			http.Error(w, "Please log in", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess, ok := CurrentSession(r)
		if !ok {
			http.Error(w, "Please log in", http.StatusUnauthorized)
			return
		}
		if sess.Role != RoleAdmin {
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}


func AddTeacher(name, subject, username, password string) error {
	username = normalize(username)

	if !usernameRe.MatchString(username) {
		return InputError{"Username must be 3-30 characters: letters, numbers, dot, dash or underscore"}
	}
	if len(password) < 6 || len(password) > 72 {
		return InputError{"Password must be 6 to 72 characters"}
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	if _, exists := accounts[username]; exists {
		return ErrExists
	}

	accounts[username] = &account{
		Session: Session{Username: username, Name: name, Role: RoleAdmin},
		subject: subject,
		hash:    hash,
	}
	return nil
}

func Teachers() []TeacherInfo {
	mu.RLock()
	defer mu.RUnlock()

	list := make([]TeacherInfo, 0)
	for _, acc := range accounts {
		if acc.Role == RoleAdmin && !acc.Owner {
			list = append(list, TeacherInfo{Name: acc.Name, Subject: acc.subject, Username: acc.Username})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list
}

func RemoveTeacher(username string) error {
	username = normalize(username)

	mu.Lock()
	defer mu.Unlock()

	acc, ok := accounts[username]
	if !ok || acc.Role != RoleAdmin {
		return ErrNotFound
	}
	if acc.Owner {
		return ErrProtected
	}

	delete(accounts, username)
	removeSessionsLocked(username)
	return nil
}


func AddStudentAccount(studentID int, name string) (string, string, error) {
	password, err := randomPassword(8)
	if err != nil {
		return "", "", err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", "", err
	}

	mu.Lock()
	defer mu.Unlock()

	username := fmt.Sprintf("stu%04d", studentID)
	for i := 1; accounts[username] != nil; i++ {
		username = fmt.Sprintf("stu%04d_%d", studentID, i)
	}

	accounts[username] = &account{
		Session: Session{Username: username, Name: name, Role: RoleStudent, StudentID: studentID},
		hash:    hash,
	}
	return username, password, nil
}

func RenameUser(username, name string) {
	mu.Lock()
	defer mu.Unlock()

	if acc, ok := accounts[username]; ok {
		acc.Name = name
	}
	for token, entry := range sessions {
		if entry.Username == username {
			entry.Name = name
			sessions[token] = entry
		}
	}
}

func RemoveUser(username string) {
	mu.Lock()
	defer mu.Unlock()

	if acc, ok := accounts[username]; ok && !acc.Owner {
		delete(accounts, username)
		removeSessionsLocked(username)
	}
}


func ResetPassword(username string) (string, error) {
	username = normalize(username)

	password, err := randomPassword(8)
	if err != nil {
		return "", err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}

	mu.Lock()
	defer mu.Unlock()

	acc, ok := accounts[username]
	if !ok {
		return "", ErrNotFound
	}
	if acc.Owner {
		return "", ErrProtected
	}

	acc.hash = hash
	removeSessionsLocked(username)
	return password, nil
}


func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func hashPassword(p string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}


const pwAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func randomPassword(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(pwAlphabet)))

	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = pwAlphabet[v.Int64()]
	}
	return string(out), nil
}

func removeSessionsLocked(username string) {
	for token, entry := range sessions {
		if entry.Username == username {
			delete(sessions, token)
		}
	}
}

func purgeExpiredLocked() {
	now := time.Now()
	for token, entry := range sessions {
		if now.After(entry.expires) {
			delete(sessions, token)
		}
	}
}

const (
	maxFails = 5
	window   = 15 * time.Minute
	lockout  = 15 * time.Minute
)

type attempt struct {
	count       int
	first       time.Time
	lockedUntil time.Time
}

type limiter struct {
	mu sync.Mutex
	m  map[string]*attempt
}

func (l *limiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a := l.m[key]
	return a != nil && time.Now().Before(a.lockedUntil)
}

func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	a := l.m[key]
	if a == nil || now.Sub(a.first) > window {
		a = &attempt{first: now}
		l.m[key] = a
	}

	a.count++
	if a.count >= maxFails {
		a.lockedUntil = now.Add(lockout)
	}

	if len(l.m) > 10000 { // keep memory bounded
		for k, v := range l.m {
			if now.Sub(v.first) > window && now.After(v.lockedUntil) {
				delete(l.m, k)
			}
		}
	}
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.m, key)
	l.mu.Unlock()
}
