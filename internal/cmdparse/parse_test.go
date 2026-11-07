package cmdparse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// programName 返回程序路径的文件名, 去掉 Windows 的 .exe 后缀.
//
// 解析会把能找到的命令补成绝对路径, 所以断言只能看文件名;
// Windows 上补出来的是 xxx.exe, 直接用 suffix 比对会失败.
func programName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".exe")
}

// TestParseDockerRun 覆盖最典型的场景: 一条带环境变量与参数的长命令.
func TestParseDockerRun(t *testing.T) {
	result, err := Parse(`docker run --rm -it -v /tmp/data:/data -e MODE=prod nginx:latest sh -c "echo hello world"`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 程序名应被解析成绝对路径 (docker 在 PATH 里时).
	if programName(result.Program) != "docker" {
		t.Fatalf("程序应为 docker, 实际 %q", result.Program)
	}

	wantArgs := []string{
		"run", "--rm", "-it", "-v", "/tmp/data:/data", "-e", "MODE=prod",
		"nginx:latest", "sh", "-c", "echo hello world",
	}
	if len(result.Args) != len(wantArgs) {
		t.Fatalf("参数数量应为 %d, 实际 %d: %v", len(wantArgs), len(result.Args), result.Args)
	}
	for index, want := range wantArgs {
		if result.Args[index] != want {
			t.Fatalf("第 %d 个参数应为 %q, 实际 %q", index, want, result.Args[index])
		}
	}
	// -e MODE=prod 是 docker 的参数, 不应被当成任务的环境变量.
	if _, ok := result.EnvVars["MODE"]; ok {
		t.Fatal("docker 的 -e 不应被解析成任务环境变量")
	}
}

// TestParseEnvPrefix 验证前导环境变量赋值被提取.
func TestParseEnvPrefix(t *testing.T) {
	result, err := Parse(`NODE_ENV=production PORT=8080 node server.js`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.EnvVars["NODE_ENV"] != "production" || result.EnvVars["PORT"] != "8080" {
		t.Fatalf("环境变量解析错误: %v", result.EnvVars)
	}
	if programName(result.Program) != "node" {
		t.Fatalf("程序应为 node, 实际 %q", result.Program)
	}
	if len(result.Args) != 1 || result.Args[0] != "server.js" {
		t.Fatalf("参数解析错误: %v", result.Args)
	}
}

// TestParseQuotedValueIsNotEnv 验证被引号包裹的 KEY=VALUE 不被当成环境变量.
func TestParseQuotedValueIsNotEnv(t *testing.T) {
	result, err := Parse(`echo "A=b"`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(result.EnvVars) != 0 {
		t.Fatalf("引号包裹的内容不应成为环境变量: %v", result.EnvVars)
	}
	if len(result.Args) != 1 || result.Args[0] != "A=b" {
		t.Fatalf("参数解析错误: %v", result.Args)
	}
}

// TestParseRedirect 验证三种标准流重定向.
func TestParseRedirect(t *testing.T) {
	result, err := Parse(`python3 job.py < input.txt > out.log 2> err.log`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Stdin != "input.txt" {
		t.Fatalf("标准输入错误: %q", result.Stdin)
	}
	if result.Stdout != "out.log" {
		t.Fatalf("标准输出错误: %q", result.Stdout)
	}
	if result.Stderr != "err.log" {
		t.Fatalf("标准错误错误: %q", result.Stderr)
	}
	// 重定向不应混进参数.
	for _, arg := range result.Args {
		if strings.ContainsAny(arg, "<>") {
			t.Fatalf("重定向符不应出现在参数里: %v", result.Args)
		}
	}
}

// TestParseAppendWarns 验证追加写入给出告警.
func TestParseAppendWarns(t *testing.T) {
	result, err := Parse(`echo hi >> out.log`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Stdout != "out.log" {
		t.Fatalf("标准输出错误: %q", result.Stdout)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("追加写入应当给出告警")
	}
}

// TestParseCdPrefix 验证 cd 被解析成工作目录.
func TestParseCdPrefix(t *testing.T) {
	result, err := Parse(`cd /tmp/work && python3 run.py`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.WorkingDir != "/tmp/work" {
		t.Fatalf("工作目录应为 /tmp/work, 实际 %q", result.WorkingDir)
	}
	if programName(result.Program) != "python3" {
		t.Fatalf("程序应为 python3, 实际 %q", result.Program)
	}
	if len(result.Args) != 1 || result.Args[0] != "run.py" {
		t.Fatalf("参数解析错误: %v", result.Args)
	}
}

// TestParsePipeWarns 验证管道只取第一段并给出告警.
func TestParsePipeWarns(t *testing.T) {
	result, err := Parse(`cat data.txt | grep error`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if programName(result.Program) != "cat" {
		t.Fatalf("应取第一段命令, 实际 %q", result.Program)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("管道应当给出告警")
	}
}

// TestParseTildeExpansion 验证 ~ 被展开.
func TestParseTildeExpansion(t *testing.T) {
	result, err := Parse(`cd ~/projects && ./build.sh`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(result.WorkingDir, home) {
		t.Fatalf("~ 应展开为家目录, 实际 %q", result.WorkingDir)
	}
}

// TestParseEmptyAndInvalid 验证空命令与未闭合引号被拒绝.
func TestParseEmptyAndInvalid(t *testing.T) {
	if _, err := Parse("   "); err == nil {
		t.Fatal("空命令应当报错")
	}
	if _, err := Parse(`echo "unterminated`); err == nil {
		t.Fatal("未闭合的双引号应当报错")
	}
	if _, err := Parse(`echo 'unterminated`); err == nil {
		t.Fatal("未闭合的单引号应当报错")
	}
}

// TestParseMissingProgramWarns 验证 PATH 中找不到程序时给出告警.
func TestParseMissingProgramWarns(t *testing.T) {
	result, err := Parse(`definitely-not-a-real-command-xyz --flag`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Resolved {
		t.Fatal("不存在的程序不应标记为已解析")
	}
	if len(result.Warnings) == 0 {
		t.Fatal("找不到程序时应当给出告警")
	}
}

// TestParseAbsolutePath 验证绝对路径直接采用.
func TestParseAbsolutePath(t *testing.T) {
	// 用平台自己的绝对路径: /bin/echo 在 Windows 上没有盘符, 不算绝对路径.
	// 反斜杠在分词里是转义符, 因此统一换成 / 再拼进命令.
	program := strings.ReplaceAll(filepath.Join(t.TempDir(), "echo"), "\\", "/")

	result, err := Parse(program + " hello")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.Program != program {
		t.Fatalf("程序应为 %q, 实际 %q", program, result.Program)
	}
	if !result.Resolved {
		t.Fatal("绝对路径应标记为已解析")
	}
}

// TestTokenizeEscapes 验证转义与引号组合.
func TestTokenizeEscapes(t *testing.T) {
	tokens, err := tokenize(`echo "a \"b\" c" 'd e' f\ g`)
	if err != nil {
		t.Fatalf("分词失败: %v", err)
	}
	want := []string{"echo", `a "b" c`, "d e", "f g"}
	if len(tokens) != len(want) {
		t.Fatalf("词数应为 %d, 实际 %d: %+v", len(want), len(tokens), tokens)
	}
	for index, expected := range want {
		if tokens[index].text != expected {
			t.Fatalf("第 %d 个词应为 %q, 实际 %q", index, expected, tokens[index].text)
		}
	}
}
