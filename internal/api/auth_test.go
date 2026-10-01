package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"raenil/internal/auth"
	"raenil/internal/config"
	"raenil/internal/store"
)

// ---- PP-194 part 1: client IP ----

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		trusted string
		peer    string
		headers map[string][]string
		want    string
	}{
		{"unset ignores X-Forwarded-For", "", "203.0.113.9:5555",
			map[string][]string{"X-Forwarded-For": {"1.2.3.4"}}, "203.0.113.9"},
		{"unset ignores CF-Connecting-IP", "", "203.0.113.9:5555",
			map[string][]string{"Cf-Connecting-Ip": {"1.2.3.4"}}, "203.0.113.9"},
		{"unset uses the peer", "", "203.0.113.9:5555", nil, "203.0.113.9"},
		{"peer without a port", "", "203.0.113.9", nil, "203.0.113.9"},
		{"IPv6 peer", "", "[2001:db8::1]:443", nil, "2001:db8::1"},
		{"CF header is believed when configured", "CF-Connecting-IP", "10.0.0.2:1",
			map[string][]string{"Cf-Connecting-Ip": {"198.51.100.7"}}, "198.51.100.7"},
		{"CF header configured but absent falls back to the peer", "CF-Connecting-IP", "10.0.0.2:1", nil, "10.0.0.2"},
		{"CF header garbage falls back to the peer", "CF-Connecting-IP", "10.0.0.2:1",
			map[string][]string{"Cf-Connecting-Ip": {"not-an-ip"}}, "10.0.0.2"},
		{"CF configured does not read X-Forwarded-For", "CF-Connecting-IP", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"1.2.3.4"}}, "10.0.0.2"},
		{"XFF takes the right-most entry, never the left-most", "X-Forwarded-For", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"6.6.6.6, 7.7.7.7, 198.51.100.7"}}, "198.51.100.7"},
		{"XFF single entry", "X-Forwarded-For", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"198.51.100.7"}}, "198.51.100.7"},
		{"XFF spread over several header lines", "x-forwarded-for", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"6.6.6.6", "198.51.100.7"}}, "198.51.100.7"},
		{"XFF with a spoofed prefix cannot change the result", "X-Forwarded-For", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"9.9.9.9,198.51.100.7"}}, "198.51.100.7"},
		{"XFF absent falls back to the peer", "X-Forwarded-For", "10.0.0.2:1", nil, "10.0.0.2"},
		{"XFF garbage right-most falls back to the peer", "X-Forwarded-For", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"1.2.3.4, junk"}}, "10.0.0.2"},
		{"XFF IPv6 is normalised", "X-Forwarded-For", "10.0.0.2:1",
			map[string][]string{"X-Forwarded-For": {"2001:DB8::0001"}}, "2001:db8::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/auth/login", nil)
			r.RemoteAddr = tc.peer
			for k, vs := range tc.headers {
				for _, v := range vs {
					r.Header.Add(k, v)
				}
			}
			if got := clientIP(r, tc.trusted); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- PP-194 part 2: limiter cleanup ----

func TestRateLimiterWindowAndSweep(t *testing.T) {
	clock := time.Unix(1_000_000, 0)
	rl := newRateLimiter(3, time.Minute)
	rl.now = func() time.Time { return clock }

	for i := 0; i < 3; i++ {
		if !rl.allow("a") {
			t.Fatalf("hit %d refused", i)
		}
	}
	if rl.allow("a") {
		t.Fatal("fourth hit inside the window allowed")
	}
	if !rl.allow("b") {
		t.Fatal("another key must have its own budget")
	}

	// 10k distinct spoofed keys, as an attacker rotating a header would send.
	for i := 0; i < 10_000; i++ {
		rl.allow(fmt.Sprintf("spoof-%d", i))
	}
	if got := rl.size(); got < 10_000 {
		t.Fatalf("size = %d, want >= 10000", got)
	}

	// Sweeping inside the window keeps live keys.
	clock = clock.Add(30 * time.Second)
	rl.sweep()
	if got := rl.size(); got < 10_000 {
		t.Fatalf("sweep inside the window dropped live keys: size %d", got)
	}

	// Two idle minutes later nothing is live, so nothing is kept.
	clock = clock.Add(2 * time.Minute)
	rl.sweep()
	if got := rl.size(); got != 0 {
		t.Fatalf("size after idle sweep = %d, want 0", got)
	}
	if !rl.allow("a") {
		t.Fatal("key must be allowed again once its hits expire")
	}
}

