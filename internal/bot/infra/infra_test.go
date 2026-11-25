package infra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func testRequest() domain.RequestConfig {
	return domain.RequestConfig{Method: "POST", URL: "https://verify.example/check/{{credential}}", Headers: map[string]string{"Authorization": "Bearer static-key"}, Query: map[string]string{"id": "{{telegram_user_id}}"}, Body: json.RawMessage(`{"credential":"{{credential}}","username":"{{telegram_username}}"}`), Rules: []domain.Rule{{Path: "/data/active", Expected: json.RawMessage("true")}, {Path: "/data/count", Expected: json.RawMessage("1")}}, IdentityPath: "/data/id"}
}

func TestVerifierJSONReplacementAndStrictAND(t *testing.T) {
	calls := 0
	v := &Verifier{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.EscapedPath() != "/check/"+url.PathEscape("a\"\\/?&\nvalue") {
			t.Fatal("credential changed path segments", r.URL.EscapedPath())
		}
		if r.URL.Host != "verify.example" || r.URL.Query().Get("id") != "42" {
			t.Fatalf("wrong request target")
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]string
		if e := json.Unmarshal(raw, &body); e != nil {
			t.Fatal(e)
		}
		if body["credential"] != "a\"\\/?&\nvalue" {
			t.Fatal("credential interpolation changed JSON structure")
		}
		return response(`{"data":{"active":true,"count":1.0,"id":9007199254740993}}`), nil
	})}}
	result, e := v.Verify(context.Background(), testRequest(), domain.VerificationInput{Credential: "a\"\\/?&\nvalue", UserID: 42})
	if e != nil || !result.Passed || result.ExternalID != "9007199254740993" || calls != 1 {
		t.Fatalf("result=%+v error=%v calls=%d", result, e, calls)
	}
	v.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"data":{"active":"true","count":1}}`), nil
	})
	result, e = v.Verify(context.Background(), testRequest(), domain.VerificationInput{})
	if e != nil || result.Passed {
		t.Fatal("string was coerced to boolean")
	}
}
func TestVerifierValidationAndPointers(t *testing.T) {
	v := NewVerifier()
	for _, mutate := range []func(*domain.RequestConfig){func(c *domain.RequestConfig) { c.URL = "https://{{credential}}/check" }, func(c *domain.RequestConfig) { c.Rules[0].Expected = json.RawMessage(`{}`) }, func(c *domain.RequestConfig) { c.Rules[0].Path = "/bad~2path" }, func(c *domain.RequestConfig) { c.Body = json.RawMessage(`{"{{credential}}":1}`) }, func(c *domain.RequestConfig) { c.Query["x"] = "{{unknown}}" }} {
		c := testRequest()
		mutate(&c)
		if !errors.Is(v.Validate(c), domain.ErrInvalid) {
			t.Fatal("accepted unsafe config")
		}
	}
	value, _ := parseJSON([]byte(`{"a/b":{"~key":[false,null]}}`))
	got, ok := pointer(value, "/a~1b/~0key/1")
	if !ok || got != nil {
		t.Fatal("JSON pointer escaping failed")
	}
	if _, ok = pointer(value, "/a~1b/~0key/01"); ok {
		t.Fatal("noncanonical array index accepted")
	}
}
func TestVerifierNeverReturnsCredentialInNetworkError(t *testing.T) {
	v := &Verifier{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("network secret credential") })}}
	_, e := v.Verify(context.Background(), testRequest(), domain.VerificationInput{Credential: "secret"})
	if !errors.Is(e, domain.ErrUnavailable) || strings.Contains(e.Error(), "secret") {
		t.Fatal("unsafe error")
	}
}

func TestTelegramWebhookAndKickPayload(t *testing.T) {
	var methods []string
	tg := &Telegram{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.URL.Path)
		var p map[string]any
		if e := json.NewDecoder(r.Body).Decode(&p); e != nil {
			t.Fatal(e)
		}
		if strings.HasSuffix(r.URL.Path, "setWebhook") {
			if p["url"] != "https://watch.example/api/v1/telegram/webhook" || p["secret_token"] != "secret" {
				t.Fatal("wrong webhook payload")
			}
		} else if p["only_if_banned"] != false {
			t.Fatal("kick must allow rejoining")
		}
		return response(`{"ok":true,"result":true}`), nil
	})}}
	if e := tg.Webhook(context.Background(), "123:key", "https://watch.example/api/v1/telegram/webhook", "secret", true); e != nil {
		t.Fatal(e)
	}
	if e := tg.Act(context.Background(), "123:key", domain.MemberAction{Action: "kick", ChatID: -100, UserID: 42}); e != nil {
		t.Fatal(e)
	}
	if len(methods) != 2 || !strings.HasSuffix(methods[1], "unbanChatMember") {
		t.Fatal("kick must use one call")
	}
}
func TestTelegramNetworkAmbiguityAndNoRetry(t *testing.T) {
	calls := 0
	tg := &Telegram{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("https://api.telegram.org/bot123:secret/sendMessage")
	})}}
	_, e := tg.Send(context.Background(), "123:secret", domain.SendMessage{Kind: "text", ChatID: 1, Text: "hello"})
	if !errors.Is(e, domain.ErrUnknown) || strings.Contains(e.Error(), "secret") || calls != 1 {
		t.Fatal("mutating error not safe/unknown")
	}
	_, e = tg.Me(context.Background(), "123:secret")
	if !errors.Is(e, domain.ErrUnavailable) || calls != 2 {
		t.Fatal("read error")
	}
}
func TestTelegramSendDocumentAndRejectBusinessFailure(t *testing.T) {
	tg := &Telegram{client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			t.Fatal(e)
		}
		f, h, e := r.FormFile("document")
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		raw, _ := io.ReadAll(f)
		if string(raw) != "file content" || h.Filename != "note.txt" || r.FormValue("caption") != "caption" {
			t.Fatal("multipart payload")
		}
		return response(`{"ok":false,"description":"upstream secret"}`), nil
	})}}
	_, e := tg.Send(context.Background(), "123:key", domain.SendMessage{ChatID: 1, Kind: "document", Text: "caption", FileName: "note.txt", File: strings.NewReader("file content")})
	if !errors.Is(e, domain.ErrUnavailable) {
		t.Fatal("business failure ignored")
	}
}
func TestTelegramClientsDisableRedirect(t *testing.T) {
	for _, c := range []*http.Client{NewTelegram().(*Telegram).client, NewVerifier().(*Verifier).client} {
		if !errors.Is(c.CheckRedirect(nil, nil), http.ErrUseLastResponse) || c.Timeout <= 0 {
			t.Fatal("unsafe client policy")
		}
	}
}
func TestCipherBindsEncryptedValues(t *testing.T) {
	c, e := NewCipher(&config.Config{EncryptionKey: make([]byte, 32)})
	if e != nil {
		t.Fatal(e)
	}
	encrypted, e := c.encrypt("profile:one", []byte("secret"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(encrypted), "secret") {
		t.Fatal("plaintext ciphertext")
	}
	if _, e = c.decrypt("profile:two", encrypted); !errors.Is(e, domain.ErrUnavailable) {
		t.Fatal("ciphertext not bound to record")
	}
	raw, e := c.decrypt("profile:one", encrypted)
	if e != nil || string(raw) != "secret" {
		t.Fatal("decrypt failed")
	}
}

func dryStore(t *testing.T) (*Store, *string) {
	t.Helper()
	db, e := gorm.Open(postgres.New(postgres.Config{DSN: "host=invalid user=unused dbname=unused sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Discard})
	if e != nil {
		t.Fatal(e)
	}
	var query string
	if e := db.Callback().Create().After("gorm:create").Register("capture", func(tx *gorm.DB) { query = tx.Statement.SQL.String() }); e != nil {
		t.Fatal(e)
	}
	return &Store{db: db}, &query
}
func TestMessageUpsertMaintainsPrivacyAndEcho(t *testing.T) {
	store, query := dryStore(t)
	if err := store.db.Callback().Create().After("gorm:create").Register("assert_privacy", func(tx *gorm.DB) {
		for _, value := range tx.Statement.Vars {
			if text, ok := value.(string); ok && text == "credential" {
				t.Fatal("sensitive content reached database parameters")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	m := domain.Message{ID: "10000000-0000-0000-0000-000000000001", ChatID: -1, TelegramMessageID: 5, Date: time.Now(), Sensitive: true, Text: "credential"}
	if e := store.PutMessage(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	for _, fragment := range []string{"ON CONFLICT", "telegram_message_id <> 0", "bot_messages.sensitive OR EXCLUDED.sensitive", "COALESCE(EXCLUDED.edited_at", "bot_messages.direction = 'out'"} {
		if !strings.Contains(*query, fragment) {
			t.Fatalf("missing %s in %s", fragment, *query)
		}
	}
	if strings.Contains(*query, "credential") {
		t.Fatal("sensitive content in SQL")
	}
	m.TelegramMessageID = 0
	if e := store.PutMessage(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(*query, `ON CONFLICT ("id")`) {
		t.Fatal("unknown sends must remain independent")
	}
}

func TestMemberUpsertOrdersEventsByTime(t *testing.T) {
	store, query := dryStore(t)
	if e := store.PutMember(context.Background(), domain.Member{ChatID: -1, UserID: 42, Status: "member", UpdatedAt: time.Now(), LastUpdateID: 1}); e != nil {
		t.Fatal(e)
	}
	for _, fragment := range []string{"EXCLUDED.updated_at > bot_members.updated_at", "EXCLUDED.updated_at = bot_members.updated_at AND EXCLUDED.last_update_id >= bot_members.last_update_id"} {
		if !strings.Contains(*query, fragment) {
			t.Fatalf("missing event ordering %s", fragment)
		}
	}
	if strings.Contains(*query, "GREATEST") {
		t.Fatal("randomized update id must not be treated as monotonic")
	}
}
