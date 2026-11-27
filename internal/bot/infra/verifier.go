package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/real-uangi/watchover-tg/internal/bot/domain"
)

type Verifier struct{ client *http.Client }

func NewVerifier() domain.Verifier { return &Verifier{client: externalClient(10 * time.Second)} }
func externalClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

var variables = []string{"{{credential}}", "{{telegram_user_id}}", "{{telegram_username}}"}

func validTemplate(s string) bool {
	for _, key := range variables {
		s = strings.ReplaceAll(s, key, "")
	}
	return !strings.Contains(s, "{{") && !strings.Contains(s, "}}")
}
func (v *Verifier) Validate(c domain.RequestConfig) error {
	if c.Method != "GET" && c.Method != "POST" {
		return domain.ErrInvalid
	}
	u, e := url.Parse(c.URL)
	if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" || strings.Contains(u.Host, "{{") || !validTemplate(c.URL) {
		return domain.ErrInvalid
	}
	for k, value := range c.Headers {
		if k == "" || strings.ContainsAny(k, "\r\n:") || strings.ContainsAny(value, "\r\n") || !validTemplate(value) || strings.Contains(k, "{{") || strings.EqualFold(k, "Host") {
			return domain.ErrInvalid
		}
	}
	for key, value := range c.Query {
		if strings.Contains(key, "{{") || !validTemplate(value) {
			return domain.ErrInvalid
		}
	}
	if len(c.Body) > 0 {
		body, e := parseJSON(c.Body)
		if e != nil || !validateBody(body) {
			return domain.ErrInvalid
		}
	}
	if len(c.Rules) == 0 {
		return domain.ErrInvalid
	}
	for _, rule := range c.Rules {
		if !validPointer(rule.Path) {
			return domain.ErrInvalid
		}
		expected, e := parseJSON(rule.Expected)
		if e != nil || !scalar(expected) {
			return domain.ErrInvalid
		}
	}
	for _, path := range []string{c.IdentityPath, c.DisplayNamePath} {
		if path != "" && !validPointer(path) {
			return domain.ErrInvalid
		}
	}
	return nil
}
func validateBody(v any) bool {
	switch x := v.(type) {
	case string:
		return validTemplate(x)
	case []any:
		for _, value := range x {
			if !validateBody(value) {
				return false
			}
		}
	case map[string]any:
		for key, value := range x {
			if strings.Contains(key, "{{") || !validateBody(value) {
				return false
			}
		}
	}
	return true
}
func parseJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if e := decoder.Decode(&value); e != nil {
		return nil, e
	}
	var extra any
	if e := decoder.Decode(&extra); e != io.EOF {
		return nil, domain.ErrInvalid
	}
	return value, nil
}
func scalar(v any) bool {
	switch v.(type) {
	case nil, bool, string, json.Number:
		return true
	}
	return false
}
func validPointer(p string) bool {
	if p == "" {
		return true
	}
	if !strings.HasPrefix(p, "/") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] == '~' {
			if i+1 >= len(p) || (p[i+1] != '0' && p[i+1] != '1') {
				return false
			}
			i++
		}
	}
	return true
}
func pointer(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	for _, part := range strings.Split(p[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch x := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = x[part]
			if !ok {
				return nil, false
			}
		case []any:
			i, e := strconv.Atoi(part)
			if e != nil || i < 0 || i >= len(x) || strconv.Itoa(i) != part {
				return nil, false
			}
			v = x[i]
		default:
			return nil, false
		}
	}
	return v, true
}
func equalScalar(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		aRat, aOK := new(big.Rat).SetString(string(x))
		bRat, bOK := new(big.Rat).SetString(string(y))
		return aOK && bOK && aRat.Cmp(bRat) == 0
	}
	return false
}
func replaceJSON(v any, r *strings.Replacer) any {
	switch x := v.(type) {
	case string:
		return r.Replace(x)
	case []any:
		for i := range x {
			x[i] = replaceJSON(x[i], r)
		}
	case map[string]any:
		for key, value := range x {
			x[key] = replaceJSON(value, r)
		}
	}
	return v
}
func (v *Verifier) Verify(ctx context.Context, c domain.RequestConfig, input domain.VerificationInput) (domain.VerificationResult, error) {
	return v.verify(ctx, c, input, nil)
}
func (v *Verifier) Test(ctx context.Context, c domain.RequestConfig, input domain.VerificationInput) (domain.VerificationTestResult, error) {
	started := time.Now()
	var test domain.VerificationTestResult
	result, err := v.verify(ctx, c, input, &test)
	test.VerificationResult = result
	test.DurationMS = time.Since(started).Milliseconds()
	if errors.Is(err, domain.ErrUnavailable) {
		return test, nil
	}
	return test, err
}
func (v *Verifier) verify(ctx context.Context, c domain.RequestConfig, input domain.VerificationInput, test *domain.VerificationTestResult) (domain.VerificationResult, error) {
	// Saved profiles and administrator tests are validated at their write boundary.
	replacer := strings.NewReplacer(variables[0], input.Credential, variables[1], strconv.FormatInt(input.UserID, 10), variables[2], input.Username)
	u, _ := url.Parse(c.URL)
	// Replace inside each path segment so a credential cannot introduce separators.
	parts := strings.Split(u.EscapedPath(), "/")
	for i, part := range parts {
		decoded, _ := url.PathUnescape(part)
		parts[i] = url.PathEscape(replacer.Replace(decoded))
	}
	u.RawPath = strings.Join(parts, "/")
	u.Path, _ = url.PathUnescape(u.RawPath)
	query := u.Query()
	for key, values := range query {
		for i := range values {
			values[i] = replacer.Replace(values[i])
		}
		query[key] = values
	}
	for key, value := range c.Query {
		query.Set(key, replacer.Replace(value))
	}
	u.RawQuery = query.Encode()
	var body io.Reader
	var rawBody []byte
	if len(c.Body) > 0 {
		value, _ := parseJSON(c.Body)
		raw, e := json.Marshal(replaceJSON(value, replacer))
		if e != nil {
			return domain.VerificationResult{}, domain.ErrInvalid
		}
		rawBody = raw
		body = bytes.NewReader(raw)
	}
	req, e := http.NewRequestWithContext(ctx, c.Method, u.String(), body)
	if e != nil {
		return domain.VerificationResult{}, domain.ErrInvalid
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range c.Headers {
		req.Header.Set(key, replacer.Replace(value))
	}
	if test != nil {
		test.Request = domain.VerificationTestRequest{Method: req.Method, URL: req.URL.String(), Headers: req.Header.Clone(), Body: string(rawBody)}
	}
	response, e := v.client.Do(req)
	if e != nil {
		if test != nil {
			var networkError net.Error
			switch {
			case errors.Is(e, context.DeadlineExceeded) || (errors.As(e, &networkError) && networkError.Timeout()):
				test.Message = "请求外部服务超时"
			case errors.Is(e, context.Canceled):
				test.Message = "请求已取消"
			default:
				test.Message = "连接外部服务失败"
			}
		}
		return domain.VerificationResult{}, domain.ErrUnavailable
	}
	defer response.Body.Close()
	if test == nil && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return domain.VerificationResult{}, domain.ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if test != nil {
		preview := raw
		if len(preview) > 1<<20 {
			preview = preview[:1<<20]
		}
		test.Response = &domain.VerificationTestResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: string(preview), Truncated: len(raw) > 1<<20}
	}
	if e != nil || len(raw) > 1<<20 {
		if test != nil {
			if e != nil {
				test.Message = "读取响应正文失败"
			} else {
				test.Message = "响应正文超过 1 MiB，已截断"
			}
		}
		return domain.VerificationResult{}, domain.ErrUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if test != nil {
			test.Message = "外部服务返回 HTTP " + strconv.Itoa(response.StatusCode)
		}
		return domain.VerificationResult{}, domain.ErrUnavailable
	}
	value, e := parseJSON(raw)
	if e != nil {
		if test != nil {
			test.Message = "响应正文不是有效 JSON"
		}
		return domain.VerificationResult{}, domain.ErrUnavailable
	}
	for _, rule := range c.Rules {
		actual, ok := pointer(value, rule.Path)
		expected, _ := parseJSON(rule.Expected)
		if !ok || !equalScalar(actual, expected) {
			if test != nil {
				test.Message = "未满足通过条件：" + rule.Path
			}
			return domain.VerificationResult{Passed: false}, nil
		}
	}
	result := domain.VerificationResult{Passed: true}
	for _, mapping := range []struct {
		path string
		dest *string
	}{{c.IdentityPath, &result.ExternalID}, {c.DisplayNamePath, &result.DisplayName}} {
		if mapping.path != "" {
			field, ok := pointer(value, mapping.path)
			if !ok {
				if test != nil {
					test.Message = "响应中缺少身份字段：" + mapping.path
				}
				return domain.VerificationResult{}, domain.ErrUnavailable
			}
			switch x := field.(type) {
			case string:
				*mapping.dest = x
			case json.Number:
				*mapping.dest = string(x)
			default:
				if test != nil {
					test.Message = "响应身份字段必须是字符串或数字：" + mapping.path
				}
				return domain.VerificationResult{}, domain.ErrUnavailable
			}
		}
	}
	if test != nil {
		test.Message = "验证通过"
	}
	return result, nil
}