// The sweeper goroutine itself empties an idle limiter, with no request needed.
func TestRateLimiterRunSweepsIdleKeys(t *testing.T) {
	rl := newRateLimiter(10, 20*time.Millisecond)
	for i := 0; i < 1000; i++ {
		rl.allow(fmt.Sprintf("k%d", i))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rl.run(ctx, 5*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for rl.size() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("idle keys not swept: size %d", rl.size())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSweepIntervalIsOneMinute(t *testing.T) {
	if sweepInterval != time.Minute {
		t.Fatalf("sweepInterval = %v, want 1m", sweepInterval)
	}
}

// Login through the whole stack: the limiter key comes from the trusted header
// or the peer, never from a header the client controls.
func TestLoginRateLimitKey(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Config
		// opts builds request i (0-based) of a flood from one real client.
		opts func(i int) []reqOpt
		// wantLimitedAfter: attempts allowed before the first 429, -1 = never.
		wantLimitedAfter int
	}{
		{"header unset: rotating X-Forwarded-For is still limited by the peer", config.Config{},
			func(i int) []reqOpt {
				return []reqOpt{remote("198.51.100.1:4000"), header("X-Forwarded-For", fmt.Sprintf("10.0.%d.%d", i/250, i%250))}
			}, 10},
		{"header unset: rotating CF-Connecting-IP is still limited by the peer", config.Config{},
			func(i int) []reqOpt {
				return []reqOpt{remote("198.51.100.2:4000"), header("CF-Connecting-IP", fmt.Sprintf("10.1.0.%d", i))}
			}, 10},
		{"header unset: different peers do not share a budget", config.Config{},
			func(i int) []reqOpt { return []reqOpt{remote(fmt.Sprintf("198.51.101.%d:4000", i))} }, -1},
		{"CF header configured: one visitor behind the proxy is limited", config.Config{TrustedProxyHeader: "CF-Connecting-IP"},
			func(i int) []reqOpt {
				return []reqOpt{remote("127.0.0.1:4000"), header("CF-Connecting-IP", "198.51.100.50")}
			}, 10},
		{"CF header configured: distinct visitors each get a budget", config.Config{TrustedProxyHeader: "CF-Connecting-IP"},
			func(i int) []reqOpt {
				return []reqOpt{remote("127.0.0.1:4000"), header("CF-Connecting-IP", fmt.Sprintf("198.51.102.%d", i))}
			}, -1},
		{"XFF configured: spoofed left-most entries do not help", config.Config{TrustedProxyHeader: "X-Forwarded-For"},
			func(i int) []reqOpt {
				return []reqOpt{remote("127.0.0.1:4000"), header("X-Forwarded-For", fmt.Sprintf("10.2.0.%d, 198.51.100.60", i))}
			}, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, tc.cfg)
			firstLimited := -1
			for i := 0; i < 14; i++ {
				w := e.do("POST", "/api/auth/login",
					map[string]string{"email": "nobody@example.test", "password": "wrong-password"}, tc.opts(i)...)
				if w.Code == http.StatusTooManyRequests {
					firstLimited = i
					break
				}
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("attempt %d = %d, want 401 or 429", i, w.Code)
				}
			}
			if firstLimited != tc.wantLimitedAfter {
				t.Fatalf("first 429 at attempt %d, want %d", firstLimited, tc.wantLimitedAfter)
			}
		})
	}
}

// ---- PP-194 part 3: login miss timing ----

func TestLoginMissIsIndistinguishable(t *testing.T) {
	e := newEnv(t, config.Config{})
	_, email := e.user("correct-horse-battery")

	var verified []string
	e.srv.verifyPassword = func(hash, pw string) bool {
		verified = append(verified, hash)
		return auth.VerifyPassword(hash, pw)
	}

	login := func(email, pw string) *httptest.ResponseRecorder {
		return e.do("POST", "/api/auth/login", map[string]string{"email": email, "password": pw})
	}
	unknown := login("nobody-"+uniq()+"@example.test", "correct-horse-battery")
	wrong := login(email, "not-the-password")

	if unknown.Code != 401 || wrong.Code != 401 {
		t.Fatalf("status unknown=%d wrong=%d, want both 401", unknown.Code, wrong.Code)
	}
	if unknown.Body.String() != wrong.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", unknown.Body.String(), wrong.Body.String())
	}
	if errBody(unknown) != "invalid_credentials" {
		t.Fatalf("error = %q, want invalid_credentials", errBody(unknown))
	}
	if unknown.Header().Get("Set-Cookie") != "" || wrong.Header().Get("Set-Cookie") != "" {
		t.Fatal("a failed login set a cookie")
	}

	// The unknown email must still have cost one real argon2 verification, of a
	// hash that parses (so it is not an instant reject).
	if len(verified) != 2 {
		t.Fatalf("verifications = %d, want 2 (one per attempt)", len(verified))
	}
	if verified[0] != loginDummyHash() {
		t.Fatal("unknown email was not verified against the dummy hash")
	}
	if parts := strings.Split(loginDummyHash(), "$"); len(parts) != 6 || parts[1] != "argon2id" {
		t.Fatalf("dummy hash is not a full argon2id hash: %q", loginDummyHash())
	}
	if !auth.VerifyPassword(loginDummyHash(), dummyPassword) {
		t.Fatal("dummy hash does not verify, so verification would not run to completion")
	}

	// Sanity: the right password still logs in and sets the session cookies.
	ok := login(email, "correct-horse-battery")
	if ok.Code != 200 {
		t.Fatalf("valid login = %d (%s)", ok.Code, ok.Body.String())
	}
	var names []string
	for _, c := range ok.Result().Cookies() {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != auth.SessionCookie+","+auth.CSRFCookie {
		t.Fatalf("cookies = %v", names)
	}
}

