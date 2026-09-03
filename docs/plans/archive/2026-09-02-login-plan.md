# Login and Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task inline. Steps use checkbox (`- [ ]`) syntax for tracking. Do not use multi-agent workflows in this repo.

**Goal:** Replace the build-time API key with real user accounts, argon2id password hashing, server-issued session cookies, admin/viewer roles, and a first-run setup flow — across the Go daemon and the React dashboard.

**Architecture:** A new `internal/auth` package owns password hashing, tokens, roles and the one-time setup token, with no HTTP or storage dependency. `internal/storage` gains `users` and `sessions` tables in their own files. `internal/server` gets a `routes()` builder (extracted from `Start()` so handlers become testable), a `requireAuth`/`requireAdmin` middleware pair that accepts *either* a session cookie *or* the existing `X-API-Key` machine token, and handlers under `/api/auth/*` and `/api/users`. The dashboard wraps its routes in an `AuthProvider` state machine (`loading → setup | anon | authed`) with `/login` and `/setup` pages.

**Tech Stack:** Go 1.25, `golang.org/x/crypto/argon2`, `database/sql` + `mattn/go-sqlite3`, `net/http` 1.22 pattern routing; React 19, react-router v7, shadcn/ui primitives on Radix, Vitest + Testing Library.

**Spec:** `docs/features/2026-09-02-login-design.md`

## Global Constraints

- Go: `GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race` must stay green; `golangci-lint` at 0 issues (config in `.golangci.yml`).
- Web: `npm run typecheck && npm test && npm run build` must stay green; `npm run build` runs `verify:css`.
- Never log a password or session token. The one-time setup token is the single, deliberate exception (spec §Bootstrap).
- All timestamps stored as UTC RFC 3339 strings, matching the existing `metrics` fix.
- Secrets compared with `crypto/subtle.ConstantTimeCompare`; random bytes only from `crypto/rand`.
- Cookie name `sys_session`; `HttpOnly`; `SameSite=Strict`; `Path=/`; `Secure` only when `r.TLS != nil` or `X-Forwarded-Proto: https`.
- Password policy: 12–128 characters, counted as runes. No composition rules.
- Generic login failure body: `{"error":"invalid email or password"}` for both wrong password and unknown email.
- Commits: conventional, imperative subjects. **No `Co-Authored-By` lines** (project rule; single human contributor). Commit only when the user asks — the commit steps below document the intended boundaries.

## File map

| File | Responsibility |
|---|---|
| `internal/auth/password.go` | argon2id hash/verify, PHC encoding, policy, timing-equalising dummy |
| `internal/auth/token.go` | session tokens, token hashing, IDs |
| `internal/auth/user.go` | `Role`, `User`, email normalisation |
| `internal/auth/setup.go` | one-time, single-use setup token |
| `internal/storage/users.go` | `users` table + CRUD |
| `internal/storage/sessions.go` | `sessions` table + CRUD + prune |
| `internal/config/config.go` | `server.insecure`, `auth.*` |
| `internal/server/session.go` | cookie issue/clear, principal resolution, request-context principal |
| `internal/server/auth.go` | `requireAuth`, `requireAdmin`, CSRF guard (keeps `AuthMiddleware` for the agent key) |
| `internal/server/auth_handlers.go` | setup, login, logout, me, change-password |
| `internal/server/user_handlers.go` | list/create/delete users |
| `internal/server/server.go` | `routes()` extracted from `Start()`; `WithAuth` |
| `cmd/daemon/main.go` | bootstrap token, `WithAuth`, session prune |
| `web/constants.ts` | same-origin URLs, no key |
| `web/services/api.ts` | `credentials`, 401 hook, auth + user calls |
| `web/hooks/useAuth.tsx` | `AuthProvider`, `useAuth` |
| `web/components/RequireAuth.tsx` | route guard |
| `web/pages/Login.tsx`, `web/pages/Setup.tsx` | forms |
| `web/components/ui/label.tsx`, `ui/dropdown-menu.tsx` | shadcn primitives |
| `web/components/UserMenu.tsx`, `AppShell.tsx`, `App.tsx` | header menu, routing |
| `web/pages/Settings.tsx` | Users + Account cards |

---

### Task 1: argon2id password hashing

**Files:**
- Create: `internal/auth/password.go`
- Test: `internal/auth/password_test.go`

**Interfaces:**
- Produces: `HashPassword(password string) (string, error)`, `VerifyPassword(encoded, password string) (bool, error)`, `ValidatePassword(password string) error`, `VerifyDummy(password string)`, errors `ErrPasswordTooShort`, `ErrPasswordTooLong`, `ErrMalformedHash`, consts `MinPasswordLength = 12`, `MaxPasswordLength = 128`.

- [ ] **Step 1: Add the dependency**

Run: `cd /home/xyfo/personal_projects/SysSentient && GOTOOLCHAIN=auto go get golang.org/x/crypto@latest`
Expected: `go.mod` gains `golang.org/x/crypto vX.Y.Z` as a direct requirement.

- [ ] **Step 2: Write the failing tests**

```go
package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword(encoded, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected the original password to verify")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	encoded, _ := HashPassword("correct horse battery staple")
	ok, err := VerifyPassword(encoded, "correct horse battery stapl3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	t.Parallel()
	a, _ := HashPassword("correct horse battery staple")
	b, _ := HashPassword("correct horse battery staple")
	if a == b {
		t.Fatal("two hashes of the same password must differ (salt)")
	}
}

func TestHashPasswordEncodesPHCWithParameters(t *testing.T) {
	t.Parallel()
	encoded, _ := HashPassword("correct horse battery staple")
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected PHC prefix: %s", encoded)
	}
}

func TestPasswordPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		password string
		want     error
	}{
		{"eleven chars rejected", "abcdefghijk", ErrPasswordTooShort},
		{"twelve chars accepted", "abcdefghijkl", nil},
		{"twelve runes accepted", "ααααααααααααα"[:24], nil}, // 12 two-byte runes
		{"129 chars rejected", strings.Repeat("a", 129), ErrPasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ValidatePassword(tc.password); !errors.Is(got, tc.want) {
				t.Fatalf("ValidatePassword(%q) = %v, want %v", tc.password, got, tc.want)
			}
		})
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "plaintext", "$argon2id$v=19$m=65536,t=3,p=2$onlysalt", "$bcrypt$x$y$z$w"} {
		if _, err := VerifyPassword(bad, "whatever-password"); !errors.Is(err, ErrMalformedHash) {
			t.Errorf("VerifyPassword(%q) error = %v, want ErrMalformedHash", bad, err)
		}
	}
}

func TestVerifyPasswordRejectsTamperedHash(t *testing.T) {
	t.Parallel()
	encoded, _ := HashPassword("correct horse battery staple")
	last := encoded[len(encoded)-1]
	swap := byte('A')
	if last == 'A' {
		swap = 'B'
	}
	tampered := encoded[:len(encoded)-1] + string(swap)
	ok, _ := VerifyPassword(tampered, "correct horse battery staple")
	if ok {
		t.Fatal("tampered hash verified")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/auth/ -run 'Password' -v`
Expected: FAIL — `undefined: HashPassword` (package does not compile yet).

- [ ] **Step 4: Write the implementation**

```go
// Package auth owns credentials: password hashing, session tokens, roles and
// the one-time first-run setup token. It deliberately has no dependency on
// net/http or storage so every piece can be tested in isolation.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	// MinPasswordLength follows NIST 800-63B: length over composition rules.
	MinPasswordLength = 12
	// MaxPasswordLength bounds hashing cost; argon2 has no bcrypt-style cap.
	MaxPasswordLength = 128

	// Above the OWASP minimum (19 MiB, t=2). 64 MiB per attempt is fine on a
	// monitoring server; the login rate limiter bounds concurrent hashes.
	argonMemory  uint32 = 64 * 1024 // KiB
	argonTime    uint32 = 3
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
	saltLength          = 16
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d characters", MaxPasswordLength)
	ErrMalformedHash    = errors.New("malformed password hash")
)

// ValidatePassword applies the length policy. Length is counted in runes so a
// user typing non-ASCII characters is not penalised.
func ValidatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

// HashPassword returns a PHC-formatted argon2id hash. Parameters travel with
// the hash so they can be raised later without a migration.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches the PHC hash. A mismatch is
// (false, nil); only an unparseable hash is an error.
func VerifyPassword(encoded, password string) (bool, error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(candidate, key) == 1, nil
}

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<key>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	var p argonParams
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	if p.memory == 0 || p.time == 0 || p.threads == 0 {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return argonParams{}, nil, nil, ErrMalformedHash
	}
	return p, salt, key, nil
}

// dummyHash is verified against when a login names an email that does not
// exist, so the response takes as long as a real mismatch and the timing does
// not enumerate accounts. Computed lazily: hashing costs 64 MiB and tens of
// milliseconds, which tests that never log in should not pay.
var dummyHash = sync.OnceValue(func() string {
	h, err := HashPassword("timing-equaliser-not-a-real-password")
	if err != nil {
		panic(err)
	}
	return h
})

// VerifyDummy burns the same work as a real verification and discards the
// result.
func VerifyDummy(password string) {
	_, _ = VerifyPassword(dummyHash(), password)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/auth/ -run 'Password' -race -v`
Expected: all 7 PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/auth/password.go internal/auth/password_test.go
git commit -m "feat(auth): add argon2id password hashing"
```

---

### Task 2: tokens, roles, email normalisation, setup token

**Files:**
- Create: `internal/auth/token.go`, `internal/auth/user.go`, `internal/auth/setup.go`
- Test: `internal/auth/token_test.go`, `internal/auth/user_test.go`, `internal/auth/setup_test.go`

**Interfaces:**
- Produces: `NewToken() (string, error)` (43-char base64url), `HashToken(token string) string` (64-char hex), `NewID() (string, error)` (32-char hex); `type Role string`, `RoleAdmin`, `RoleViewer`, `ParseRole(string) (Role, error)`, `ErrInvalidRole`; `type User struct{ID, Email string; Role Role}`; `NormalizeEmail(string) (string, error)`, `ErrInvalidEmail`; `type SetupToken`, `NewSetupToken() (*SetupToken, error)`, `(*SetupToken).String() string`, `(*SetupToken).Consume(candidate string) bool`.

- [ ] **Step 1: Write the failing tests**

`token_test.go`:
```go
package auth

import "testing"

func TestNewTokenIsUniqueAndURLSafe(t *testing.T) {
	t.Parallel()
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewToken()
	if a == b {
		t.Fatal("two tokens were identical")
	}
	if len(a) != 43 {
		t.Fatalf("token length = %d, want 43 (32 bytes base64url, unpadded)", len(a))
	}
	for _, c := range a {
		if c == '+' || c == '/' || c == '=' {
			t.Fatalf("token contains non-URL-safe char %q", c)
		}
	}
}

func TestHashTokenIsDeterministicHex(t *testing.T) {
	t.Parallel()
	h1 := HashToken("abc")
	h2 := HashToken("abc")
	if h1 != h2 {
		t.Fatal("HashToken not deterministic")
	}
	if len(h1) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(h1))
	}
	if h1 == "abc" {
		t.Fatal("HashToken returned its input")
	}
}

func TestNewIDIs32Hex(t *testing.T) {
	t.Parallel()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("id length = %d, want 32", len(id))
	}
}
```

`user_test.go`:
```go
package auth

import (
	"errors"
	"testing"
)

func TestParseRole(t *testing.T) {
	t.Parallel()
	if r, err := ParseRole("admin"); err != nil || r != RoleAdmin {
		t.Fatalf("ParseRole(admin) = %v, %v", r, err)
	}
	if r, err := ParseRole("viewer"); err != nil || r != RoleViewer {
		t.Fatalf("ParseRole(viewer) = %v, %v", r, err)
	}
	if _, err := ParseRole("root"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("ParseRole(root) error = %v, want ErrInvalidRole", err)
	}
}

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		err  error
	}{
		{"  Ops@Example.COM ", "ops@example.com", nil},
		{"ops@example.com", "ops@example.com", nil},
		{"", "", ErrInvalidEmail},
		{"no-at-sign", "", ErrInvalidEmail},
		{"@example.com", "", ErrInvalidEmail},
		{"ops@", "", ErrInvalidEmail},
		{"two@@example.com", "", ErrInvalidEmail},
		{"sp ace@example.com", "", ErrInvalidEmail},
	}
	for _, tc := range cases {
		got, err := NormalizeEmail(tc.in)
		if !errors.Is(err, tc.err) || got != tc.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}
```

`setup_test.go`:
```go
package auth

import "testing"

