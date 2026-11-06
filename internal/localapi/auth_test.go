package localapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// newTestAuth 创建一个使用临时密钥的鉴权器.
func newTestAuth(t *testing.T) *auth {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := newAuth(filepath.Join(t.TempDir(), "secret"), logger)
	if err != nil {
		t.Fatalf("创建鉴权器失败: %v", err)
	}
	return a
}

// TestTokenMatches 验证令牌比较只接受完全相同的值.
func TestTokenMatches(t *testing.T) {
	if !tokenMatches("abc", "abc") {
		t.Fatal("相同令牌应匹配")
	}
	if tokenMatches("abc", "abd") {
		t.Fatal("不同令牌不应匹配")
	}
	if tokenMatches("abc", "abcd") {
		t.Fatal("长度不同不应匹配")
	}
}

// TestCookieRoundTrip 验证签发的 cookie 能被校验通过.
func TestCookieRoundTrip(t *testing.T) {
	a := newTestAuth(t)
	now := time.Now()

	value, err := a.encodeCookie(cookiePayload{
		Version:   cookieVersion,
		Authority: "127.0.0.1:20008",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("签发 cookie 失败: %v", err)
	}

	payload, ok := a.decodeCookie(value)
	if !ok {
		t.Fatal("合法 cookie 应当通过校验")
	}
	if payload.Authority != "127.0.0.1:20008" {
		t.Fatalf("权威不符: %s", payload.Authority)
	}
}

// TestCookieRejectsTampered 验证篡改载荷或签名都会被拒绝.
func TestCookieRejectsTampered(t *testing.T) {
	a := newTestAuth(t)
	now := time.Now()
	value, err := a.encodeCookie(cookiePayload{
		Version:   cookieVersion,
		Authority: "127.0.0.1:20008",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("签发 cookie 失败: %v", err)
	}

	cases := map[string]string{
		"空值":    "",
		"格式错误":  "garbage",
		"版本不符":  "v2.abc.def",
		"载荷被篡改": tamperPayload(value),
		"签名被截断": value[:len(value)-4],
	}
	for name, candidate := range cases {
		if _, ok := a.decodeCookie(candidate); ok {
			t.Fatalf("%s 的 cookie 不应通过校验", name)
		}
	}
}

// TestCookieRejectsExpired 验证过期 cookie 被拒绝.
func TestCookieRejectsExpired(t *testing.T) {
	a := newTestAuth(t)
	past := time.Now().Add(-2 * time.Hour)

	value, err := a.encodeCookie(cookiePayload{
		Version:   cookieVersion,
		Authority: "127.0.0.1:20008",
		IssuedAt:  past.Unix(),
		ExpiresAt: past.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("签发 cookie 失败: %v", err)
	}
	if _, ok := a.decodeCookie(value); ok {
		t.Fatal("过期 cookie 不应通过校验")
	}
}

// TestCookieRejectsOtherSecret 验证换密钥后旧 cookie 失效.
func TestCookieRejectsOtherSecret(t *testing.T) {
	first := newTestAuth(t)
	second := newTestAuth(t)

	now := time.Now()
	value, err := first.encodeCookie(cookiePayload{
		Version:   cookieVersion,
		Authority: "127.0.0.1:20008",
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("签发 cookie 失败: %v", err)
	}
	if _, ok := second.decodeCookie(value); ok {
		t.Fatal("其他密钥签发的 cookie 不应通过校验")
	}
}

// TestTrustedRequest 验证请求围栏.
func TestTrustedRequest(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		origin  string
		fetch   string
		trusted bool
	}{
		{name: "回环无 Origin", host: "127.0.0.1:20008", trusted: true},
		{name: "localhost", host: "localhost:20008", trusted: true},
		{name: "IPv6 回环", host: "[::1]:20008", trusted: true},
		{name: "同源 Origin", host: "127.0.0.1:20008", origin: "http://127.0.0.1:20008", trusted: true},
		{name: "跨站 Origin", host: "127.0.0.1:20008", origin: "http://evil.example", trusted: false},
		{name: "不透明 Origin", host: "127.0.0.1:20008", origin: "null", trusted: false},
		{name: "cross-site 标记", host: "127.0.0.1:20008", fetch: "cross-site", trusted: false},
		{name: "DNS rebinding", host: "evil.example:20008", trusted: false},
		{name: "空 Host", host: "", trusted: false},
	}
	for _, testCase := range cases {
		request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		request.Host = testCase.host
		if testCase.origin != "" {
			request.Header.Set("Origin", testCase.origin)
		}
		if testCase.fetch != "" {
			request.Header.Set("Sec-Fetch-Site", testCase.fetch)
		}
		if got := trustedRequest(request); got != testCase.trusted {
			t.Fatalf("%s: 期望 %v, 实际 %v", testCase.name, testCase.trusted, got)
		}
	}
}

// TestServeIndexMintsCookie 验证带令牌访问会换发 cookie 并重定向到干净地址.
func TestServeIndexMintsCookie(t *testing.T) {
	a := newTestAuth(t)
	served := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { served = true })

	request := httptest.NewRequest(http.MethodGet, "/settings?token="+a.LaunchToken(), nil)
	request.Host = "127.0.0.1:20008"
	recorder := httptest.NewRecorder()

	a.serveIndex(recorder, request, next)

	if served {
		t.Fatal("换发 cookie 时不应直接渲染页面")
	}
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("应返回 303, 实际 %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "/settings" {
		t.Fatalf("重定向地址应去掉令牌且保留路径, 实际 %q", location)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("应当种下会话 cookie")
	}
	cookie := cookies[0]
	if !cookie.HttpOnly {
		t.Fatal("cookie 必须是 HttpOnly, 防止脚本读取")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("cookie 必须是 SameSite=Strict")
	}
}

// TestServeIndexRejectsBadToken 验证错误令牌被拒绝.
func TestServeIndexRejectsBadToken(t *testing.T) {
	a := newTestAuth(t)
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("不应渲染页面")
	})

	request := httptest.NewRequest(http.MethodGet, "/?token=wrong", nil)
	request.Host = "127.0.0.1:20008"
	recorder := httptest.NewRecorder()

	a.serveIndex(recorder, request, next)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("应返回 401, 实际 %d", recorder.Code)
	}
}