// ---- PP-194 part 4: setup race ----

// tempDatabase makes an empty, migrated database so setup (which only works
// while the users table is empty) can be tested without touching real data.
func tempDatabase(t *testing.T) string {
	t.Helper()
	base := testDSN(t)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	name := "raenil_setup_" + uniq()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Skipf("cannot create a scratch database (needs CREATEDB): %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, base)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func TestSetupCreatesExactlyOneUserUnderConcurrency(t *testing.T) {
	dsn := tempDatabase(t)
	e := newEnvOn(t, dsn, config.Config{})

	const n = 8
	codes := make([]int, n)
	bodies := make([]string, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			w := e.do("POST", "/api/auth/setup", map[string]string{
				"email": fmt.Sprintf("first%d@example.test", i), "password": "a-strong-password"})
			codes[i], bodies[i] = w.Code, w.Body.String()
		}(i)
	}
	close(start)
	wg.Wait()

	created, conflicts := 0, 0
	for i, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
			if !strings.Contains(bodies[i], "already_set_up") {
				t.Fatalf("409 body = %s, want already_set_up", bodies[i])
			}
		default:
			t.Fatalf("request %d = %d (%s), want 201 or 409", i, c, bodies[i])
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Fatalf("created=%d conflicts=%d, want 1 and %d", created, conflicts, n-1)
	}
	var users int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("users = %d (err %v), want exactly 1", users, err)
	}

	// Later attempts are refused too, and the first user can log in.
	if w := e.do("POST", "/api/auth/setup", map[string]string{"email": "late@example.test", "password": "a-strong-password"}); w.Code != 409 {
		t.Fatalf("late setup = %d, want 409", w.Code)
	}
}

func TestSetupValidationAndSession(t *testing.T) {
	e := newEnvOn(t, tempDatabase(t), config.Config{})

	bad := e.do("POST", "/api/auth/setup", map[string]string{"email": "a@example.test", "password": "short"})
	if bad.Code != 400 {
		t.Fatalf("short password = %d, want 400", bad.Code)
	}
	ok := e.do("POST", "/api/auth/setup", map[string]string{"email": "a@example.test", "password": "long-enough-pw"})
	if ok.Code != 201 {
		t.Fatalf("setup = %d (%s)", ok.Code, ok.Body.String())
	}
	var haveSession bool
	for _, c := range ok.Result().Cookies() {
		if c.Name == auth.SessionCookie && c.Value != "" {
			haveSession = true
		}
	}
	if !haveSession {
		t.Fatal("setup answered 201 without a session cookie")
	}
}

// The store call alone must be race-free, with no hashing between requests to
// spread them out: many callers released together, exactly one wins.
func TestCreateFirstUserIsAtomic(t *testing.T) {
	e := newEnvOn(t, tempDatabase(t), config.Config{})
	const n = 32
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = e.store.CreateFirstUser(context.Background(), fmt.Sprintf("u%d@example.test", i), "x")
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for i, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, store.ErrConflict):
		default:
			t.Fatalf("caller %d: unexpected error %v", i, err)
		}
	}
	var users int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if wins != 1 || users != 1 {
		t.Fatalf("wins=%d users=%d, want exactly 1 each", wins, users)
	}
}

// grantSession failing must surface as a 500, never as success without a cookie.
func TestGrantSessionErrorIsReturned(t *testing.T) {
	e := newEnv(t, config.Config{})
	// A user id that is not a uuid makes the session insert fail in Postgres.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/", nil)
	if err := e.srv.grantSession(w, r, "not-a-uuid"); err == nil {
		t.Fatal("grantSession swallowed the error")
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("grantSession set cookies despite failing")
	}
}
