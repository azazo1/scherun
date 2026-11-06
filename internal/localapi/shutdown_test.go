package localapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/azazo1/scherun/internal/scheduler"
	"github.com/azazo1/scherun/internal/store"
)

// newTestServer 启动一个监听随机端口的服务.
func newTestServer(t *testing.T) *Server {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sched := scheduler.New(db, logger)
	t.Cleanup(sched.Close)

	server, err := New(Options{
		Bind:       "127.0.0.1:0",
		Version:    "test",
		SecretPath: filepath.Join(t.TempDir(), "secret"),
		DB:         db,
		Scheduler:  sched,
		Logger:     logger,
	})
	if err != nil {
		t.Fatalf("创建服务失败: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("启动服务失败: %v", err)
	}
	return server
}

// TestShutdownWithActiveSSE 验证有 SSE 长连接时关闭能及时返回.
//
// 这是一个死锁回归测试: http.Server.Shutdown 会等所有活动连接结束,
// 而 SSE 原先在等 done 关闭, done 又要等 Shutdown 返回才关闭,
// 两者互相等待, 只能靠 ctx 超时打破, 表现为退出卡满 5 秒.
func TestShutdownWithActiveSSE(t *testing.T) {
	server := newTestServer(t)

	// 建立一条 SSE 连接并保持.
	request, err := http.NewRequest(
		http.MethodGet,
		server.BaseURL()+"api/events",
		nil,
	)
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+server.Token())

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("建立 SSE 连接失败: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("SSE 应返回 200, 实际 %d", response.StatusCode)
	}
	// 读一点数据, 确保连接确实已建立并进入推送循环.
	buf := make([]byte, 64)
	_ = response.Body.(io.Reader)
	go func() { _, _ = io.ReadFull(response.Body, buf) }()
	time.Sleep(200 * time.Millisecond)

	// 关闭必须很快完成. 这里刻意用远小于 Shutdown 超时的阈值:
	// 一旦死锁回归, Shutdown 会卡到 ctx 超时才返回, 断言立刻失败,
	// 而不是像宽阈值那样把慢速失败也当成通过.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		server.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("关闭耗时 %v, 说明仍在等 SSE 超时 (死锁回归)", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("关闭被 SSE 连接阻塞, 说明死锁未修复")
	}

	// 关闭后 done 通道应当已关闭.
	select {
	case <-server.Done():
	case <-time.After(time.Second):
		t.Fatal("关闭后 Done 通道未关闭")
	}
}
