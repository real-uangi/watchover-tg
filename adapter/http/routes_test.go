package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	authapp "github.com/real-uangi/watchover-tg/internal/auth/application"
	authinfra "github.com/real-uangi/watchover-tg/internal/auth/infra"
	"github.com/real-uangi/watchover-tg/internal/bot/application"
	"github.com/real-uangi/watchover-tg/internal/bot/domain"
	botinfra "github.com/real-uangi/watchover-tg/internal/bot/infra"
	"github.com/real-uangi/watchover-tg/internal/shared/config"
)

type routeStore struct {
	domain.Store
	settingsErr error
}

func (s *routeStore) GetSettings(context.Context) (domain.Settings, error) {
	return domain.Settings{BotID: 11, Token: "private-token", WebhookSecret: "webhook-secret", RetentionDays: 30, HasToken: true}, s.settingsErr
}
func (*routeStore) Message(context.Context, string) (domain.Message, error) {
	return domain.Message{ID: "record", FileID: "file", FileName: "page.html", Kind: "document", Date: time.Now()}, nil
}

type routeTelegram struct{ domain.Telegram }

func (*routeTelegram) Download(context.Context, string, domain.Message) (domain.File, error) {
	return domain.File{Body: io.NopCloser(strings.NewReader("<script>bad()</script>")), MIMEType: "text/html", Name: "page.html", Size: 22}, nil
}
func routeEngine(t *testing.T) *gin.Engine {
	t.Helper()
	bot := application.NewService(&routeStore{}, &routeTelegram{}, nil, nil, "https://watch.example")
	return routeEngineWithBot(t, bot)
}
func routeEngineWithBot(t *testing.T, bot *application.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	cfg := &config.Config{Origin: "https://watch.example", SecureCookies: true, AdminPassword: "test-password"}
	auth := authapp.NewService(authinfra.NewVerifier(cfg), authinfra.NewSessions())
	if err := Register(engine, cfg, auth, bot, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>management</html>")}}); err != nil {
		t.Fatal(err)
	}
	return engine
}
func perform(engine *gin.Engine, method, path, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, engine *gin.Engine) *http.Cookie {
	t.Helper()
	w := perform(engine, "POST", "/api/v1/admin/auth/login", `{"password":"test-password"}`, "https://watch.example", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal(cookies)
	}
	c := cookies[0]
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/api/v1/admin" {
		t.Fatal("unsafe session cookie", c)
	}
	return c
}
func TestAdminBoundaryAndLogout(t *testing.T) {
	engine := routeEngine(t)
	if w := perform(engine, "POST", "/api/v1/admin/auth/login", `{"password":"test-password"}`, "https://other.example", nil); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := perform(engine, "GET", "/api/v1/admin/settings", "", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie := login(t, engine)
	w := perform(engine, "GET", "/api/v1/admin/settings", "", "", cookie)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Header())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("private-token")) || bytes.Contains(w.Body.Bytes(), []byte("webhook-secret")) {
		t.Fatal("settings leaked a secret")
	}
	if w := perform(engine, "POST", "/api/v1/admin/auth/logout", `{}`, "https://watch.example", cookie); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := perform(engine, "GET", "/api/v1/admin/settings", "", "", cookie); w.Code != 401 {
		t.Fatal("session remained valid", w.Code)
	}
}
func TestProtocolErrorsAndSPABoundary(t *testing.T) {
	engine := routeEngine(t)
	for _, tc := range []struct {
		method, path, body, origin string
		code                       int
	}{
		{"GET", "/api/v1/unknown", "", "", 404},
		{"POST", "/api/v1/admin/auth/login", "{", "https://watch.example", 400},
		{"POST", "/api/v1/admin/auth/login", strings.Repeat("x", 70<<10), "https://watch.example", 400},
		{"POST", "/api/v1/admin/auth/login", `{"password":"` + strings.Repeat("x", 70<<10) + `"}`, "https://watch.example", 413},
		{"POST", "/api/v1/telegram/webhook", `{"update_id":4}`, "", 403},
	} {
		w := perform(engine, tc.method, tc.path, tc.body, tc.origin, nil)
		if w.Code != tc.code {
			t.Fatalf("%s got %d: %s", tc.path, w.Code, w.Body)
		}
		var envelope struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Code != w.Code {
			t.Fatal("HTTP/code mismatch", w.Body.String())
		}
	}
	if w := perform(engine, "GET", "/settings", "", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "management") {
		t.Fatal("SPA fallback missing")
	}
}
func TestDocumentProxyCannotExecuteOnAdminOrigin(t *testing.T) {
	engine := routeEngine(t)
	cookie := login(t, engine)
	w := perform(engine, "GET", "/api/v1/admin/messages/file?id=record", "", "", cookie)
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatal(w.Code, w.Header())
	}
	if strings.Contains(w.Header().Get("Location"), "token") {
		t.Fatal("token URL leaked")
	}
}
func TestSaveMustExplicitlyProvideEnabled(t *testing.T) {
	engine := routeEngine(t)
	cookie := login(t, engine)
	w := perform(engine, "POST", "/api/v1/admin/chats/save", `{"id":-100}`, "https://watch.example", cookie)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
}

