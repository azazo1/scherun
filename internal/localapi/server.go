package localapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/azazo1/scherun/internal/config"
	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/store"
	"github.com/azazo1/scherun/internal/webui"
)

// Options 是服务启动参数.
type Options struct {
	// Bind 是监听地址, 例如 127.0.0.1:20008.
	Bind string
	// WebFS 是前端静态资源根.
	WebFS fs.FS
	// Version 是展示用版本号.
	Version string
	// Commit 是构建提交.
	Commit string
	// RuntimePath 是 runtime.json 路径, 为空时不写.
	RuntimePath string
	// SecretPath 是 cookie 签名密钥路径.
	SecretPath string
	// Config 是启动时的配置, 界面保存配置时回写.
	Config *config.Config
	// DB 用于任务持久化与目录浏览之外的数据访问.
	DB *store.DB
	// Scheduler 负责任务调度.
	Scheduler *scheduler.Scheduler
	// Logger 是日志器.
	Logger *slog.Logger
	// OnExit 在界面请求退出时调用.
	OnExit func()
	// OnOpen 在第二实例请求聚焦时调用.
	OnOpen func()
}

// Server 是本地 HTTP 服务.
type Server struct {
	opts     Options
	logger   *slog.Logger
	auth     *auth
	webFS    fs.FS
	httpSrv  *http.Server
	listener net.Listener
	started  time.Time
	port     int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// New 创建服务, 不开始监听.
func New(opts Options) (*Server, error) {
	if opts.Logger == nil {
		return nil, fmt.Errorf("缺少日志器")
	}
	if opts.Scheduler == nil {
		return nil, fmt.Errorf("缺少调度器")
	}
	if opts.DB == nil {
		return nil, fmt.Errorf("缺少数据库")
	}
	auth, err := newAuth(opts.SecretPath, opts.Logger)
	if err != nil {
		return nil, err
	}
	return &Server{
		opts:    opts,
		logger:  opts.Logger.With("component", "localapi"),
		auth:    auth,
		webFS:   opts.WebFS,
		started: time.Now(),
		done:    make(chan struct{}),
	}, nil
}

// Start 开始监听.
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.opts.Bind)
	if err != nil {
		return fmt.Errorf("监听 %s 失败 (端口可能已被占用): %w", s.opts.Bind, err)
	}
	s.listener = listener

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		listener.Close()
		return fmt.Errorf("无法获取监听端口")
	}
	s.port = tcpAddr.Port

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.httpSrv = &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if err := s.httpSrv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP 服务异常退出", "error", err)
		}
	}()

	if s.opts.RuntimePath != "" {
		state := RuntimeState{
			PID:       pid(),
			Port:      s.port,
			URL:       s.URL(),
			Token:     s.auth.LaunchToken(),
			StartedAt: s.started,
		}
		if err := writeRuntime(s.opts.RuntimePath, state); err != nil {
			s.logger.Warn("写入运行状态失败", "error", err)
		}
	}

	s.logger.Info("本地服务已启动", "url", s.URL(), "port", s.port)
	return nil
}

// URL 返回带令牌的访问地址.
//
// 令牌放在查询参数里, 浏览器首次打开时用它换发 cookie, 随后地址被重定向成干净形式.
func (s *Server) URL() string {
	host := s.opts.Bind
	if h, _, err := net.SplitHostPort(host); err == nil {
		if h == "" || h == "0.0.0.0" || h == "::" {
			host = net.JoinHostPort("127.0.0.1", fmt.Sprint(s.port))
		} else {
			host = net.JoinHostPort(h, fmt.Sprint(s.port))
		}
	}
	return fmt.Sprintf("http://%s/?%s=%s", host, tokenQuery, s.auth.LaunchToken())
}

// BaseURL 返回不带令牌的地址, 用于展示与日志.
func (s *Server) BaseURL() string {
	url := s.URL()
	if idx := strings.Index(url, "/?"); idx >= 0 {
		return url[:idx] + "/"
	}
	return url
}

// Port 返回实际监听端口.
func (s *Server) Port() int { return s.port }

// Token 返回启动令牌.
func (s *Server) Token() string { return s.auth.LaunchToken() }

// Shutdown 关闭服务并删除 runtime.json.
func (s *Server) Shutdown(ctx context.Context) {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.httpSrv != nil {
			if err := s.httpSrv.Shutdown(ctx); err != nil {
				s.logger.Warn("HTTP 服务未能优雅关闭", "error", err)
				_ = s.httpSrv.Close()
			}
		}
		if s.opts.RuntimePath != "" {
			if err := removeRuntime(s.opts.RuntimePath); err != nil {
				s.logger.Warn("删除运行状态失败", "error", err)
			}
		}
		close(s.done)
	})
}

// Done 在服务关闭后关闭.
func (s *Server) Done() <-chan struct{} { return s.done }

// handler 组装路由.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()

	// 界面入口需要单独处理令牌换发.
	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serveStatic(w, r)
	})
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只对根路径与前端路由做鉴权; 静态资源同样要求已登录.
		s.auth.serveIndex(w, r, index)
	}))

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/tasks", s.requireAuth(s.handleTasks))
	mux.HandleFunc("/api/tasks/", s.requireAuth(s.handleTaskByID))
	mux.HandleFunc("/api/tasks/reorder", s.requireAuth(s.handleReorder))
	mux.HandleFunc("/api/warnings", s.requireAuth(s.handleWarnings))
	mux.HandleFunc("/api/config", s.requireAuth(s.handleConfig))
	mux.HandleFunc("/api/fs/list", s.requireAuth(s.handleFSList))
	mux.HandleFunc("/api/events", s.requireAuth(s.handleEvents))
	mux.HandleFunc("/api/app/exit", s.requireAuth(s.handleExit))
	mux.HandleFunc("/api/app/focus", s.requireAuth(s.handleFocus))

	return withSecurityHeaders(mux)
}

// requireAuth 包装需要鉴权的处理器.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.admit(w, r) {
			return
		}
		next(w, r)
	}
}

// withSecurityHeaders 添加基础安全响应头.
//
// CSP 只允许同源资源, 因此即使页面上出现注入内容也无法外联;
// frame-ancestors 与 X-Frame-Options 一起挡住点击劫持.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self' data:; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'none'; "+
				"form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

// serveStatic 托管前端资源, 未命中的路径回退到 index.html 支持前端路由.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if s.webFS == nil || !webui.Built() {
		// 没有构建产物时给出可读提示, 而不是让界面空白.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(webui.Index))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(s.webFS, path); err != nil {
		// 前端路由 (如 /settings) 交给 index.html 处理.
		path = "index.html"
	}
	if filepath.Ext(path) != ".html" {
		// 静态资源带哈希, 可以长期缓存.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	http.ServeFileFS(w, r, s.webFS, path)
}

// pid 返回当前进程 id.
func pid() int {
	return os.Getpid()
}
