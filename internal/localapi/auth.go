// Package localapi 实现本地 HTTP 服务: Web 界面, REST 接口与 SSE.
//
// 鉴权沿用 DeepSeek Harness 的做法, 分三层:
//  1. 进程启动令牌 (launch token): 每次启动随机生成, 只通过命令行与 runtime.json 传递,
//     浏览器首次带 ?token= 访问时换发签名 cookie, 随后重定向到干净地址.
//  2. 签名 cookie: 用持久化的 HMAC 密钥签名, 载荷绑定 Host 权威与有效期,
//     因此别的站点即使拿到 cookie 也不能跨权威使用, 且 cookie 本身不可伪造.
//  3. 请求围栏: 校验 Host 必须是回环地址 (防 DNS rebinding), 拒绝 cross-site 的
//     Sec-Fetch-Site, 并校验 Origin 与 Host 同源.
//
// 三层叠加后, 本机其他进程即使知道端口也无法调用接口: 它们既拿不到进程内令牌,
// 也伪造不出签名 cookie.
package localapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 令牌与 cookie 参数.
const (
	// tokenBytes 是启动令牌与签名密钥的字节数.
	tokenBytes = 32
	// tokenQuery 是首次访问携带令牌的查询参数名.
	tokenQuery = "token"
	// cookiePrefix 是签名 cookie 名的前缀, 后接权威的哈希.
	cookiePrefix = "scherun-auth-"
	// cookieMaxAge 是签名 cookie 的有效期.
	cookieMaxAge = 7 * 24 * time.Hour
	// cookieVersion 是 cookie 载荷版本.
	cookieVersion = 1
)

// auth 负责令牌生成, cookie 签名与请求围栏.
type auth struct {
	launchToken string
	secret      []byte
	logger      *slog.Logger
}

// cookiePayload 是签名 cookie 里携带的内容.
type cookiePayload struct {
	Version   int    `json:"v"`
	Authority string `json:"a"`
	IssuedAt  int64  `json:"i"`
	ExpiresAt int64  `json:"e"`
}

// newAuth 创建鉴权器.
//
// 签名密钥持久化在数据目录, 因此浏览器 cookie 在应用重启后仍然有效;
// 启动令牌每次启动都重新生成, 旧链接自然失效.
func newAuth(secretPath string, logger *slog.Logger) (*auth, error) {
	secret, err := loadOrCreateSecret(secretPath)
	if err != nil {
		return nil, err
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	return &auth{launchToken: token, secret: secret, logger: logger}, nil
}

// LaunchToken 返回本次进程的启动令牌.
func (a *auth) LaunchToken() string { return a.launchToken }

// randomToken 生成 base64url 编码的随机串.
func randomToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机令牌失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// loadOrCreateSecret 读取或创建 cookie 签名密钥.
func loadOrCreateSecret(path string) ([]byte, error) {
	if content, err := os.ReadFile(path); err == nil {
		if decoded, decErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(content))); decErr == nil && len(decoded) == tokenBytes {
			return decoded, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建密钥目录失败: %w", err)
	}
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("生成签名密钥失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(buf)), 0o600); err != nil {
		return nil, fmt.Errorf("写入签名密钥失败: %w", err)
	}
	return buf, nil
}

