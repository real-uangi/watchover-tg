package infra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

func TestVerifierTestCapturesCompleteRequestAndResponse(t *testing.T) {
	input := domain.VerificationInput{Credential: `a"\/?&value`, UserID: 42, Username: "test-user"}
	config := testRequest()
	config.URL += "?credential={{credential}}"
	config.Headers["X-Credential"] = "{{credential}}"
	config.DisplayNamePath = "/data/name"
	rawResponse, err := json.Marshal(map[string]any{"data": map[string]any{"active": true, "count": 1, "id": input.Credential, "name": "external-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	var request domain.VerificationTestRequest
	calls := 0
	v := NewVerifier().(*Verifier)
	v.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		request = domain.VerificationTestRequest{Method: r.Method, URL: r.URL.String(), Headers: r.Header.Clone(), Body: string(raw)}
		if r.URL.Query().Get("credential") != input.Credential || r.URL.Query().Get("id") != "42" {
			t.Fatal("query variables were not interpolated")
		}
		response := response(string(rawResponse))
		response.Header = http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"external-secret=visible", "second=value"}}
		return response, nil
	})
	result, err := v.Test(context.Background(), config, input)
	if err != nil || !result.Passed || calls != 1 || result.ExternalID != input.Credential || result.DisplayName != "external-secret" {
		t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
	}
	if !reflect.DeepEqual(result.Request, request) || result.Request.Headers["Authorization"][0] != "Bearer static-key" || result.Request.Headers["X-Credential"][0] != input.Credential {
		t.Fatalf("request details changed: %+v", result.Request)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(result.Request.Body), &body); err != nil || body["credential"] != input.Credential || body["username"] != input.Username {
		t.Fatalf("request body changed: %q error=%v", result.Request.Body, err)
	}
	if result.Response == nil || result.Response.StatusCode != 200 || result.Response.Body != string(rawResponse) || result.Response.Truncated || !reflect.DeepEqual(result.Response.Headers["Set-Cookie"], []string{"external-secret=visible", "second=value"}) {
		t.Fatalf("response details changed: %+v", result.Response)
	}
}

type failedResponseReader struct{}

func (failedResponseReader) Read([]byte) (int, error) { return 0, errors.New("private read failure") }

func TestVerifierTestExplainsFailuresWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		respond     func() (*http.Response, error)
		message     string
		body        string
		status      int
		truncated   bool
		verifyError error
	}{
		{name: "non-2xx HTML", respond: func() (*http.Response, error) {
			r := response("<html>permission denied</html>")
			r.StatusCode = 403
			r.Header.Set("Content-Type", "text/html")
			return r, nil
		}, message: "HTTP 403", body: "<html>permission denied</html>", status: 403, verifyError: domain.ErrUnavailable},
		{name: "redirect", respond: func() (*http.Response, error) {
			r := response("moved")
			r.StatusCode = 302
			r.Header.Set("Location", "https://elsewhere.example/")
			return r, nil
		}, message: "HTTP 302", body: "moved", status: 302, verifyError: domain.ErrUnavailable},
		{name: "invalid JSON", respond: func() (*http.Response, error) { return response("not JSON"), nil }, message: "不是有效 JSON", body: "not JSON", status: 200, verifyError: domain.ErrUnavailable},
		{name: "response too large", respond: func() (*http.Response, error) { return response(strings.Repeat("a", (1<<20)+1)), nil }, message: "超过 1 MiB", body: strings.Repeat("a", 1<<20), status: 200, truncated: true, verifyError: domain.ErrUnavailable},
		{name: "read failure", respond: func() (*http.Response, error) {
			r := response("")
			r.Body = io.NopCloser(io.MultiReader(strings.NewReader("partial response"), failedResponseReader{}))
			return r, nil
		}, message: "读取响应正文失败", body: "partial response", status: 200, verifyError: domain.ErrUnavailable},
		{name: "timeout", respond: func() (*http.Response, error) { return nil, context.DeadlineExceeded }, message: "超时", verifyError: domain.ErrUnavailable},
		{name: "network", respond: func() (*http.Response, error) { return nil, errors.New("private network secret") }, message: "连接外部服务失败", verifyError: domain.ErrUnavailable},
		{name: "canceled", respond: func() (*http.Response, error) { return nil, context.Canceled }, message: "请求已取消", verifyError: domain.ErrUnavailable},
		{name: "rule mismatch", respond: func() (*http.Response, error) { return response(`{"data":{"active":false}}`), nil }, message: "未满足通过条件：/data/active", body: `{"data":{"active":false}}`, status: 200},
		{name: "missing identity", respond: func() (*http.Response, error) { return response(`{"data":{"active":true,"count":1}}`), nil }, message: "缺少身份字段", body: `{"data":{"active":true,"count":1}}`, status: 200, verifyError: domain.ErrUnavailable},
		{name: "invalid identity", respond: func() (*http.Response, error) { return response(`{"data":{"active":true,"count":1,"id":false}}`), nil }, message: "身份字段必须是字符串或数字", body: `{"data":{"active":true,"count":1,"id":false}}`, status: 200, verifyError: domain.ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			v := NewVerifier().(*Verifier)
			v.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return tc.respond() })
			result, err := v.Test(context.Background(), testRequest(), domain.VerificationInput{Credential: "secret"})
			if err != nil || result.Passed || calls != 1 || !strings.Contains(result.Message, tc.message) || strings.Contains(result.Message, "private") {
				t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
			if tc.status == 0 {
				if result.Response != nil {
					t.Fatalf("unexpected response: %+v", result.Response)
				}
			} else if result.Response == nil || result.Response.StatusCode != tc.status || result.Response.Body != tc.body || result.Response.Truncated != tc.truncated {
				t.Fatalf("response does not match expected status/body/truncation: status=%d response=%+v", tc.status, result.Response)
			}
			calls = 0
			verified, err := v.Verify(context.Background(), testRequest(), domain.VerificationInput{Credential: "secret"})
			if verified.Passed || !errors.Is(err, tc.verifyError) || calls != 1 || (err != nil && (strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private"))) {
				t.Fatalf("ordinary verification changed: result=%+v error=%v calls=%d", verified, err, calls)
			}
		})
	}
}

func TestVerifierTestReturnsLocalRequestErrors(t *testing.T) {
	config := testRequest()
	config.Method = "invalid method"
	v := NewVerifier().(*Verifier)
	v.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached transport")
		return nil, nil
	})
	if _, err := v.Test(context.Background(), config, domain.VerificationInput{Credential: "secret"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("expected local configuration error, got %v", err)
	}
}