func TestSetupTokenConsumesExactlyOnce(t *testing.T) {
	t.Parallel()
	tok, err := NewSetupToken()
	if err != nil {
		t.Fatal(err)
	}
	if tok.Consume("wrong") {
		t.Fatal("wrong token consumed")
	}
	if !tok.Consume(tok.String()) {
		t.Fatal("correct token rejected")
	}
	if tok.Consume(tok.String()) {
		t.Fatal("token consumed twice")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/auth/ -v 2>&1 | head -5`
Expected: build failure — `undefined: NewToken` etc.

- [ ] **Step 3: Write the implementations**

`token.go`:
```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const tokenBytes = 32

// NewToken returns a 256-bit random session token, base64url without padding
// so it is safe in a cookie value.
func NewToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken derives the value that is stored. A leaked sessions table then
// yields nothing usable, because SHA-256 cannot be reversed to the cookie.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// NewID returns a 128-bit random identifier for users.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
```

`user.go`:
```go
package auth

import (
	"errors"
	"strings"
)

// Role is what a user may do. Two levels for now; the check is centralised so
// adding more later is a one-place change.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

var (
	ErrInvalidRole  = errors.New("invalid role")
	ErrInvalidEmail = errors.New("invalid email address")
)

// ParseRole validates a role supplied by a client.
func ParseRole(s string) (Role, error) {
	switch Role(strings.TrimSpace(s)) {
	case RoleAdmin:
		return RoleAdmin, nil
	case RoleViewer:
		return RoleViewer, nil
	default:
		return "", ErrInvalidRole
	}
}

// User is the identity attached to an authenticated request. It carries no
// secrets so it can be serialised to the dashboard as-is.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

// NormalizeEmail lower-cases and trims an address and rejects obviously
// malformed input. Deliverability is not checked; uniqueness is the
// database's job.
func NormalizeEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \t\r\n") {
		return "", ErrInvalidEmail
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return "", ErrInvalidEmail
	}
	return s, nil
}
```

`setup.go`:
```go
package auth

import (
	"crypto/subtle"
	"sync"
)

// SetupToken gates first-run admin creation. It lives only in memory, is
// printed once to the daemon log, and is consumed by the first successful
// use — so a default password never exists, not even briefly.
type SetupToken struct {
	mu       sync.Mutex
	value    string
	consumed bool
}

func NewSetupToken() (*SetupToken, error) {
	v, err := NewToken()
	if err != nil {
		return nil, err
	}
	return &SetupToken{value: v}, nil
}

// String returns the token for the one log line that announces it.
func (t *SetupToken) String() string {
	return t.value
}

// Consume reports whether candidate matches, and if so retires the token.
func (t *SetupToken) Consume(candidate string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(t.value)) != 1 {
		return false
	}
	t.consumed = true
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/auth/ -race -v`
Expected: all PASS (Task 1 + Task 2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): add tokens, roles, email normalisation and setup token"
```

---

### Task 3: users table and CRUD

**Files:**
- Create: `internal/storage/users.go`
- Modify: `internal/storage/sqlite.go:38-45` (call `createAuthTables` after `createPostMigrationIndexes`)
- Test: `internal/storage/users_test.go`

**Interfaces:**
- Produces: `ErrNotFound`, `ErrDuplicateEmail`; `type UserRecord struct{ID, Email, PasswordHash, Role string; CreatedAt time.Time; LastLoginAt *time.Time}`; `(*Store).CountUsers() (int, error)`, `CountAdmins() (int, error)`, `CreateUser(UserRecord) error`, `GetUserByEmail(string) (*UserRecord, error)`, `GetUserByID(string) (*UserRecord, error)`, `ListUsers() ([]UserRecord, error)`, `DeleteUser(id string) error`, `UpdatePasswordHash(id, hash string) error`, `TouchLastLogin(id string, at time.Time) error`.
- The DSN already sets `_foreign_keys=ON` (`sqlite.go:53`), so `ON DELETE CASCADE` on sessions is honoured.

- [ ] **Step 1: Write the failing tests**

```go
package storage

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newUserStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestCreateUserAndCount(t *testing.T) {
	store := newUserStore(t)
	if n, _ := store.CountUsers(); n != 0 {
		t.Fatalf("fresh store has %d users", n)
	}
	err := store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if n, _ := store.CountUsers(); n != 1 {
		t.Fatalf("CountUsers = %d, want 1", n)
	}
	if n, _ := store.CountAdmins(); n != 1 {
		t.Fatalf("CountAdmins = %d, want 1", n)
	}
}

func TestCreateUserRejectsDuplicateEmailCaseInsensitively(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	err := store.CreateUser(UserRecord{ID: "u2", Email: "OPS@example.com", PasswordHash: "h", Role: "viewer", CreatedAt: time.Now()})
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("second insert error = %v, want ErrDuplicateEmail", err)
	}
}

func TestGetUserByEmailIsCaseInsensitiveAndRoundTrips(t *testing.T) {
	store := newUserStore(t)
	created := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "hash-1", Role: "viewer", CreatedAt: created})
	u, err := store.GetUserByEmail("Ops@Example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if u.ID != "u1" || u.PasswordHash != "hash-1" || u.Role != "viewer" || !u.CreatedAt.Equal(created) || u.LastLoginAt != nil {
		t.Fatalf("unexpected record: %+v", u)
	}
}

func TestGetUserMissingReturnsErrNotFound(t *testing.T) {
	store := newUserStore(t)
	if _, err := store.GetUserByEmail("nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByEmail error = %v, want ErrNotFound", err)
	}
	if _, err := store.GetUserByID("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUserByID error = %v, want ErrNotFound", err)
	}
}

func TestUpdatePasswordHashAndTouchLastLogin(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "ops@example.com", PasswordHash: "old", Role: "admin", CreatedAt: time.Now()})
	if err := store.UpdatePasswordHash("u1", "new"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 2, 12, 30, 0, 0, time.UTC)
	if err := store.TouchLastLogin("u1", at); err != nil {
		t.Fatal(err)
	}
	u, _ := store.GetUserByID("u1")
	if u.PasswordHash != "new" {
		t.Fatalf("hash = %q, want new", u.PasswordHash)
	}
	if u.LastLoginAt == nil || !u.LastLoginAt.Equal(at) {
		t.Fatalf("LastLoginAt = %v, want %v", u.LastLoginAt, at)
	}
}

func TestListAndDeleteUsers(t *testing.T) {
	store := newUserStore(t)
	_ = store.CreateUser(UserRecord{ID: "u1", Email: "a@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()})
	_ = store.CreateUser(UserRecord{ID: "u2", Email: "b@example.com", PasswordHash: "h", Role: "viewer", CreatedAt: time.Now()})
	users, err := store.ListUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("ListUsers = %d users, %v", len(users), err)
	}
	if err := store.DeleteUser("u2"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUser("u2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
	if n, _ := store.CountUsers(); n != 1 {
		t.Fatalf("CountUsers after delete = %d, want 1", n)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/storage/ -run 'User' -v 2>&1 | head -5`
Expected: build failure — `undefined: UserRecord`.

- [ ] **Step 3: Write the implementation**

`users.go`:
```go
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mattn/go-sqlite3"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrDuplicateEmail = errors.New("email already registered")
)

// UserRecord is the stored shape of an account. PasswordHash never leaves the
// storage/server boundary; handlers map this to auth.User before responding.
type UserRecord struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

// createAuthTables is called from NewStore after the metrics migrations. Both
// tables are new, so plain CREATE IF NOT EXISTS is the whole migration story.
func createAuthTables(db *sql.DB) error {
	stmts := []string{`
		CREATE TABLE IF NOT EXISTS users (
			id            TEXT PRIMARY KEY,
			email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
			password_hash TEXT NOT NULL,
			role          TEXT NOT NULL CHECK (role IN ('admin', 'viewer')),
			created_at    DATETIME NOT NULL,
			last_login_at DATETIME
		)`, `
		CREATE TABLE IF NOT EXISTS sessions (
			token_hash   TEXT PRIMARY KEY,
			user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at   DATETIME NOT NULL,
			expires_at   DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create auth tables: %w", err)
		}
	}
	return nil
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(u UserRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO users (id, email, password_hash, role, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, NULL)`,
		u.ID, u.Email, u.PasswordHash, u.Role, u.CreatedAt.UTC())
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
		return ErrDuplicateEmail
	}
	return err
}

const userColumns = `id, email, password_hash, role, created_at, last_login_at`

func scanUser(row interface{ Scan(...any) error }) (*UserRecord, error) {
	var u UserRecord
	var lastLogin sql.NullTime
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	if lastLogin.Valid {
		t := lastLogin.Time.UTC()
		u.LastLoginAt = &t
	}
	return &u, nil
}

func (s *Store) GetUserByEmail(email string) (*UserRecord, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE email = ? COLLATE NOCASE`, email))
}