// TestAdmitRequiresCredentials 验证没有凭据的请求被拒绝, 带 Bearer 的放行.
func TestAdmitRequiresCredentials(t *testing.T) {
	a := newTestAuth(t)

	request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	request.Host = "127.0.0.1:20008"
	recorder := httptest.NewRecorder()
	if a.admit(recorder, request) {
		t.Fatal("无凭据请求不应通过")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("应返回 401, 实际 %d", recorder.Code)
	}

	authorized := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	authorized.Host = "127.0.0.1:20008"
	authorized.Header.Set("Authorization", "Bearer "+a.LaunchToken())
	if !a.admit(httptest.NewRecorder(), authorized) {
		t.Fatal("带正确 Bearer 的请求应通过")
	}

	wrong := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	wrong.Host = "127.0.0.1:20008"
	wrong.Header.Set("Authorization", "Bearer wrong")
	if a.admit(httptest.NewRecorder(), wrong) {
		t.Fatal("错误 Bearer 不应通过")
	}
}

// tamperPayload 改动 cookie 载荷但保留原签名, 用于测试签名校验.
func tamperPayload(value string) string {
	for index, char := range value {
		if char == 'v' && index == 0 {
			continue
		}
		// 翻转载荷区第一个不是分隔符的字符.
		if char == '.' {
			continue
		}
		replacement := 'A'
		if char == 'A' {
			replacement = 'B'
		}
		return value[:index] + string(replacement) + value[index+1:]
	}
	return value
}
