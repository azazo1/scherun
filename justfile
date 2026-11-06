# scherun 的日常操作入口.
#
# 依赖: Go 1.27+, bun, just, uv (仅脚本运行时需要), 以及 C 工具链 (托盘需要 cgo).

[private]
default:
    @just --list

# 安装依赖 (Go 模块与前端依赖).
deps:
    go mod download
    cd web && bun install

# 构建前端产物并同步到 go:embed 目录.
web:
    cd web && bun run build
    uv run --no-project python scripts/embed_web.py

# 构建二进制到 dist/scherun.
build: web
    uv run --no-project python scripts/build.py

# 启动应用 (自动打开浏览器).
run: build
    ./dist/scherun

# 启动应用但不开浏览器.
run-quiet: build
    ./dist/scherun --no-browser

# 开发模式: Vite 热更新 + Go 后端, 使用隔离数据目录.
dev:
    uv run --no-project python scripts/dev.py

# 调试实例, 使用隔离数据目录 .tmp/scherun-debug 与 debug 级别日志.
# 单实例锁按数据目录绑定, 因此可与正式实例并行运行, 互不影响.
# 启动隔离数据目录的调试实例.
debug:
    uv run --no-project python scripts/debug.py

# 格式化 Go 代码.
fmt:
    gofmt -w cmd internal

# 静态检查.
vet:
    go vet ./...

# 运行测试 (Go 与前端).
test:
    go test ./...
    cd web && node --test --experimental-strip-types "src/**/*.test.ts"

# 带竞态检测运行测试.
test-race:
    go test -race ./...

# 清理构建产物与临时目录.
clean:
    rm -rf dist .tmp web/dist