func (s *Store) GetUserByID(id string) (*UserRecord, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

func (s *Store) ListUsers() ([]UserRecord, error) {
	rows, err := s.db.Query(`SELECT ` + userColumns + ` FROM users ORDER BY created_at ASC, email ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []UserRecord
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (s *Store) DeleteUser(id string) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdatePasswordHash(id, hash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchLastLogin(id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, at.UTC(), id)
	return err
}
```

In `sqlite.go` `NewStore`, directly after the `createPostMigrationIndexes` block:
```go
	if err := createAuthTables(db); err != nil {
		_ = db.Close()
		return nil, err
	}
```
and change the blank driver import `_ "github.com/mattn/go-sqlite3"` to a named import only in `users.go` (the blank import in `sqlite.go` can stay).

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/storage/ -race -v -run 'User'`
Expected: 6 PASS. Then `GOTOOLCHAIN=auto go test ./internal/storage/ -race` — whole package still green.

- [ ] **Step 5: Commit**

```bash
git add internal/storage/users.go internal/storage/users_test.go internal/storage/sqlite.go
git commit -m "feat(storage): add users and sessions tables with user CRUD"
```

---

### Task 4: sessions CRUD and prune

**Files:**
- Create: `internal/storage/sessions.go`
- Test: `internal/storage/sessions_test.go`

**Interfaces:**
- Produces: `type SessionRecord struct{TokenHash, UserID string; CreatedAt, ExpiresAt, LastSeenAt time.Time}`; `(*Store).CreateSession(SessionRecord) error`, `GetSession(tokenHash string) (*SessionRecord, error)` (`ErrNotFound`), `TouchSession(tokenHash string, lastSeen, expiresAt time.Time) error`, `DeleteSession(tokenHash string) error`, `DeleteUserSessions(userID, keepTokenHash string) error`, `PruneExpiredSessions(now time.Time) (int64, error)`.

- [ ] **Step 1: Write the failing tests**

```go
package storage

import (
	"errors"
	"testing"
	"time"
)

func seedUser(t *testing.T, store *Store, id string) {
	t.Helper()
	if err := store.CreateUser(UserRecord{ID: id, Email: id + "@example.com", PasswordHash: "h", Role: "admin", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	rec := SessionRecord{TokenHash: "t1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour), LastSeenAt: now}
	if err := store.CreateSession(rec); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSession("t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "u1" || !got.ExpiresAt.Equal(rec.ExpiresAt) || !got.CreatedAt.Equal(now) {
		t.Fatalf("unexpected session: %+v", got)
	}
	if _, err := store.GetSession("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session error = %v, want ErrNotFound", err)
	}
}

func TestTouchSessionExtendsExpiry(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	_ = store.CreateSession(SessionRecord{TokenHash: "t1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	later := now.Add(30 * time.Minute)
	if err := store.TouchSession("t1", later, later.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetSession("t1")
	if !got.LastSeenAt.Equal(later) || !got.ExpiresAt.Equal(later.Add(24*time.Hour)) {
		t.Fatalf("touch not applied: %+v", got)
	}
}

func TestDeleteSessionAndDeleteUserSessionsKeepsOne(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Now()
	for _, h := range []string{"a", "b", "c"} {
		_ = store.CreateSession(SessionRecord{TokenHash: h, UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	}
	if err := store.DeleteSession("a"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUserSessions("u1", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("b"); err != nil {
		t.Fatalf("kept session missing: %v", err)
	}
	for _, h := range []string{"a", "c"} {
		if _, err := store.GetSession(h); !errors.Is(err, ErrNotFound) {
			t.Fatalf("session %q should be gone, got %v", h, err)
		}
	}
}

func TestDeletingUserCascadesSessions(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Now()
	_ = store.CreateSession(SessionRecord{TokenHash: "a", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	if err := store.DeleteUser("u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSession("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session survived user deletion: %v", err)
	}
}

func TestPruneExpiredSessions(t *testing.T) {
	store := newUserStore(t)
	seedUser(t, store, "u1")
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	_ = store.CreateSession(SessionRecord{TokenHash: "old", UserID: "u1", CreatedAt: now.Add(-48 * time.Hour), ExpiresAt: now.Add(-time.Minute), LastSeenAt: now.Add(-time.Hour)})
	_ = store.CreateSession(SessionRecord{TokenHash: "live", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now})
	n, err := store.PruneExpiredSessions(now)
	if err != nil || n != 1 {
		t.Fatalf("PruneExpiredSessions = %d, %v; want 1, nil", n, err)
	}
	if _, err := store.GetSession("live"); err != nil {
		t.Fatalf("live session pruned: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/storage/ -run 'Session' -v 2>&1 | head -5`
Expected: build failure — `undefined: SessionRecord`.

- [ ] **Step 3: Write the implementation**

```go
package storage

import (
	"database/sql"
	"errors"
	"time"
)

// SessionRecord stores the SHA-256 of a session token, never the token.
type SessionRecord struct {
	TokenHash  string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

func (s *Store) CreateSession(rec SessionRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)`,
		rec.TokenHash, rec.UserID, rec.CreatedAt.UTC(), rec.ExpiresAt.UTC(), rec.LastSeenAt.UTC())
	return err
}

func (s *Store) GetSession(tokenHash string) (*SessionRecord, error) {
	var rec SessionRecord
	err := s.db.QueryRow(`
		SELECT token_hash, user_id, created_at, expires_at, last_seen_at
		FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&rec.TokenHash, &rec.UserID, &rec.CreatedAt, &rec.ExpiresAt, &rec.LastSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rec.CreatedAt, rec.ExpiresAt, rec.LastSeenAt = rec.CreatedAt.UTC(), rec.ExpiresAt.UTC(), rec.LastSeenAt.UTC()
	return &rec, nil
}

// TouchSession records activity and slides the idle expiry forward.
func (s *Store) TouchSession(tokenHash string, lastSeen, expiresAt time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
		lastSeen.UTC(), expiresAt.UTC(), tokenHash)
	return err
}

func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteUserSessions revokes every session for a user except keepTokenHash —
// used after a password change so the changing browser stays signed in while
// every other device is signed out.
func (s *Store) DeleteUserSessions(userID, keepTokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND token_hash <> ?`, userID, keepTokenHash)
	return err
}

func (s *Store) PruneExpiredSessions(now time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, now.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/storage/ -race`
Expected: PASS (all storage tests, including the cascade test which proves `_foreign_keys=ON` is live).

- [ ] **Step 5: Commit**

```bash
git add internal/storage/sessions.go internal/storage/sessions_test.go
git commit -m "feat(storage): add session persistence and pruning"
```

---

### Task 5: configuration keys

**Files:**
- Modify: `internal/config/config.go` (`ServerConfig`, new `AuthConfig`, `Config.Auth`, defaults, `Validate`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `ServerConfig.Insecure bool` (`server.insecure`), `type AuthConfig struct{SessionIdleHours, SessionMaxDays, LoginRatePerMinute int}` (`auth.session_idle_hours`, `auth.session_max_days`, `auth.login_rate_per_minute`), `Config.Auth AuthConfig`.

- [ ] **Step 1: Write the failing tests** (append to `config_test.go`)

```go
func TestAuthDefaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Insecure {
		t.Fatal("server.insecure must default to false")
	}
	if cfg.Auth.SessionIdleHours != 24 || cfg.Auth.SessionMaxDays != 30 || cfg.Auth.LoginRatePerMinute != 5 {
		t.Fatalf("unexpected auth defaults: %+v", cfg.Auth)
	}
}

func TestAuthEnvOverrides(t *testing.T) {
	t.Setenv("SYS_SENTIENT_SERVER_INSECURE", "true")
	t.Setenv("SYS_SENTIENT_AUTH_SESSION_IDLE_HOURS", "2")
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Server.Insecure || cfg.Auth.SessionIdleHours != 2 {
		t.Fatalf("env override not applied: insecure=%v idle=%d", cfg.Server.Insecure, cfg.Auth.SessionIdleHours)
	}
}

func TestAuthValidationRejectsZeroes(t *testing.T) {
	for _, env := range []string{"SYS_SENTIENT_AUTH_SESSION_IDLE_HOURS", "SYS_SENTIENT_AUTH_SESSION_MAX_DAYS", "SYS_SENTIENT_AUTH_LOGIN_RATE_PER_MINUTE"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "0")
			if _, err := LoadConfig(""); err == nil {
				t.Fatalf("%s=0 should fail validation", env)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/config/ -run 'Auth' -v 2>&1 | head -5`
Expected: build failure — `cfg.Auth undefined`.

- [ ] **Step 3: Write the implementation**

In `ServerConfig` add after `AllowedOrigins`:
```go
	// Insecure disables authentication entirely. It exists for throwaway
	// local runs and is warned about on every start; never set it on a
	// network-reachable install.
	Insecure bool `mapstructure:"insecure"`
```

Add the type and field:
```go
type AuthConfig struct {
	// SessionIdleHours is how long a session survives without activity.
	SessionIdleHours int `mapstructure:"session_idle_hours"`
	// SessionMaxDays caps a session's total life regardless of activity.
	SessionMaxDays int `mapstructure:"session_max_days"`
	// LoginRatePerMinute bounds password attempts per client IP.
	LoginRatePerMinute int `mapstructure:"login_rate_per_minute"`
}
```
`Config` gains `Auth AuthConfig \`mapstructure:"auth"\``.

Defaults, next to the other `SetDefault` calls:
```go
	v.SetDefault("server.insecure", false)
	v.SetDefault("auth.session_idle_hours", 24)
	v.SetDefault("auth.session_max_days", 30)
	v.SetDefault("auth.login_rate_per_minute", 5)
```

In `Validate`, before the final `return nil`:
```go
	if c.Auth.SessionIdleHours < 1 {
		return fmt.Errorf("auth.session_idle_hours must be at least 1, got %d", c.Auth.SessionIdleHours)
	}
	if c.Auth.SessionMaxDays < 1 {
		return fmt.Errorf("auth.session_max_days must be at least 1, got %d", c.Auth.SessionMaxDays)
	}
	if c.Auth.LoginRatePerMinute < 1 {
		return fmt.Errorf("auth.login_rate_per_minute must be at least 1, got %d", c.Auth.LoginRatePerMinute)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/config/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add auth session and insecure-mode settings"
```

---

### Task 6: session cookies, `requireAuth`/`requireAdmin`, `routes()`

**Files:**
- Create: `internal/server/session.go`
- Modify: `internal/server/auth.go` (add `authenticate`, `requireAuth`, `requireAdmin`; keep `AuthMiddleware` + `validAPIKey` for the agent key)
- Modify: `internal/server/server.go` (struct fields, `NewServer` defaults, `WithAuth`, extract `routes()` from `Start()`, drop `validWebSocketAPIKey`, drop the `?api_key=` WebSocket query)
- Test: `internal/server/auth_test.go`; delete `TestServerWebSocketAPIKeyUsesAuthValidator` from `server_test.go:109-120`

**Interfaces:**
- Consumes: `auth.NewToken/HashToken/ParseRole/User/Role*`, `storage.SessionRecord/CreateSession/GetSession/TouchSession/DeleteSession/GetUserByID`, `config.AuthConfig`, `config.ServerConfig.Insecure`.
- Produces: `(*Server).WithAuth(cfg config.AuthConfig, setup *auth.SetupToken) *Server`; `(*Server).routes() http.Handler`; `(*Server).requireAuth(http.HandlerFunc) http.HandlerFunc`; `(*Server).requireAdmin(http.HandlerFunc) http.HandlerFunc`; `(*Server).authenticate(*http.Request) (principal, bool)`; `(*Server).issueSession(w, r, userID string, now time.Time) error`; `clearSessionCookie(w, r)`; `principalFrom(ctx) (principal, bool)`; `type principal struct{user auth.User; tokenHash string; viaCookie bool}`; const `sessionCookieName = "sys_session"`.

- [ ] **Step 1: Write the failing tests**

`internal/server/auth_test.go`:
```go
package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
	"sys-sentient/internal/storage"
)

func newAuthTestServer(t *testing.T, cfg config.ServerConfig) (*Server, *storage.Store) {
	t.Helper()
	store, err := storage.NewStore(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	srv := NewServer(cfg, testPrivacy(), store, nil, nil, nil).
		WithAuth(config.AuthConfig{SessionIdleHours: 24, SessionMaxDays: 30, LoginRatePerMinute: 5}, nil)
	t.Cleanup(func() {
		srv.Hub.Close()
		_ = store.Close()
	})
	return srv, store
}

func seedAccount(t *testing.T, store *storage.Store, id, email, password string, role auth.Role) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(role), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

// sessionCookie mints a session directly, so middleware can be tested before
// the login handler exists.
func sessionCookie(t *testing.T, srv *Server, userID string) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := srv.issueSession(rec, req, userID, time.Now()); err != nil {
		t.Fatalf("issueSession: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func TestProtectedRouteRejectsAnonymous(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestProtectedRouteAcceptsMachineKey(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{APIKey: "machine-key"})
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("X-API-Key", "machine-key")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestProtectedRouteAcceptsSessionCookie(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleViewer)
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.AddCookie(sessionCookie(t, srv, "u1"))
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleAdmin)

	plain := sessionCookie(t, srv, "u1")
	if plain.Name != sessionCookieName || !plain.HttpOnly || plain.SameSite != http.SameSiteStrictMode || plain.Path != "/" {
		t.Fatalf("cookie attributes wrong: %+v", plain)
	}
	if plain.Secure {
		t.Fatal("Secure must not be set on a plain-HTTP request")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	if err := srv.issueSession(rec, req, "u1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !rec.Result().Cookies()[0].Secure {
		t.Fatal("Secure must be set when forwarded over https")
	}
}

func TestViewerCannotCallAdminEndpoints(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "v1", "viewer@example.com", "correct horse battery", auth.RoleViewer)
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "v1"))
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "a1"))
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
		t.Fatalf("admin status = %d, must pass the auth gate", rec.Code)
	}
}

func TestCrossSiteMutationRejected(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", nil)
	req.AddCookie(sessionCookie(t, srv, "a1"))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestInsecureModeBypassesAuth(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{Insecure: true})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleAdmin)
	token, _ := auth.NewToken()
	past := time.Now().Add(-time.Minute)
	_ = store.CreateSession(storage.SessionRecord{TokenHash: auth.HashToken(token), UserID: "u1", CreatedAt: past.Add(-time.Hour), ExpiresAt: past, LastSeenAt: past})
	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebSocketRejectsQueryKeyAndAcceptsCookie(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{APIKey: "machine-key"})
	seedAccount(t, store, "u1", "ops@example.com", "correct horse battery", auth.RoleViewer)

	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ws/metrics?api_key=machine-key", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("query-string key status = %d, want 401 (query auth removed)", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws/metrics", nil)
	req.AddCookie(sessionCookie(t, srv, "u1"))
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("cookie-authenticated upgrade was rejected as unauthenticated")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -run 'Protected|SessionCookie|Viewer|CrossSite|Insecure|Expired|WebSocketRejects' 2>&1 | head -5`
Expected: build failure — `srv.WithAuth undefined`, `srv.routes undefined`.

- [ ] **Step 3: Write `session.go`**

```go
package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

const sessionCookieName = "sys_session"

// sessionTouchInterval bounds how often a sliding expiry is written back, so
// the dashboard's two-second polling does not turn into a write per request.
const sessionTouchInterval = 5 * time.Minute

// principal is who a request acts as and how they proved it. Handlers that
// need to revoke "this" session read tokenHash; handlers that gate on browser
// origin read viaCookie.
type principal struct {
	user      auth.User
	tokenHash string
	viaCookie bool
}

type principalKey struct{}

func withPrincipal(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principalFrom(ctx context.Context) (principal, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p, ok
}

var (
	// apiKeyPrincipal is the synthetic identity behind the machine token.
	apiKeyPrincipal = principal{user: auth.User{ID: "api-key", Email: "api-key", Role: auth.RoleAdmin}}
	// insecurePrincipal is used when server.insecure disables auth.
	insecurePrincipal = principal{user: auth.User{ID: "insecure", Email: "insecure", Role: auth.RoleAdmin}}
)

// requestIsSecure decides the cookie's Secure flag per request. Setting it
// unconditionally would silently break every plain-HTTP LAN install, because
// browsers drop Secure cookies on http://.
func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string, now time.Time) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	rec := storage.SessionRecord{
		TokenHash:  auth.HashToken(token),
		UserID:     userID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.sessionIdle),
		LastSeenAt: now,
	}
	if err := s.store.CreateSession(rec); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		// The browser keeps the cookie for the absolute cap; the server
		// enforces the shorter idle window on every request.
		MaxAge: int(s.sessionMax.Seconds()),
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// resolveSession turns a cookie into a principal, enforcing both the idle
// and the absolute expiry, and sliding the idle window on activity.
func (s *Server) resolveSession(r *http.Request, now time.Time) (principal, bool) {
	if s.store == nil {
		return principal{}, false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return principal{}, false
	}
	hash := auth.HashToken(c.Value)
	rec, err := s.store.GetSession(hash)
	if err != nil {
		return principal{}, false
	}
	if now.After(rec.ExpiresAt) || now.After(rec.CreatedAt.Add(s.sessionMax)) {
		_ = s.store.DeleteSession(hash)
		return principal{}, false
	}
	u, err := s.store.GetUserByID(rec.UserID)
	if err != nil {
		return principal{}, false
	}
	role, err := auth.ParseRole(u.Role)
	if err != nil {
		return principal{}, false
	}
	if now.Sub(rec.LastSeenAt) > sessionTouchInterval {
		_ = s.store.TouchSession(hash, now, now.Add(s.sessionIdle))
	}
	return principal{
		user:      auth.User{ID: u.ID, Email: u.Email, Role: role},
		tokenHash: hash,
		viaCookie: true,
	}, true
}
```

- [ ] **Step 4: Add the middleware to `auth.go`**

Append (keep the existing `AuthMiddleware` type and `validAPIKey`; the agent-key path on `/api/ingest` still uses `AuthenticateFunc`):
```go
func apiKeyFromRequest(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// authenticate resolves the caller: insecure mode, then the machine key,
// then a session cookie. Query-string credentials are deliberately not read.
func (s *Server) authenticate(r *http.Request) (principal, bool) {
	if s.config.Insecure {
		return insecurePrincipal, true
	}
	if key := apiKeyFromRequest(r); key != "" && s.authMiddleware.validAPIKey(key) {
		return apiKeyPrincipal, true
	}
	return s.resolveSession(r, time.Now())
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// requireAuth gates a handler on any authenticated principal. Cookie
// sessions additionally refuse cross-site mutations: SameSite=Strict already
// stops browsers sending the cookie, this is the belt to that brace.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := s.authenticate(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if p.viaCookie && isMutating(r.Method) && strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
			writeJSONError(w, http.StatusForbidden, "cross-site request rejected")
			return
		}
		next(w, r.WithContext(withPrincipal(r.Context(), p)))
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if p, _ := principalFrom(r.Context()); p.user.Role != auth.RoleAdmin {
			writeJSONError(w, http.StatusForbidden, "admin role required")
			return
		}
		next(w, r)
	})
}
```
Add `"time"` and `"sys-sentient/internal/auth"` to the imports.

- [ ] **Step 5: Rework `server.go`**

Struct — add after `logsLimiter`:
```go
	// loginLimiter bounds password attempts per client IP; each attempt costs
	// an argon2 verification, so this also caps memory pressure.
	loginLimiter *rateLimiter
	sessionIdle  time.Duration
	sessionMax   time.Duration
	// setupToken is non-nil only until the first admin exists.
	setupToken *auth.SetupToken
```
`NewServer` literal — add:
```go
		loginLimiter: newRateLimiter(5, 12*time.Second),
		sessionIdle:  24 * time.Hour,
		sessionMax:   30 * 24 * time.Hour,
```
Add after `NewServer`:
```go
// WithAuth applies the operator's session settings and the first-run setup
// token. Tests that never log in can skip it and take the defaults.
func (s *Server) WithAuth(cfg config.AuthConfig, setup *auth.SetupToken) *Server {
	s.sessionIdle = time.Duration(cfg.SessionIdleHours) * time.Hour
	s.sessionMax = time.Duration(cfg.SessionMaxDays) * 24 * time.Hour
	s.loginLimiter = newRateLimiter(cfg.LoginRatePerMinute, time.Minute/time.Duration(cfg.LoginRatePerMinute))
	s.setupToken = setup
	return s
}
```
Replace `Start()` from `mux := http.NewServeMux()` through `handler := s.securityHeaders(s.enableCORS(mux))` with `handler := s.routes()`, and replace the `if s.authMiddleware.enabled {...} else {...}` block with:
```go
	if s.config.Insecure {
		slog.Warn("server.insecure is set: authentication is DISABLED; anyone who can reach this port has full admin access")
	} else {
		slog.Info("authentication enabled: session login or X-API-Key required for /api and /ws")
		slog.Warn("serving plain HTTP: session cookies will not carry the Secure flag; terminate TLS in front of this daemon for production")
	}
```
Add the extracted builder (the auth/user routes reference handlers written in Tasks 7–8; add them here now, they compile once those tasks land — or add them incrementally as each task lands, whichever keeps `go vet` green at every step):
```go
// routes builds the full handler chain. Kept separate from Start so tests
// can drive the real mux, middleware included, without opening a socket.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Public.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handlePrometheusMetrics)
	mux.HandleFunc("GET /api/auth/setup", s.handleSetupStatus)
	mux.HandleFunc("POST /api/auth/setup", rateLimit(s.loginLimiter, "12", s.handleSetup))
	mux.HandleFunc("POST /api/auth/login", rateLimit(s.loginLimiter, "12", s.handleLogin))

	// Any authenticated principal.
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))
	mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("POST /api/auth/password", s.requireAuth(s.handleChangePassword))
	mux.HandleFunc("GET /api/metrics", s.requireAuth(s.handleMetrics))
	mux.HandleFunc("GET /api/insights", s.requireAuth(s.handleInsights))
	mux.HandleFunc("GET /api/logs", s.requireAuth(rateLimit(s.logsLimiter, "2", s.handleLogs)))
	mux.HandleFunc("GET /api/hosts", s.requireAuth(s.handleHosts))
	mux.HandleFunc("GET /api/alerts", s.requireAuth(s.handleAlerts))
	mux.HandleFunc("GET /api/alerts/rules", s.requireAuth(s.handleAlertRules))
	mux.HandleFunc("GET /api/alerts/history", s.requireAuth(s.handleAlertHistory))

	// Admin only: anything that spends money, changes state, or manages people.
	mux.HandleFunc("POST /api/analyze", s.requireAdmin(rateLimit(s.analyzeLimiter, "60", s.handleAnalyze)))
	mux.HandleFunc("POST /api/alerts/{ruleID}/acknowledge", s.requireAdmin(s.handleAcknowledgeAlert))
	mux.HandleFunc("GET /api/users", s.requireAdmin(s.handleListUsers))
	mux.HandleFunc("POST /api/users", s.requireAdmin(s.handleCreateUser))
	mux.HandleFunc("DELETE /api/users/{id}", s.requireAdmin(s.handleDeleteUser))

	// Agents authenticate with their own key.
	mux.HandleFunc("POST /api/ingest", s.agentAuth.AuthenticateFunc(s.handleIngest))

	// WebSocket: same principals as the REST API. The browser sends the
	// cookie on a same-origin upgrade; scripts send X-API-Key.
	mux.HandleFunc("GET /ws/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !s.isOriginAllowed(r.Header.Get("Origin")) {
			writeJSONError(w, http.StatusForbidden, "origin not allowed")
			return
		}
		if _, ok := s.authenticate(r); !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		r = markWebSocketOriginValidated(r)
		ServeWs(s.Hub, w, r)
	})

	mux.Handle("/", staticHandler("./web/dist"))
	return s.securityHeaders(s.enableCORS(mux))
}
```
Delete `validWebSocketAPIKey` (`server.go:370-375`) and its test (`server_test.go:109-120`).

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -race`
Expected: PASS. If Tasks 7–8 handlers are not yet written, temporarily register only the routes that exist and add the rest as those tasks land — never stub a handler with an empty body.

- [ ] **Step 7: Commit**

```bash
git add internal/server/session.go internal/server/auth.go internal/server/server.go internal/server/auth_test.go internal/server/server_test.go
git commit -m "feat(server): session cookies, role middleware and testable routes"
```

---

### Task 7: setup, login, logout, me, change-password handlers

**Files:**
- Create: `internal/server/auth_handlers.go`
- Test: `internal/server/auth_handlers_test.go`

**Interfaces:**
- Consumes: Task 6 (`issueSession`, `clearSessionCookie`, `principalFrom`, `s.setupToken`), `auth.*`, `storage.*`.
- Produces: `handleSetupStatus`, `handleSetup`, `handleLogin`, `handleLogout`, `handleMe`, `handleChangePassword`; helpers `writeJSONStatus(w, status int, payload any)`, `decodeJSONBody(w, r, dst any) error` (4 KiB cap, unknown fields rejected); const `loginFailureMessage = "invalid email or password"`.
- Response shape for setup/login/me: `{"user":{"id":"…","email":"…","role":"admin"}}`.

- [ ] **Step 1: Write the failing tests**

```go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
)

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func firstCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no %s cookie in response", sessionCookieName)
	return nil
}

func TestSetupStatusReflectsUserCount(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	rec := doJSON(t, srv.routes(), http.MethodGet, "/api/auth/setup", nil)
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte(`"needsSetup":true`)) {
		t.Fatalf("fresh install: %d %s", rec.Code, rec.Body.String())
	}
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	rec = doJSON(t, srv.routes(), http.MethodGet, "/api/auth/setup", nil)
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"needsSetup":false`)) {
		t.Fatalf("after first user: %s", rec.Body.String())
	}
}

func TestSetupRequiresTheToken(t *testing.T) {
	srv, _ := newAuthTestServer(t, config.ServerConfig{})
	tok, _ := auth.NewSetupToken()
	srv.setupToken = tok
	rec := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": "wrong", "email": "admin@example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", rec.Code)
	}
	// A bad password must not burn the token.
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "admin@example.com", "password": "short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", rec.Code)
	}
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "Admin@Example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(t, rec)
	me := doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie)
	if me.Code != 200 || !bytes.Contains(me.Body.Bytes(), []byte(`"email":"admin@example.com"`)) || !bytes.Contains(me.Body.Bytes(), []byte(`"role":"admin"`)) {
		t.Fatalf("me after setup: %d %s", me.Code, me.Body.String())
	}
	rec = doJSON(t, srv.routes(), http.MethodPost, "/api/auth/setup",
		map[string]string{"token": tok.String(), "email": "second@example.com", "password": "correct horse battery"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup status = %d, want 409", rec.Code)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	wrongPw := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "not the password"})
	noUser := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "ghost@example.com", "password": "not the password"})
	if wrongPw.Code != 401 || noUser.Code != 401 {
		t.Fatalf("statuses = %d/%d, want 401/401", wrongPw.Code, noUser.Code)
	}
	if wrongPw.Body.String() != noUser.Body.String() {
		t.Fatalf("bodies differ:\n%s\n%s", wrongPw.Body.String(), noUser.Body.String())
	}
	if len(wrongPw.Result().Cookies()) != 0 {
		t.Fatal("failed login set a cookie")
	}
}

func TestLoginLogoutLifecycle(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	rec := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "ADMIN@example.com", "password": "correct horse battery"})
	if rec.Code != 200 {
		t.Fatalf("login status = %d: %s", rec.Code, rec.Body.String())
	}
	cookie := firstCookie(t, rec)
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie).Code != 200 {
		t.Fatal("me with fresh cookie should be 200")
	}
	if u, _ := store.GetUserByID("a1"); u.LastLoginAt == nil {
		t.Fatal("login did not record last_login_at")
	}
	out := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/logout", nil, cookie)
	if out.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", out.Code)
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, cookie).Code != 401 {
		t.Fatal("session survived logout")
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	login := func() *http.Cookie {
		return firstCookie(t, doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "correct horse battery"}))
	}
	laptop, phone := login(), login()
	bad := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/password", map[string]string{"currentPassword": "wrong wrong wrong", "newPassword": "new horse battery staple"}, laptop)
	if bad.Code != 401 {
		t.Fatalf("wrong current password status = %d, want 401", bad.Code)
	}
	ok := doJSON(t, srv.routes(), http.MethodPost, "/api/auth/password", map[string]string{"currentPassword": "correct horse battery", "newPassword": "new horse battery staple"}, laptop)
	if ok.Code != http.StatusNoContent {
		t.Fatalf("change status = %d, want 204: %s", ok.Code, ok.Body.String())
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, laptop).Code != 200 {
		t.Fatal("the changing session must stay signed in")
	}
	if doJSON(t, srv.routes(), http.MethodGet, "/api/auth/me", nil, phone).Code != 401 {
		t.Fatal("other sessions must be revoked")
	}
	if doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "admin@example.com", "password": "new horse battery staple"}).Code != 200 {
		t.Fatal("new password does not log in")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -run 'Setup|Login|ChangePassword' 2>&1 | head -5`
Expected: build failure — `s.handleSetupStatus undefined`.

- [ ] **Step 3: Write the handlers**

```go
package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

const (
	// maxAuthBodyBytes caps credential payloads; nothing legitimate is bigger.
	maxAuthBodyBytes = 4 << 10
	// loginFailureMessage is shared by "no such user" and "wrong password" so
	// the response cannot be used to enumerate accounts.
	loginFailureMessage = "invalid email or password"
)

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	setProtectedJSONHeaders(w)
	w.WriteHeader(status)
	writeJSONBody(w, payload)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func userResponse(u auth.User) map[string]any {
	return map[string]any{"user": u}
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "storage not initialized")
		return
	}
	n, err := s.store.CountUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read users")
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]bool{"needsSetup": n == 0 && !s.config.Insecure})
}

type setupRequest struct {
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleSetup creates the first admin. Everything that can fail on user
// input is checked before the one-time token is consumed, so a typo does
// not force a daemon restart to mint a new one.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	n, err := s.store.CountUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read users")
		return
	}
	if n > 0 {
		writeJSONError(w, http.StatusConflict, "setup already completed")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.setupToken == nil || !s.setupToken.Consume(req.Token) {
		writeJSONError(w, http.StatusForbidden, "invalid setup token")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := auth.NewID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	now := time.Now()
	if err := s.store.CreateUser(storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(auth.RoleAdmin), CreatedAt: now}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	_ = s.store.TouchLastLogin(id, now)
	if err := s.issueSession(w, r, id, now); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSONStatus(w, http.StatusCreated, userResponse(auth.User{ID: id, Email: email, Role: auth.RoleAdmin}))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		auth.VerifyDummy(req.Password)
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	u, err := s.store.GetUserByEmail(email)
	if errors.Is(err, storage.ErrNotFound) {
		// Burn the same work as a real check so timing does not reveal
		// which emails exist.
		auth.VerifyDummy(req.Password)
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, req.Password)
	if err != nil || !ok {
		writeJSONError(w, http.StatusUnauthorized, loginFailureMessage)
		return
	}
	role, err := auth.ParseRole(u.Role)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "user has an invalid role")
		return
	}
	now := time.Now()
	_ = s.store.TouchLastLogin(u.ID, now)
	if err := s.issueSession(w, r, u.ID, now); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSONStatus(w, http.StatusOK, userResponse(auth.User{ID: u.ID, Email: u.Email, Role: role}))
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if p, ok := principalFrom(r.Context()); ok && p.viaCookie {
		_ = s.store.DeleteSession(p.tokenHash)
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	writeJSONStatus(w, http.StatusOK, userResponse(p.user))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !p.viaCookie {
		writeJSONError(w, http.StatusBadRequest, "password change requires a signed-in browser session")
		return
	}
	var req changePasswordRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := s.store.GetUserByID(p.user.ID)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ok, err := auth.VerifyPassword(u.PasswordHash, req.CurrentPassword)
	if err != nil || !ok {
		writeJSONError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.UpdatePasswordHash(u.ID, hash); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to update password")
		return
	}
	// Sign out every other device; the one that changed the password stays.
	_ = s.store.DeleteUserSessions(u.ID, p.tokenHash)
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/auth_handlers.go internal/server/auth_handlers_test.go
git commit -m "feat(server): first-run setup, login, logout and password change"
```

---

### Task 8: user management handlers

**Files:**
- Create: `internal/server/user_handlers.go`
- Test: `internal/server/user_handlers_test.go`

**Interfaces:**
- Consumes: Task 7 helpers, `storage.ListUsers/CreateUser/GetUserByID/CountAdmins/DeleteUser`, `auth.ParseRole/NormalizeEmail/HashPassword/NewID`.
- Produces: `handleListUsers`, `handleCreateUser`, `handleDeleteUser`; wire type `managedUser{ID, Email, Role string; CreatedAt time.Time; LastLoginAt *time.Time}` with JSON `id, email, role, createdAt, lastLoginAt`.

- [ ] **Step 1: Write the failing tests**

```go
package server

import (
	"bytes"
	"net/http"
	"testing"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/config"
)

func TestUserManagementIsAdminOnly(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "v1", "viewer@example.com", "correct horse battery", auth.RoleViewer)
	rec := doJSON(t, srv.routes(), http.MethodGet, "/api/users", nil, sessionCookie(t, srv, "v1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer list status = %d, want 403", rec.Code)
	}
}

func TestAdminListsCreatesAndDeletesUsers(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	admin := sessionCookie(t, srv, "a1")

	created := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "new@example.com", "password": "correct horse battery", "role": "viewer"}, admin)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	dup := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "NEW@example.com", "password": "correct horse battery", "role": "viewer"}, admin)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409", dup.Code)
	}
	badRole := doJSON(t, srv.routes(), http.MethodPost, "/api/users",
		map[string]string{"email": "x@example.com", "password": "correct horse battery", "role": "root"}, admin)
	if badRole.Code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", badRole.Code)
	}

	list := doJSON(t, srv.routes(), http.MethodGet, "/api/users", nil, admin)
	if list.Code != 200 || !bytes.Contains(list.Body.Bytes(), []byte(`"email":"new@example.com"`)) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	if doJSON(t, srv.routes(), http.MethodPost, "/api/auth/login", map[string]string{"email": "new@example.com", "password": "correct horse battery"}).Code != 200 {
		t.Fatal("created user cannot log in")
	}

	newUser, _ := store.GetUserByEmail("new@example.com")
	del := doJSON(t, srv.routes(), http.MethodDelete, "/api/users/"+newUser.ID, nil, admin)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", del.Code)
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/"+newUser.ID, nil, admin).Code != http.StatusNotFound {
		t.Fatal("deleting a missing user should be 404")
	}
}

func TestCannotDeleteSelfOrLastAdmin(t *testing.T) {
	srv, store := newAuthTestServer(t, config.ServerConfig{})
	seedAccount(t, store, "a1", "admin@example.com", "correct horse battery", auth.RoleAdmin)
	seedAccount(t, store, "a2", "second@example.com", "correct horse battery", auth.RoleAdmin)
	a1 := sessionCookie(t, srv, "a1")
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a1", nil, a1).Code != http.StatusBadRequest {
		t.Fatal("deleting yourself should be 400")
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a2", nil, a1).Code != http.StatusNoContent {
		t.Fatal("deleting another admin while two exist should succeed")
	}
	a1Again := sessionCookie(t, srv, "a1")
	_ = a1Again
	seedAccount(t, store, "v1", "viewer@example.com", "correct horse battery", auth.RoleViewer)
	// a1 is now the last admin; a viewer cannot delete anyway, so create a
	// second admin, delete a1 from it, and confirm the last-admin guard.
	seedAccount(t, store, "a3", "third@example.com", "correct horse battery", auth.RoleAdmin)
	a3 := sessionCookie(t, srv, "a3")
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a1", nil, a3).Code != http.StatusNoContent {
		t.Fatal("deleting a1 while a3 exists should succeed")
	}
	if doJSON(t, srv.routes(), http.MethodDelete, "/api/users/a3", nil, a3).Code != http.StatusBadRequest {
		t.Fatal("a3 deleting itself is 400 (self guard fires before last-admin guard)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -run 'UserManagement|AdminLists|CannotDelete' 2>&1 | head -5`
Expected: build failure — `s.handleListUsers undefined`.

- [ ] **Step 3: Write the handlers**

```go
package server

import (
	"errors"
	"net/http"
	"time"

	"sys-sentient/internal/auth"
	"sys-sentient/internal/storage"
)

// managedUser is the admin-facing view of an account. No hash, ever.
type managedUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

func toManagedUser(u storage.UserRecord) managedUser {
	return managedUser{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt}
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.ListUsers()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	users := make([]managedUser, 0, len(records))
	for _, rec := range records {
		users = append(users, toManagedUser(rec))
	}
	writeJSONStatus(w, http.StatusOK, users)
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	role, err := auth.ParseRole(req.Role)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "role must be admin or viewer")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := auth.NewID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	rec := storage.UserRecord{ID: id, Email: email, PasswordHash: hash, Role: string(role), CreatedAt: time.Now()}
	if err := s.store.CreateUser(rec); err != nil {
		if errors.Is(err, storage.ErrDuplicateEmail) {
			writeJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSONStatus(w, http.StatusCreated, toManagedUser(rec))
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, _ := principalFrom(r.Context())
	if id == p.user.ID {
		writeJSONError(w, http.StatusBadRequest, "you cannot delete your own account")
		return
	}
	target, err := s.store.GetUserByID(id)
	if errors.Is(err, storage.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	if target.Role == string(auth.RoleAdmin) {
		n, err := s.store.CountAdmins()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to count admins")
			return
		}
		if n <= 1 {
			writeJSONError(w, http.StatusConflict, "cannot delete the last admin")
			return
		}
	}
	if err := s.store.DeleteUser(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOTOOLCHAIN=auto go test ./internal/server/ -race && GOTOOLCHAIN=auto go vet ./...`
Expected: PASS, vet clean.

- [ ] **Step 5: Commit**

```bash
git add internal/server/user_handlers.go internal/server/user_handlers_test.go
git commit -m "feat(server): admin user management endpoints"
```

---

### Task 9: daemon bootstrap, session pruning, operator docs

**Files:**
- Modify: `cmd/daemon/main.go` (after storage init ~line 79; `NewServer` call line 101; both `dbTicker` cases at ~162 and ~339)
- Modify: `config.yaml.example:13-21`, `SECURITY.md:23-38`, `README.md:17,61`, `QUICK_START.md:27-70`, `CHANGELOG.md` (Unreleased → Added / Changed / Removed)

**Interfaces:**
- Consumes: `auth.NewSetupToken`, `storage.CountUsers/PruneExpiredSessions`, `server.WithAuth`, `config.Auth`, `config.Server.Insecure`.

- [ ] **Step 1: Wire the bootstrap** (no unit test — this is glue; the E2E task proves it)

Add to `main.go` after `logger.Info("storage initialized", ...)`:
```go
	setupToken := bootstrapSetup(store, cfg, logger)
```
Change the `NewServer` call to:
```go
	srv := server.NewServer(cfg.Server, cfg.Privacy, store, aiService, evaluator, dispatcher).
		WithAuth(cfg.Auth, setupToken)
```
Add the function (near `runServerOnly`):
```go
// bootstrapSetup mints the one-time first-run token when no account exists.
// Printing a secret to the log is deliberate and documented: it is the only
// way to create the first admin without a default password, it is single-use,
// and it dies with the process.
func bootstrapSetup(store *storage.Store, cfg *config.Config, logger *slog.Logger) *auth.SetupToken {
	if cfg.Server.Insecure {
		return nil
	}
	n, err := store.CountUsers()
	if err != nil {
		logger.Error("failed to count users", "error", err)
		os.Exit(1)
	}
	if n > 0 {
		return nil
	}
	token, err := auth.NewSetupToken()
	if err != nil {
		logger.Error("failed to generate setup token", "error", err)
		os.Exit(1)
	}
	logger.Warn("FIRST-RUN SETUP REQUIRED: no users exist yet",
		"url", fmt.Sprintf("http://localhost:%d/setup", cfg.Server.Port),
		"token", token.String(),
	)
	return token
}
```
In **both** `case <-dbTicker.C:` blocks add after the alert-events prune:
```go
			if _, err := store.PruneExpiredSessions(time.Now()); err != nil {
				logger.Error("error pruning expired sessions", "error", err)
			}
```
Add `"sys-sentient/internal/auth"` to the imports.

- [ ] **Step 2: Verify it builds and the token prints**

Run: `GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go build -o /tmp/sd-check ./cmd/daemon && (SYS_SENTIENT_DATABASE_PATH=$(mktemp -d)/t.db SYS_SENTIENT_SERVER_PORT=18099 timeout 4 /tmp/sd-check 2>&1 | grep -m1 'FIRST-RUN SETUP')`
Expected: one line containing `url=http://localhost:18099/setup token=…`.

- [ ] **Step 3: Update operator docs**

`config.yaml.example` — replace lines 15–21 (the API-key comment) with:
```yaml
  # Machine token for scripts and curl: send as `X-API-Key` or
  # `Authorization: Bearer`. Browser users sign in at /login instead; on first
  # run the daemon logs a one-time setup token and /setup creates the admin.
  api_key: ""
  # Disable authentication entirely. Only for throwaway local runs; the daemon
  # warns on every start and anyone who can reach the port is an admin.
  insecure: false

auth:
  session_idle_hours: 24      # sign out after this much inactivity
  session_max_days: 30        # …and unconditionally after this long
  login_rate_per_minute: 5    # per client IP; each attempt costs an argon2 hash
```

`SECURITY.md` — replace the "readable by anyone" section (lines 23–38) with a section titled **Authentication** describing: accounts with argon2id, session cookies (`HttpOnly`, `SameSite=Strict`, `Secure` under TLS), first-run token, roles, machine token for scripts, `server.insecure`, and that TLS termination in front of the daemon is still required for the `Secure` flag.

`README.md:17` — delete the "API key is readable" caveat sentence; `README.md:61` row becomes `| server.api_key | – | Machine token for scripts (X-API-Key). Browser users log in. |`; add rows for `server.insecure` and `auth.*`.

`QUICK_START.md:27-70` — remove every `VITE_SYS_SENTIENT_API_KEY` instruction; replace "Configure API Keys" with "First run": start the daemon, read the setup URL and token from the log, open `/setup`, create the admin.

`CHANGELOG.md` under `[Unreleased]`:
```markdown
### Added
- **Login** — user accounts (argon2id), server-issued session cookies, admin
  and viewer roles, first-run setup token, `/api/auth/*` and `/api/users`.

### Changed
- `/api/*` and `/ws/*` now require a session cookie or `X-API-Key`. The
  "no key configured → everything open" mode is gone; `server.insecure: true`
  is the explicit escape hatch.

### Removed
- `VITE_SYS_SENTIENT_API_KEY` and the `?api_key=` WebSocket query parameter.
```

- [ ] **Step 4: Commit**

```bash
git add cmd/daemon/main.go config.yaml.example SECURITY.md README.md QUICK_START.md CHANGELOG.md
git commit -m "feat(daemon): first-run setup token and session pruning"
```

---

### Task 10: web — same-origin URLs, credentials, auth API client

**Files:**
- Modify: `web/constants.ts` (drop `API_KEY`, `authHeaders`, query param), `web/vite.config.ts` (dev proxy), `web/vite-env.d.ts:6` (drop the env declaration), `web/services/api.ts` (11 `authHeaders()` call sites, `fetchWithTimeout`, new functions), `web/hooks/useWebSocket.ts:133` (unchanged call, URL now has no query)
- Test: `web/services/api.test.ts`

**Interfaces:**
- Produces from `services/api.ts`: `interface AuthUser {id: string; email: string; role: 'admin' | 'viewer'}`, `interface ManagedUser extends AuthUser {createdAt: string; lastLoginAt: string | null}`, `class UnauthorizedError extends Error`, `onUnauthorized(handler: (() => void) | null): void`, `fetchMe(): Promise<AuthUser | null>`, `fetchSetupStatus(): Promise<boolean>`, `login(email, password): Promise<AuthUser>`, `logout(): Promise<void>`, `completeSetup(token, email, password): Promise<AuthUser>`, `changePassword(currentPassword, newPassword): Promise<void>`, `fetchUsers(): Promise<ManagedUser[]>`, `createUser(email, password, role): Promise<ManagedUser>`, `deleteUser(id): Promise<void>`.

- [ ] **Step 1: Write the failing tests** (append to `api.test.ts`)

```ts
import { fetchMe, login, onUnauthorized, fetchSetupStatus } from './api.js';

test('fetchMe returns null on 401 and does not fire the unauthorized hook', async () => {
  let fired = 0;
  onUnauthorized(() => { fired += 1; });
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'authentication required' }), { status: 401 });
  assert.equal(await fetchMe(), null);
  assert.equal(fired, 0, '/api/auth/* 401s are expected states, not session loss');
  onUnauthorized(null);
});

test('fetchMe returns the user and sends credentials', async () => {
  let init: RequestInit | undefined;
  globalThis.fetch = async (_input, i) => {
    init = i;
    return new Response(JSON.stringify({ user: { id: 'u1', email: 'ops@example.com', role: 'admin' } }), { status: 200 });
  };
  assert.deepEqual(await fetchMe(), { id: 'u1', email: 'ops@example.com', role: 'admin' });
  assert.equal(init?.credentials, 'same-origin');
});

test('login maps 401 and 429 to friendly errors', async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'invalid email or password' }), { status: 401 });
  await assert.rejects(login('a@example.com', 'x'), /Invalid email or password/);
  globalThis.fetch = async () => new Response('', { status: 429 });
  await assert.rejects(login('a@example.com', 'x'), /Too many attempts/);
});

test('a 401 on a data route fires the unauthorized hook', async () => {
  let fired = 0;
  onUnauthorized(() => { fired += 1; });
  globalThis.fetch = async () => new Response(JSON.stringify({ error: 'authentication required' }), { status: 401 });
  await fetchSetupStatus().catch(() => undefined); // auth route: must not fire
  const { fetchHosts } = await import('./api.js');
  await fetchHosts(); // data route: fires
  assert.equal(fired, 1);
  onUnauthorized(null);
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run services/api.test.ts`
Expected: FAIL — `fetchMe is not a function`.

- [ ] **Step 3: Rewrite `constants.ts`**

```ts
export const REFRESH_RATE_MS = 2000; // 2 seconds
export const LOG_REFRESH_RATE_MS = 10000; // 10 seconds

const viteEnv = import.meta.env ?? {};

// The dashboard is served by the daemon itself, so same-origin is the
// default: cookies flow, no host is hard-coded, and a reverse proxy just
// works. The env overrides remain for split deployments.
const origin = typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080';

export const API_BASE_URL = viteEnv.VITE_SYS_SENTIENT_API_URL || `${origin}/api`;
export const WS_BASE_URL =
  viteEnv.VITE_SYS_SENTIENT_WS_URL || `${origin.replace(/^http/, 'ws')}/ws/metrics`;

/** No credentials in the URL: the browser attaches the session cookie itself. */
export const metricsWebSocketURL = (): string => WS_BASE_URL;
```
Remove `VITE_SYS_SENTIENT_API_KEY` from `vite-env.d.ts`. In `vite.config.ts` add under `server`:
```ts
        proxy: {
          '/api': 'http://localhost:8080',
          '/health': 'http://localhost:8080',
          '/ws': { target: 'ws://localhost:8080', ws: true },
        },
```

- [ ] **Step 4: Update `api.ts`**

Change the import to `import { API_BASE_URL } from '../constants.js';` and delete every `headers: authHeaders(),` / `{ headers: authHeaders() }` argument (11 sites). In `fetchWithTimeout` pass `credentials: 'same-origin'` and add the 401 hook:
```ts
export class UnauthorizedError extends Error {
    constructor() {
        super('Not authenticated');
        this.name = 'UnauthorizedError';
    }
}

let unauthorizedHandler: (() => void) | null = null;

/** AuthProvider registers here so any data request that comes back 401
 *  drops the app to the login screen instead of rendering zeros forever. */
export function onUnauthorized(handler: (() => void) | null): void {
    unauthorizedHandler = handler;
}

const isAuthRoute = (input: RequestInfo | URL): boolean => String(input).includes('/api/auth/');

async function fetchWithTimeout(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const controller = new AbortController();
    const timeoutID = setTimeout(() => controller.abort(), API_REQUEST_TIMEOUT_MS);

    try {
        const response = await fetch(input, {
            credentials: 'same-origin',
            ...init,
            signal: controller.signal,
        });
        if (response.status === 401 && !isAuthRoute(input)) {
            unauthorizedHandler?.();
        }
        return response;
    } catch (error) {
        if (error instanceof DOMException && error.name === 'AbortError') {
            throw new Error('Request timed out');
        }
        throw error;
    } finally {
        clearTimeout(timeoutID);
    }
}
```
Append the auth client:
```ts
export interface AuthUser {
    id: string;
    email: string;
    role: 'admin' | 'viewer';
}

export interface ManagedUser extends AuthUser {
    createdAt: string;
    lastLoginAt: string | null;
}

const JSON_HEADERS = { 'Content-Type': 'application/json' };

function asAuthUser(payload: unknown): AuthUser {
    const record = asRecord(payload);
    const user = asRecord(record?.user) ?? record;
    const role = user?.role === 'admin' ? 'admin' : 'viewer';
    return {
        id: nonEmptyString(user?.id, ''),
        email: nonEmptyString(user?.email, ''),
        role,
    };
}

export const fetchMe = async (): Promise<AuthUser | null> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/me`);
    if (response.status === 401) return null;
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to load session'));
    return asAuthUser(await response.json());
};

export const fetchSetupStatus = async (): Promise<boolean> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/setup`);
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to check setup state'));
    const payload = asRecord(await response.json());
    return payload?.needsSetup === true;
};

export const login = async (email: string, password: string): Promise<AuthUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/login`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ email, password }),
    });
    if (response.status === 401) throw new Error('Invalid email or password');
    if (response.status === 429) throw new Error('Too many attempts. Try again in a minute.');
    if (!response.ok) throw new Error(await readAPIError(response, 'Sign-in failed'));
    return asAuthUser(await response.json());
};

export const logout = async (): Promise<void> => {
    await fetchWithTimeout(`${API_BASE_URL}/auth/logout`, { method: 'POST' });
};

export const completeSetup = async (token: string, email: string, password: string): Promise<AuthUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/setup`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ token, email, password }),
    });
    if (response.status === 403) throw new Error('That setup token is not valid. Check the daemon log.');
    if (response.status === 409) throw new Error('Setup has already been completed. Sign in instead.');
    if (!response.ok) throw new Error(await readAPIError(response, 'Setup failed'));
    return asAuthUser(await response.json());
};

export const changePassword = async (currentPassword: string, newPassword: string): Promise<void> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/auth/password`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ currentPassword, newPassword }),
    });
    if (!response.ok) throw new Error(await readAPIError(response, 'Password change failed'));
};

function asManagedUser(payload: unknown): ManagedUser {
    const record = asRecord(payload);
    return {
        ...asAuthUser(record),
        createdAt: nonEmptyString(record?.createdAt, ''),
        lastLoginAt: typeof record?.lastLoginAt === 'string' ? record.lastLoginAt : null,
    };
}

export const fetchUsers = async (): Promise<ManagedUser[]> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users`);
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to load users'));
    const payload = await response.json();
    return Array.isArray(payload) ? payload.map(asManagedUser) : [];
};

export const createUser = async (email: string, password: string, role: AuthUser['role']): Promise<ManagedUser> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ email, password, role }),
    });
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to create user'));
    return asManagedUser(await response.json());
};

export const deleteUser = async (id: string): Promise<void> => {
    const response = await fetchWithTimeout(`${API_BASE_URL}/users/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (!response.ok) throw new Error(await readAPIError(response, 'Failed to delete user'));
};
```
(`asRecord`, `nonEmptyString`, `readAPIError` already exist in `api.ts`.)

- [ ] **Step 5: Run tests and typecheck**

Run: `cd web && npm run typecheck && npx vitest run services/api.test.ts`
Expected: typecheck clean (the `authHeaders` import is gone everywhere), tests PASS.

- [ ] **Step 6: Commit**

```bash
git add web/constants.ts web/vite.config.ts web/vite-env.d.ts web/services/api.ts web/services/api.test.ts
git commit -m "feat(web): cookie-based API client and auth endpoints"
```

---

### Task 11: `AuthProvider` and `RequireAuth`

**Files:**
- Create: `web/hooks/useAuth.tsx`, `web/components/RequireAuth.tsx`
- Test: `web/hooks/useAuth.test.tsx`, `web/components/RequireAuth.test.tsx`

**Interfaces:**
- Consumes: `fetchMe`, `fetchSetupStatus`, `login`, `logout`, `completeSetup`, `onUnauthorized`, `AuthUser` from `services/api`.
- Produces: `type AuthStatus = 'loading' | 'setup' | 'anon' | 'authed'`; `interface AuthContextValue {status: AuthStatus; user: AuthUser | null; signIn(email, password): Promise<void>; signOut(): Promise<void>; finishSetup(token, email, password): Promise<void>}`; `AuthProvider`, `useAuth()`; `RequireAuth` (route element rendering `<Outlet/>`).

- [ ] **Step 1: Write the failing tests**

`hooks/useAuth.test.tsx`:
```tsx
import assert from 'node:assert/strict';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  fetchMe: vi.fn(),
  fetchSetupStatus: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(async () => undefined),
  completeSetup: vi.fn(),
  onUnauthorized: vi.fn(),
}));
vi.mock('../services/api', () => api);

import { AuthProvider, useAuth } from './useAuth';

const Probe = () => {
  const { status, user } = useAuth();
  return <output>{status}:{user?.email ?? '-'}</output>;
};

afterEach(() => vi.clearAllMocks());

test('resolves to authed when /me returns a user', async () => {
  api.fetchMe.mockResolvedValue({ id: 'u1', email: 'ops@example.com', role: 'admin' });
  render(<AuthProvider><Probe /></AuthProvider>);
  assert.equal(screen.getByRole('status').textContent, 'loading:-');
  await waitFor(() => assert.equal(screen.getByRole('status').textContent, 'authed:ops@example.com'));
});

test('resolves to setup when no session and the daemon needs setup', async () => {
  api.fetchMe.mockResolvedValue(null);
  api.fetchSetupStatus.mockResolvedValue(true);
  render(<AuthProvider><Probe /></AuthProvider>);
  await waitFor(() => assert.equal(screen.getByRole('status').textContent, 'setup:-'));
});

test('resolves to anon when no session and setup is done', async () => {
  api.fetchMe.mockResolvedValue(null);
  api.fetchSetupStatus.mockResolvedValue(false);
  render(<AuthProvider><Probe /></AuthProvider>);
  await waitFor(() => assert.equal(screen.getByRole('status').textContent, 'anon:-'));
});

test('drops to anon when a data request reports 401', async () => {
  api.fetchMe.mockResolvedValue({ id: 'u1', email: 'ops@example.com', role: 'admin' });
  render(<AuthProvider><Probe /></AuthProvider>);
  await waitFor(() => assert.equal(screen.getByRole('status').textContent, 'authed:ops@example.com'));
  const handler = api.onUnauthorized.mock.calls.at(-1)?.[0] as () => void;
  handler();
  await waitFor(() => assert.equal(screen.getByRole('status').textContent, 'anon:-'));
});
```

`components/RequireAuth.test.tsx`:
```tsx
import assert from 'node:assert/strict';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  fetchMe: vi.fn(async () => null),
  fetchSetupStatus: vi.fn(async () => false),
  login: vi.fn(),
  logout: vi.fn(),
  completeSetup: vi.fn(),
  onUnauthorized: vi.fn(),
}));
vi.mock('../services/api', () => api);

import { AuthProvider } from '../hooks/useAuth';
import { RequireAuth } from './RequireAuth';

test('redirects anonymous visitors to /login', async () => {
  render(
    <MemoryRouter initialEntries={['/processes']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<h1>Sign in</h1>} />
          <Route element={<RequireAuth />}>
            <Route path="/processes" element={<h1>Processes</h1>} />
          </Route>
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  );
  await waitFor(() => assert.ok(screen.getByRole('heading', { name: 'Sign in' })));
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run hooks/useAuth.test.tsx components/RequireAuth.test.tsx`
Expected: FAIL — cannot resolve `./useAuth` / `./RequireAuth`.

- [ ] **Step 3: Write `useAuth.tsx`**

```tsx
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import {
  AuthUser,
  completeSetup as apiCompleteSetup,
  fetchMe,
  fetchSetupStatus,
  login as apiLogin,
  logout as apiLogout,
  onUnauthorized,
} from '../services/api';

export type AuthStatus = 'loading' | 'setup' | 'anon' | 'authed';

interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  finishSetup: (token: string, email: string, password: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

/**
 * Owns the session state machine: loading → setup | anon | authed.
 * One probe of /api/auth/me on mount decides the branch; a 401 from any
 * later data request drops back to anon via the api module's hook.
 */
export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [status, setStatus] = useState<AuthStatus>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);

  useEffect(() => {
    onUnauthorized(() => {
      setUser(null);
      setStatus('anon');
    });
    return () => onUnauthorized(null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const me = await fetchMe();
        if (cancelled) return;
        if (me) {
          setUser(me);
          setStatus('authed');
          return;
        }
        const needsSetup = await fetchSetupStatus();
        if (!cancelled) setStatus(needsSetup ? 'setup' : 'anon');
      } catch {
        // The daemon is unreachable; the login page will surface the error
        // on submit rather than blocking the whole app on a spinner.
        if (!cancelled) setStatus('anon');
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    const me = await apiLogin(email, password);
    setUser(me);
    setStatus('authed');
  }, []);

  const signOut = useCallback(async () => {
    await apiLogout();
    setUser(null);
    setStatus('anon');
  }, []);

  const finishSetup = useCallback(async (token: string, email: string, password: string) => {
    const me = await apiCompleteSetup(token, email, password);
    setUser(me);
    setStatus('authed');
  }, []);

  const value = useMemo(
    () => ({ status, user, signIn, signOut, finishSetup }),
    [status, user, signIn, signOut, finishSetup],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
```

- [ ] **Step 4: Write `RequireAuth.tsx`**

```tsx
import React from 'react';
import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { useAuth } from '../hooks/useAuth';
import { Skeleton } from './ui/skeleton';

/** Route guard. Renders the nested routes only once a session is confirmed. */
export const RequireAuth: React.FC = () => {
  const { status } = useAuth();
  const location = useLocation();

  if (status === 'loading') {
    return (
      <div role="status" aria-live="polite" className="app-backdrop flex min-h-screen items-center justify-center p-6">
        <Skeleton className="h-8 w-48" />
        <span className="sr-only">Checking your session…</span>
      </div>
    );
  }
  if (status === 'setup') return <Navigate to="/setup" replace />;
  if (status === 'anon') return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  return <Outlet />;
};
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd web && npx vitest run hooks/useAuth.test.tsx components/RequireAuth.test.tsx`
Expected: 5 PASS.

- [ ] **Step 6: Commit**

```bash
git add web/hooks/useAuth.tsx web/hooks/useAuth.test.tsx web/components/RequireAuth.tsx web/components/RequireAuth.test.tsx
git commit -m "feat(web): auth provider state machine and route guard"
```

---

### Task 12: Login and Setup pages

**Files:**
- Create: `web/components/ui/label.tsx`, `web/pages/Login.tsx`, `web/pages/Setup.tsx`
- Test: `web/pages/Login.test.tsx`

**Interfaces:**
- Consumes: `useAuth()` (`status`, `signIn`, `finishSetup`), `Card*`, `Input`, `Button`.
- Produces: default-exported `Login` and `Setup` page components; `Label` (`React.ComponentProps<'label'>`).

- [ ] **Step 1: Write the failing test**

```tsx
import assert from 'node:assert/strict';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, test, vi } from 'vitest';

const auth = vi.hoisted(() => ({
  status: 'anon' as const,
  user: null,
  signIn: vi.fn(),
  signOut: vi.fn(),
  finishSetup: vi.fn(),
}));
vi.mock('../hooks/useAuth', () => ({ useAuth: () => auth }));

import Login from './Login';

afterEach(() => vi.clearAllMocks());

const renderLogin = () =>
  render(
    <MemoryRouter initialEntries={['/login']}>
      <Login />
    </MemoryRouter>,
  );

test('has labelled email and password fields', () => {
  renderLogin();
  assert.ok(screen.getByLabelText('Email'));
  assert.ok(screen.getByLabelText('Password'));
});

test('submits the entered credentials', async () => {
  auth.signIn.mockResolvedValue(undefined);
  renderLogin();
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'ops@example.com' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'correct horse battery' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await waitFor(() => assert.deepEqual(auth.signIn.mock.calls[0], ['ops@example.com', 'correct horse battery']));
});

test('announces a failed sign-in and links it to the fields', async () => {
  auth.signIn.mockRejectedValue(new Error('Invalid email or password'));
  renderLogin();
  fireEvent.change(screen.getByLabelText('Email'), { target: { value: 'ops@example.com' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  const alert = await screen.findByRole('alert');
  assert.match(alert.textContent ?? '', /Invalid email or password/);
  assert.equal(screen.getByLabelText('Password').getAttribute('aria-invalid'), 'true');
  assert.equal(screen.getByLabelText('Password').getAttribute('aria-describedby'), alert.id);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run pages/Login.test.tsx`
Expected: FAIL — cannot resolve `./Login`.

- [ ] **Step 3: Write `ui/label.tsx`**

```tsx
import * as React from 'react';

import { cn } from '../../lib/utils';

/** Native <label>: it already associates with its control via htmlFor, so no
 *  Radix wrapper is needed. */
function Label({ className, ...props }: React.ComponentProps<'label'>) {
  return (
    <label
      className={cn('text-sm leading-none font-medium select-none peer-disabled:cursor-not-allowed peer-disabled:opacity-60', className)}
      {...props}
    />
  );
}

export { Label };
```

- [ ] **Step 4: Write `Login.tsx`**

```tsx
import React, { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Activity } from 'lucide-react';

import { useAuth } from '../hooks/useAuth';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';

const Login: React.FC = () => {
  const { status, signIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from || '/';

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const errorRef = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    if (status === 'authed') navigate(from, { replace: true });
    if (status === 'setup') navigate('/setup', { replace: true });
  }, [status, navigate, from]);

  useEffect(() => {
    if (error) errorRef.current?.focus();
  }, [error]);

  const onSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setPending(true);
    setError(null);
    try {
      await signIn(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign-in failed');
    } finally {
      setPending(false);
    }
  };

  const describedBy = error ? 'login-error' : undefined;

  return (
    <main className="app-backdrop flex min-h-screen items-center justify-center p-4">
      <Card className="elevated w-full max-w-sm">
        <CardHeader className="flex-col items-start gap-1">
          <div className="text-primary flex items-center gap-2">
            <Activity className="size-5" aria-hidden="true" />
            <span className="text-sm font-semibold tracking-wide">SysSentient</span>
          </div>
          <CardTitle className="text-lg">Sign in</CardTitle>
          <CardDescription>Monitoring console</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} noValidate className="grid gap-4" aria-busy={pending}>
            <div className="grid gap-1.5">
              <Label htmlFor="login-email">Email</Label>
              <Input
                id="login-email"
                type="email"
                autoComplete="email"
                required
                autoFocus
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                aria-invalid={error ? true : undefined}
                aria-describedby={describedBy}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="login-password">Password</Label>
              <Input
                id="login-password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                aria-invalid={error ? true : undefined}
                aria-describedby={describedBy}
              />
            </div>
            {error && (
              <p id="login-error" ref={errorRef} tabIndex={-1} role="alert" className="text-crit text-sm">
                {error}
              </p>
            )}
            <Button type="submit" disabled={pending} className="mt-1">
              {pending ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
};

export default Login;
```

- [ ] **Step 5: Write `Setup.tsx`**

Same shell as Login, with fields **Setup token** (`id="setup-token"`, `autoComplete="off"`, helper text `id="setup-token-hint"`: "Printed once in the daemon log at startup."), **Email** (`autoComplete="email"`), **Password** (`autoComplete="new-password"`, helper `id="setup-password-hint"`: "At least 12 characters."), **Confirm password** (`autoComplete="new-password"`). Client-side checks before calling `finishSetup(token, email, password)`: passwords match, length ≥ 12; failures render into the same `role="alert"` paragraph with `id="setup-error"` and set `aria-invalid` on the offending field(s). `aria-describedby` on each input lists its hint id plus `setup-error` when present. On `status === 'authed'` navigate to `/`; on `status === 'anon'` (setup already done) navigate to `/login`. Title "Create the first admin", description "This account owns the console. You can add more users afterwards." Submit label "Create admin" / "Creating…".

- [ ] **Step 6: Run test to verify it passes**

Run: `cd web && npx vitest run pages/Login.test.tsx && npm run typecheck`
Expected: 3 PASS, typecheck clean.

- [ ] **Step 7: Commit**

```bash
git add web/components/ui/label.tsx web/pages/Login.tsx web/pages/Setup.tsx web/pages/Login.test.tsx
git commit -m "feat(web): login and first-run setup pages"
```

---

### Task 13: user menu, app shell, routing

**Files:**
- Create: `web/components/ui/dropdown-menu.tsx`, `web/components/UserMenu.tsx`
- Modify: `web/components/AppShell.tsx:42-60` (props), header block ~line 92 (render menu), `web/App.tsx`, `web/App.test.tsx:10-16` (mock block)

**Interfaces:**
- Consumes: `@radix-ui/react-dropdown-menu` (already installed), `useAuth`, `AuthUser`, `RequireAuth`, `Login`, `Setup`.
- Produces: `UserMenu({user: AuthUser; onSignOut: () => void})`; `AppShell` gains props `user: AuthUser | null; onSignOut: () => void`.

- [ ] **Step 1: Update the App test mock so it fails for the right reason**

In `App.test.tsx` extend the `vi.mock('./services/api', …)` factory with:
```ts
  fetchMe: vi.fn(async () => ({ id: 'u1', email: 'admin@example.com', role: 'admin' })),
  fetchSetupStatus: vi.fn(async () => false),
  onUnauthorized: vi.fn(),
  logout: vi.fn(async () => undefined),
  login: vi.fn(),
  completeSetup: vi.fn(),
  fetchActiveAlerts: vi.fn(async () => []),
  fetchHosts: vi.fn(async () => []),
```
(Keep any of these that already exist.) Add one test:
```tsx
  test('shows the signed-in user in the header', async () => {
    render(<App />);
    await screen.findByRole('button', { name: /account menu for admin@example.com/i });
  });
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run App.test.tsx -t 'signed-in user'`
Expected: FAIL — no such button.

- [ ] **Step 3: Write `ui/dropdown-menu.tsx`** (shadcn wrapper)

```tsx
import * as React from 'react';
import * as DropdownMenuPrimitive from '@radix-ui/react-dropdown-menu';

import { cn } from '../../lib/utils';

const DropdownMenu = DropdownMenuPrimitive.Root;
const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;

function DropdownMenuContent({ className, sideOffset = 6, ...props }: React.ComponentProps<typeof DropdownMenuPrimitive.Content>) {
  return (
    <DropdownMenuPrimitive.Portal>
      <DropdownMenuPrimitive.Content
        sideOffset={sideOffset}
        className={cn(
          'bg-popover text-popover-foreground elevated z-50 min-w-[12rem] rounded-md border p-1 shadow-md',
          'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
          className,
        )}
        {...props}
      />
    </DropdownMenuPrimitive.Portal>
  );
}

function DropdownMenuItem({ className, ...props }: React.ComponentProps<typeof DropdownMenuPrimitive.Item>) {
  return (
    <DropdownMenuPrimitive.Item
      className={cn(
        'focus:bg-accent focus:text-accent-foreground relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none',
        'data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
        className,
      )}
      {...props}
    />
  );
}

function DropdownMenuLabel({ className, ...props }: React.ComponentProps<typeof DropdownMenuPrimitive.Label>) {
  return <DropdownMenuPrimitive.Label className={cn('px-2 py-1.5 text-xs', className)} {...props} />;
}

function DropdownMenuSeparator({ className, ...props }: React.ComponentProps<typeof DropdownMenuPrimitive.Separator>) {
  return <DropdownMenuPrimitive.Separator className={cn('bg-border -mx-1 my-1 h-px', className)} {...props} />;
}

export { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator };
```

- [ ] **Step 4: Write `UserMenu.tsx`**

```tsx
import React from 'react';
import { Link } from 'react-router-dom';
import { KeyRound, LogOut, UserRound } from 'lucide-react';

import { AuthUser } from '../services/api';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu';

export const UserMenu: React.FC<{ user: AuthUser; onSignOut: () => void }> = ({ user, onSignOut }) => (
  <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button variant="ghost" size="sm" aria-label={`Account menu for ${user.email}`} className="gap-2">
        <UserRound className="size-4" aria-hidden="true" />
        <span className="hidden max-w-[14rem] truncate sm:inline">{user.email}</span>
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuLabel className="flex items-center justify-between gap-3">
        <span className="truncate">{user.email}</span>
        <Badge variant="outline" className="px-1.5 py-0 text-[10px] uppercase">{user.role}</Badge>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuItem asChild>
        <Link to="/settings#account">
          <KeyRound className="size-4" aria-hidden="true" />
          Change password
        </Link>
      </DropdownMenuItem>
      <DropdownMenuItem onSelect={onSignOut}>
        <LogOut className="size-4" aria-hidden="true" />
        Sign out
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
);
```

- [ ] **Step 5: Thread it through `AppShell`**

Add to `Props`: `user: AuthUser | null; onSignOut: () => void;` (import `AuthUser` from `../services/api` and `UserMenu` from `./UserMenu`). Destructure both. Immediately after the host `<Select>` block (the `hosts.length > 1 ? … : …` expression) render:
```tsx
            {user && <UserMenu user={user} onSignOut={onSignOut} />}
```

- [ ] **Step 6: Rewrite `App.tsx` routing**

```tsx
import React from 'react';
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';

import AppShell from './components/AppShell';
import ErrorBoundary from './components/ErrorBoundary';
import { RequireAuth } from './components/RequireAuth';
import { AuthProvider, useAuth } from './hooks/useAuth';
import { DashboardProvider, useDashboard } from './hooks/useDashboardData';
import { formatDuration } from './lib/utils';

import Overview from './pages/Overview';
import Processes from './pages/Processes';
import Logs from './pages/Logs';
import Insights from './pages/Insights';
import Alerts from './pages/Alerts';
import Settings from './pages/Settings';
import Login from './pages/Login';
import Setup from './pages/Setup';

/** Reads shared state so the shell can render feed status in the header. */
const Shell: React.FC = () => {
  const { feed, current, hosts, selectedHost, selectHost, firingAlerts } = useDashboard();
  const { user, signOut } = useAuth();
  return (
    <AppShell
      feed={feed}
      hostname={current.hostname || 'unknown host'}
      uptimeLabel={formatDuration(current.uptimeSeconds)}
      hosts={hosts}
      selectedHost={selectedHost}
      onSelectHost={selectHost}
      firingAlerts={firingAlerts}
      user={user}
      onSignOut={() => void signOut()}
    />
  );
};

/** The live data feed only starts once a session exists — otherwise the
 *  socket and pollers would spin on 401s behind the login page. */
const Console: React.FC = () => (
  <DashboardProvider>
    <Shell />
  </DashboardProvider>
);

const App: React.FC = () => (
  <ErrorBoundary>
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/setup" element={<Setup />} />
          <Route element={<RequireAuth />}>
            <Route element={<Console />}>
              <Route index element={<Overview />} />
              <Route path="processes" element={<Processes />} />
              <Route path="logs" element={<Logs />} />
              <Route path="insights" element={<Insights />} />
              <Route path="alerts" element={<Alerts />} />
              <Route path="settings" element={<Settings />} />
            </Route>
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  </ErrorBoundary>
);

export default App;
```

- [ ] **Step 7: Run the whole web suite**

Run: `cd web && npm run typecheck && npm test`
Expected: PASS. Existing `App.test.tsx` tests still pass because `fetchMe` resolves to an admin before the dashboard mounts; if a test now needs to `await` the header before asserting, add `await screen.findByRole('button', { name: /account menu/i })` at its top rather than lengthening timeouts.

- [ ] **Step 8: Commit**

```bash
git add web/components/ui/dropdown-menu.tsx web/components/UserMenu.tsx web/components/AppShell.tsx web/App.tsx web/App.test.tsx
git commit -m "feat(web): authenticated routing and account menu"
```

---

### Task 14: Settings — Users and Account cards

**Files:**
- Modify: `web/pages/Settings.tsx` (append two `<Card>`s inside the grid before the closing `</div>` at line 173)
- Test: `web/pages/Settings.test.tsx`

**Interfaces:**
- Consumes: `useAuth().user`, `fetchUsers`, `createUser`, `deleteUser`, `changePassword`, `ManagedUser`.

- [ ] **Step 1: Write the failing tests**

```tsx
import assert from 'node:assert/strict';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  fetchHealth: vi.fn(async () => null),
  fetchUsers: vi.fn(async () => [
    { id: 'u1', email: 'admin@example.com', role: 'admin', createdAt: '2026-09-02T09:00:00Z', lastLoginAt: null },
    { id: 'u2', email: 'viewer@example.com', role: 'viewer', createdAt: '2026-09-02T09:00:00Z', lastLoginAt: null },
  ]),
  createUser: vi.fn(),
  deleteUser: vi.fn(async () => undefined),
  changePassword: vi.fn(async () => undefined),
}));
vi.mock('../services/api', () => api);

const auth = vi.hoisted(() => ({ user: { id: 'u1', email: 'admin@example.com', role: 'admin' } }));
vi.mock('../hooks/useAuth', () => ({ useAuth: () => auth }));
vi.mock('../hooks/useDashboardData', () => ({
  useDashboard: () => ({ current: { hostname: 'h' }, feed: { label: 'Live', detail: '' }, hosts: [] }),
}));

import Settings from './Settings';

afterEach(() => vi.clearAllMocks());

test('admins see the user list with self and last-admin guards', async () => {
  render(<MemoryRouter><Settings /></MemoryRouter>);
  await screen.findByText('viewer@example.com');
  const deleteSelf = screen.getByRole('button', { name: /delete admin@example.com/i });
  assert.equal(deleteSelf.hasAttribute('disabled'), true);
  fireEvent.click(screen.getByRole('button', { name: /delete viewer@example.com/i }));
  fireEvent.click(screen.getByRole('button', { name: /confirm delete/i }));
  await waitFor(() => assert.deepEqual(api.deleteUser.mock.calls[0], ['u2']));
});

test('viewers do not see user management', async () => {
  auth.user = { id: 'u2', email: 'viewer@example.com', role: 'viewer' };
  render(<MemoryRouter><Settings /></MemoryRouter>);
  await waitFor(() => assert.equal(screen.queryByRole('heading', { name: 'Users' }), null));
  auth.user = { id: 'u1', email: 'admin@example.com', role: 'admin' };
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npx vitest run pages/Settings.test.tsx`
Expected: FAIL — no "Users" content, `fetchUsers` never called.

- [ ] **Step 3: Add the cards**

At the top of `Settings.tsx` import `useAuth`, `fetchUsers`, `createUser`, `deleteUser`, `changePassword`, `ManagedUser`, `Button`, `Input`, `Label`, `Select*`, `Users`/`KeyRound` icons. Add state: `users: ManagedUser[]`, `usersError`, `newEmail`, `newPassword`, `newRole: 'viewer' | 'admin'`, `pendingDelete: string | null`, `currentPw`, `nextPw`, `pwMessage`. Load users in a `useEffect` gated on `user?.role === 'admin'`.

**Users card** (admin only, `id="users"`): heading `<CardTitle>Users</CardTitle>`; a `<table>` with `<th scope="col">` Email / Role / Last sign-in / Actions; each row's delete button has `aria-label={`Delete ${u.email}`}`, is `disabled` when `u.id === user.id` (with `title="You cannot delete your own account"`) or when `u.role === 'admin' && admins === 1` (`title="The last admin cannot be deleted"`). Clicking sets `pendingDelete = u.id`, which swaps the cell to two buttons: **Confirm delete** (`aria-label={`Confirm delete ${u.email}`}`, `variant="destructive"`, calls `deleteUser` then reloads) and **Cancel**. Below the table an "Add user" form: labelled Email (`autoComplete="off"`), Password (`autoComplete="new-password"`, hint "At least 12 characters."), Role `<Select>` (viewer/admin, `aria-label="Role"`), submit "Add user"; errors render in `role="alert"`.

**Account card** (everyone, `id="account"`): heading "Account", the signed-in email + role badge, and a "Change password" form: Current password (`autoComplete="current-password"`), New password (`autoComplete="new-password"`, hint "At least 12 characters."), submit; success text in `role="status"` ("Password updated. Other devices have been signed out."), errors in `role="alert"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd web && npx vitest run pages/Settings.test.tsx && npm run typecheck && npm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/pages/Settings.tsx web/pages/Settings.test.tsx
git commit -m "feat(web): user management and password change in settings"
```

---

### Task 15: end-to-end verification

**Files:** none new. Uses the built binary and a browser.

- [ ] **Step 1: Full gate**

Run:
```bash
cd /home/xyfo/personal_projects/SysSentient
gofmt -l . ; GOTOOLCHAIN=auto go vet ./... && GOTOOLCHAIN=auto go test ./... -race
make lint
cd web && npm audit --audit-level=moderate && npm run typecheck && npm test && npm run build && cd ..
```
Expected: gofmt prints nothing; every command exits 0; golangci-lint reports 0 issues.

- [ ] **Step 2: Rebuild and restart the daemon**

Run:
```bash
pkill -f '^\./sys-daemon' ; sleep 1
go build -o sys-daemon ./cmd/daemon && (./sys-daemon > /tmp/sysd.log 2>&1 &) ; sleep 2
grep -m1 'FIRST-RUN SETUP' /tmp/sysd.log
```
Expected: the log line with `url=http://localhost:8080/setup token=<43 chars>`. Note: the existing `sys-sentient.db` has no `users` rows, so the token is minted on first start after upgrade — this is the migration path the spec promises.

- [ ] **Step 3: Drive the API with curl**

```bash
TOKEN=$(grep -m1 'FIRST-RUN SETUP' /tmp/sysd.log | sed -E 's/.*token=([A-Za-z0-9_-]+).*/\1/')
curl -s localhost:8080/api/auth/setup                                            # {"needsSetup":true}
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/metrics               # 401
curl -s -c /tmp/cj -X POST localhost:8080/api/auth/setup -H 'Content-Type: application/json' \
  -d "{\"token\":\"$TOKEN\",\"email\":\"admin@example.com\",\"password\":\"correct horse battery\"}"   # 201 {"user":…}
curl -s -b /tmp/cj localhost:8080/api/auth/me                                    # {"user":{…"role":"admin"}}
curl -s -b /tmp/cj -o /dev/null -w '%{http_code}\n' localhost:8080/api/metrics   # 200
curl -s -o /dev/null -w '%{http_code}\n' 'localhost:8080/ws/metrics?api_key=x'   # 401
curl -s -b /tmp/cj -X POST localhost:8080/api/users -H 'Content-Type: application/json' \
  -d '{"email":"viewer@example.com","password":"correct horse battery","role":"viewer"}'   # 201
curl -s -c /tmp/cv -X POST localhost:8080/api/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"viewer@example.com","password":"correct horse battery"}'          # 200
curl -s -b /tmp/cv -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/analyze   # 403
curl -s -b /tmp/cj -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/auth/logout  # 204
curl -s -b /tmp/cj -o /dev/null -w '%{http_code}\n' localhost:8080/api/auth/me   # 401
```
Expected: exactly the codes in the comments. Also confirm `grep -c 'Set-Cookie' <(curl -si -X POST localhost:8080/api/auth/login -d '{}' -H 'Content-Type: application/json')` is `0` — a failed login sets no cookie.

- [ ] **Step 4: Drive the browser** (playwright MCP)

1. Navigate to `http://localhost:8080/processes` → lands on `/login` (or `/setup` on a fresh DB). Screenshot.
2. Sign in as `admin@example.com` → redirected back to `/processes`, header shows the account menu, socket goes LIVE, no console errors.
3. Open the account menu → "Sign out" → back on `/login`; `document.cookie` does not contain `sys_session` (it is HttpOnly, so it never did — check the network tab shows `Set-Cookie` with `HttpOnly; SameSite=Strict` and `Max-Age=-1`… i.e. expired, on logout).
4. Settings → Users: add a viewer, delete it; confirm the self-delete button is disabled and explains why in its tooltip.
5. Keyboard only: Tab through the login form, submit with Enter; on a bad password, focus lands on the error text.

- [ ] **Step 5: AI artifact check**

Run: `git status --porcelain | grep -E '\.claude|\.agents|CLAUDE\.md|skills-lock' || echo clean`
Expected: `clean`.

- [ ] **Step 6: Commit** (when the user asks; otherwise report the file list)

```bash
git add -A docs/features
git commit -m "docs(auth): login design spec and implementation plan"
```