// tokenMatches 常量时间比较两个令牌.
func tokenMatches(actual, expected string) bool {
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

// authority 返回请求的 Host 权威 (含端口).
func authority(r *http.Request) string {
	return r.Host
}

// cookieName 由权威派生 cookie 名, 避免不同端口互相覆盖.
func cookieName(host string) string {
	sum := sha256.Sum256([]byte(host))
	return cookiePrefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

// sign 用 HMAC-SHA256 签名.
func (a *auth) sign(body string) []byte {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

// encodeCookie 生成 v1.<payload>.<signature> 形式的 cookie 值.
func (a *auth) encodeCookie(payload cookiePayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化 cookie 载荷失败: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := base64.RawURLEncoding.EncodeToString(a.sign(body))
	return fmt.Sprintf("v%d.%s.%s", payload.Version, body, sig), nil
}

// decodeCookie 校验并解析 cookie.
func (a *auth) decodeCookie(value string) (*cookiePayload, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return nil, false
	}
	expected := a.sign(parts[1])
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(actual, expected) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var payload cookiePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	if payload.Version != cookieVersion {
		return nil, false
	}
	now := time.Now().Unix()
	if payload.IssuedAt > now || payload.ExpiresAt <= now || payload.ExpiresAt <= payload.IssuedAt {
		return nil, false
	}
	return &payload, true
}

// cookieValue 从 Cookie 头里取出指定名字的值.
func cookieValue(header, name string) (string, bool) {
	for _, segment := range strings.Split(header, ";") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == name {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

// isLoopbackHost 判断 Host 是否为回环地址.
//
// 这是防 DNS rebinding 的关键: 浏览器把 Host 填成它以为在访问的域名,
// 所以被 rebind 的页面会带着攻击者的域名过来, 即使连接落到了本服务.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// trustedRequest 执行请求围栏检查.
func trustedRequest(r *http.Request) bool {
	if !isLoopbackHost(r.Host) {
		return false
	}
	// 明确标记为跨站的请求一律拒绝.
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	// "null" 是不透明来源 (沙盒 iframe, file: 页面), 必须拒绝.
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

// authenticated 判断请求是否已通过 cookie 鉴权.
func (a *auth) authenticated(r *http.Request) bool {
	raw, ok := cookieValue(r.Header.Get("Cookie"), cookieName(authority(r)))
	if !ok {
		return false
	}
	payload, ok := a.decodeCookie(raw)
	if !ok {
		return false
	}
	return payload.Authority == authority(r)
}

// bearerMatches 校验 Authorization: Bearer <launch token>.
//
// 供 CLI 与第二个实例使用, 它们从 runtime.json 读到令牌.
func (a *auth) bearerMatches(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	return tokenMatches(strings.TrimSpace(strings.TrimPrefix(header, prefix)), a.launchToken)
}

// admit 是 /api/* 的统一准入检查.
func (a *auth) admit(w http.ResponseWriter, r *http.Request) bool {
	if !trustedRequest(r) {
		writeProblem(w, http.StatusForbidden, "请求来源不可信")
		return false
	}
	if a.authenticated(r) || a.bearerMatches(r) {
		return true
	}
	writeProblem(w, http.StatusUnauthorized, "未通过本地鉴权, 请从托盘重新打开界面")
	return false
}

// serveIndex 处理界面入口请求, 必要时用令牌换发 cookie.
//
// 令牌只出现在 URL 一次: 校验通过后立刻种下 HttpOnly 签名 cookie,
// 并 303 重定向到去掉 token 参数的同一路径, 这样地址栏、历史记录与
// Referer 里都不会再留下令牌.
func (a *auth) serveIndex(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if !trustedRequest(r) {
		writeProblem(w, http.StatusForbidden, "请求来源不可信")
		return
	}

	tokens := r.URL.Query()[tokenQuery]
	if len(tokens) > 0 {
		// 令牌有效: 换发 cookie 后重定向到干净地址.
		if len(tokens) == 1 && tokenMatches(tokens[0], a.launchToken) {
			if !a.mintCookie(w, r) {
				writeProblem(w, http.StatusInternalServerError, "生成会话失败")
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, cleanPath(r), http.StatusSeeOther)
			return
		}
		// 令牌无效但已有有效 cookie: 同样重定向到干净地址, 不因为一个过期链接把人挡在外面.
		if a.authenticated(r) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			http.Redirect(w, r, cleanPath(r), http.StatusSeeOther)
			return
		}
		writeProblem(w, http.StatusUnauthorized, "访问令牌无效, 请从托盘重新打开界面")
		return
	}

	if !a.authenticated(r) {
		writeProblem(w, http.StatusUnauthorized, "未通过本地鉴权, 请从托盘重新打开界面")
		return
	}
	next.ServeHTTP(w, r)
}

// mintCookie 签发并种下会话 cookie, 失败时返回 false.
func (a *auth) mintCookie(w http.ResponseWriter, r *http.Request) bool {
	now := time.Now()
	payload := cookiePayload{
		Version:   cookieVersion,
		Authority: authority(r),
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(cookieMaxAge).Unix(),
	}
	value, err := a.encodeCookie(payload)
	if err != nil {
		a.logger.Error("签发会话 cookie 失败", "error", err)
		return false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(authority(r)),
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(cookieMaxAge / time.Second),
	})
	return true
}

// cleanPath 返回去掉令牌参数后的同路径地址.
func cleanPath(r *http.Request) string {
	query := r.URL.Query()
	query.Del(tokenQuery)
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	if encoded := query.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

// RuntimeState 是写入 runtime.json 的内容, 供 CLI 与第二实例发现服务.
type RuntimeState struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
}

// writeRuntime 原子写入 runtime.json.
func writeRuntime(path string, state RuntimeState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建运行目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".runtime-*.json")
	if err != nil {
		return fmt.Errorf("创建临时运行文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置运行文件权限失败: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		tmp.Close()
		return fmt.Errorf("写入运行文件失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭运行文件失败: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("替换运行文件失败: %w", err)
	}
	return nil
}

// ReadRuntime 读取 runtime.json.
func ReadRuntime(path string) (*RuntimeState, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state RuntimeState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, fmt.Errorf("解析运行文件失败: %w", err)
	}
	if state.Port <= 0 || state.Token == "" {
		return nil, fmt.Errorf("运行文件内容不完整")
	}
	return &state, nil
}

// removeRuntime 删除 runtime.json.
func removeRuntime(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// portOf 从监听地址解析端口.
func portOf(address string) (int, error) {
	_, portRaw, err := net.SplitHostPort(address)
	if err != nil {
		return 0, fmt.Errorf("监听地址无效: %w", err)
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return 0, fmt.Errorf("监听端口无效: %w", err)
	}
	return port, nil
}
