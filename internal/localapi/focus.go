package localapi

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Focus 请求已运行的实例打开管理界面.
//
// 第二个实例启动时用它把自己的意图转达给主实例:
// 从 runtime.json 读到端口与令牌, 带上 Bearer 头调用 /api/app/focus.
//
// 注意不要用 runtime.json 里的 url 拼接口地址: 那个地址带 ?token= 查询参数,
// 直接拼接会得到 /?token=.../api/app/focus 这种错路径.
func Focus(port int, token string) error {
	if port <= 0 || token == "" {
		return fmt.Errorf("运行状态不完整")
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/app/focus", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("构造聚焦请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送聚焦请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("聚焦请求返回 %d", resp.StatusCode)
	}
	return nil
}