type routeProfileVerifier struct {
	domain.Verifier
	result domain.VerificationTestResult
	calls  int
}

func (v *routeProfileVerifier) Test(context.Context, domain.RequestConfig, domain.VerificationInput) (domain.VerificationTestResult, error) {
	v.calls++
	return v.result, nil
}

func TestProfileDiagnosticsStayInsideAdminBoundary(t *testing.T) {
	v := &routeProfileVerifier{
		Verifier: botinfra.NewVerifier(),
		result: domain.VerificationTestResult{
			Message:    "外部服务返回 HTTP 403",
			DurationMS: 12,
			Request: domain.VerificationTestRequest{
				Method:  "POST",
				URL:     "https://verify.example/check?key=test-credential",
				Headers: map[string][]string{"Authorization": {"Bearer api-key"}},
				Body:    `{"credential":"test-credential"}`,
			},
			Response: &domain.VerificationTestResponse{
				StatusCode: 403,
				Headers:    map[string][]string{"Set-Cookie": {"first=1", "second=2"}},
				Body:       "<html>permission denied</html>",
			},
		},
	}
	// A nil store ensures testing does not read or write persisted business data.
	engine := routeEngineWithBot(t, application.NewService(nil, nil, v, nil, "https://watch.example"))
	cookie := login(t, engine)
	body := `{"profile":{"request":{"method":"POST","url":"https://verify.example/check","rules":[{"path":"/valid","expected":true}]}},"credential":"test-credential","userId":42}`
	for _, tc := range []struct {
		name, body, origin string
		cookie             *http.Cookie
		status             int
	}{
		{"missing session", body, "https://watch.example", nil, 401},
		{"wrong origin", body, "https://other.example", cookie, 403},
		{"invalid config", strings.Replace(body, `"method":"POST"`, `"method":"DELETE"`, 1), "https://watch.example", cookie, 400},
		{"missing credential", strings.Replace(body, `"credential":"test-credential"`, `"credential":""`, 1), "https://watch.example", cookie, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := perform(engine, "POST", "/api/v1/admin/profiles/test", tc.body, tc.origin, tc.cookie)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || v.calls != 0 {
				t.Fatalf("status=%d headers=%v calls=%d", w.Code, w.Header(), v.calls)
			}
			var envelope struct {
				Data any `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || envelope.Data != nil {
				t.Fatal("rejected request exposed diagnostics", err)
			}
		})
	}
	w := perform(engine, "POST", "/api/v1/admin/profiles/test", body, "https://watch.example", cookie)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || v.calls != 1 {
		t.Fatal("external failure was not returned as a test result", w.Code, w.Header(), v.calls)
	}
	var envelope struct {
		Code int                           `json:"code"`
		Data domain.VerificationTestResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	result := envelope.Data
	if envelope.Code != 200 || result.Passed || result.Message != v.result.Message || result.DurationMS != 12 || result.Request.URL != v.result.Request.URL || result.Request.Headers["Authorization"][0] != "Bearer api-key" || result.Request.Body != v.result.Request.Body || result.Response == nil {
		t.Fatalf("incomplete diagnostics: %+v", result)
	}
	if result.Response.StatusCode != 403 || result.Response.Body != v.result.Response.Body || len(result.Response.Headers["Set-Cookie"]) != 2 || result.Response.Truncated {
		t.Fatalf("incomplete external response: %+v", result.Response)
	}

	v.result.Response = nil
	v.result.Message = "外部请求超时"
	w = perform(engine, "POST", "/api/v1/admin/profiles/test", body, "https://watch.example", cookie)
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || w.Code != 200 || envelope.Data.Response != nil || envelope.Data.Message != v.result.Message || v.calls != 2 {
		t.Fatal("missing response was not represented as null", err)
	}
}
