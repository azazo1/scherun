// Package cmdparse 把一条 shell 命令行解析成任务字段.
//
// 用途是让用户在新建任务时直接粘贴命令, 由程序拆出程序路径, 参数,
// 环境变量, 工作目录与标准流重定向, 填进表单由用户确认.
//
// 它只做静态解析, 不执行任何命令, 也不做变量展开: 命令里出现的 $VAR
// 会原样保留, 由任务运行时的环境决定.
package cmdparse

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Result 是解析结果, 字段与任务定义一一对应.
type Result struct {
	Program    string            `json:"program"`
	Args       []string          `json:"args"`
	WorkingDir string            `json:"working_dir,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
	Stdin      string            `json:"stdin,omitempty"`
	Stdout     string            `json:"stdout,omitempty"`
	Stderr     string            `json:"stderr,omitempty"`
	// Warnings 是需要提醒用户的地方, 例如管道不受支持.
	Warnings []string `json:"warnings,omitempty"`
	// Resolved 表示程序路径是否已在 PATH 中解析为绝对路径.
	Resolved bool `json:"resolved"`
}

// tokenKind 区分普通词与操作符.
type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenOp
)

// token 是词法分析产出的一个单元.
type token struct {
	kind tokenKind
	text string
	// quoted 表示该词被引号包裹过, 用于避免把 "A=b" 当成环境变量赋值.
	quoted bool
}

// envAssignPattern 匹配形如 KEY=VALUE 的环境变量前缀.
var envAssignPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// Parse 解析命令行.
//
// 返回的 Result 可能只填了部分字段: 例如纯 `cd /tmp` 只设置工作目录.
func Parse(input string) (Result, error) {
	result := Result{Args: []string{}}

	tokens, err := tokenize(input)
	if err != nil {
		return result, err
	}
	if len(tokens) == 0 {
		return result, fmt.Errorf("命令为空")
	}

	segments, separators := splitSegments(tokens)

	// 第一段如果是 cd, 就把它当成工作目录, 主命令往后找.
	var commandTokens []token
	for index, segment := range segments {
		if len(segment) == 0 {
			continue
		}
		if isCdSegment(segment) {
			if dir, ok := cdTarget(segment); ok {
				result.WorkingDir = expandHome(dir)
				// cd 后面还有段落时, 后续段落才是主命令.
				if index < len(segments)-1 {
					continue
				}
				return result, nil
			}
		}
		if commandTokens == nil {
			commandTokens = segment
			continue
		}
		// 已经拿到主命令, 后面的段落无法表达成单个任务.
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("命令包含多个执行段 (分隔符 %q), 只取第一段; 需要管道或串联时请改用 shell -c 包装",
				separatorBefore(separators, index)))
		break
	}

	if len(commandTokens) == 0 {
		return result, fmt.Errorf("没有找到要执行的程序")
	}

	if err := parseSegment(commandTokens, &result); err != nil {
		return result, err
	}
	return result, nil
}

// isCdSegment 判断该段是否是 cd 命令.
func isCdSegment(segment []token) bool {
	if len(segment) == 0 || segment[0].kind != tokenWord {
		return false
	}
	return segment[0].text == "cd"
}

// cdTarget 取出 cd 的目标目录.
func cdTarget(segment []token) (string, bool) {
	for _, item := range segment[1:] {
		if item.kind == tokenWord {
			return item.text, true
		}
	}
	// 单独的 cd 表示回到用户家目录.
	return "~", true
}

// separatorBefore 返回第 index 段之前的分隔符, 用于告警文案.
func separatorBefore(separators []string, index int) string {
	if index-1 >= 0 && index-1 < len(separators) {
		return separators[index-1]
	}
	return "?"
}

// parseSegment 解析单个执行段.
func parseSegment(tokens []token, result *Result) error {
	index := 0

	// 先吃掉前导的环境变量赋值.
	for index < len(tokens) {
		item := tokens[index]
		if item.kind != tokenWord || item.quoted || !envAssignPattern.MatchString(item.text) {
			break
		}
		key, value, _ := strings.Cut(item.text, "=")
		if result.EnvVars == nil {
			result.EnvVars = map[string]string{}
		}
		result.EnvVars[key] = value
		index++
	}

	var words []string
	for index < len(tokens) {
		item := tokens[index]

		if item.kind == tokenWord {
			words = append(words, item.text)
			index++
			continue
		}

		// 处理重定向操作符.
		switch item.text {
		case ">", ">>", "2>", "2>>", "&>":
			if index+1 >= len(tokens) || tokens[index+1].kind != tokenWord {
				return fmt.Errorf("重定向符 %s 后面缺少文件路径", item.text)
			}
			target := expandHome(tokens[index+1].text)
			if item.text == "2>" || item.text == "2>>" {
				result.Stderr = target
			} else {
				// &> 同时重定向两个流, 但任务只记录一个目标.
				result.Stdout = target
				if item.text == "&>" {
					result.Stderr = target
				}
			}
			if strings.HasSuffix(item.text, ">>") {
				result.Warnings = append(result.Warnings,
					"追加写入 (>>) 暂不支持, 任务每次运行会覆盖该文件")
			}
			index += 2
		case "<":
			if index+1 >= len(tokens) || tokens[index+1].kind != tokenWord {
				return fmt.Errorf("重定向符 < 后面缺少文件路径")
			}
			result.Stdin = expandHome(tokens[index+1].text)
			index += 2
		case "2>&1":
			result.Warnings = append(result.Warnings,
				"标准错误合并到标准输出 (2>&1) 暂不支持, 请分别指定两个输出文件")
			index++
		default:
			return fmt.Errorf("无法识别的操作符 %q", item.text)
		}
	}

	if len(words) == 0 {
		return fmt.Errorf("没有找到要执行的程序")
	}

	result.Program = words[0]
	if len(words) > 1 {
		result.Args = words[1:]
	}

	// 把程序名解析成绝对路径: 任务直接 exec 该路径, 界面也要靠它判断可执行性.
	resolveProgram(result)
	return nil
}

// resolveProgram 尝试把程序名解析成绝对路径.
func resolveProgram(result *Result) {
	program := result.Program

	// 带路径分隔符的按路径处理, 只需要展开 ~.
	if strings.ContainsRune(program, os.PathSeparator) || strings.Contains(program, "/") {
		result.Program = expandHome(program)
		result.Resolved = filepath.IsAbs(result.Program)
		return
	}

	if path, err := exec.LookPath(program); err == nil {
		if abs, err := filepath.Abs(path); err == nil {
			result.Program = abs
		} else {
			result.Program = path
		}
		result.Resolved = true
		return
	}

	result.Warnings = append(result.Warnings,
		fmt.Sprintf("在 PATH 中找不到 %q, 请确认程序已安装或填写绝对路径", program))
}

// expandHome 把开头的 ~ 替换成用户家目录.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

// splitSegments 按 && || ; | & 切分执行段, 同时返回分隔符序列.
func splitSegments(tokens []token) ([][]token, []string) {
	var (
		segments   [][]token
		separators []string
		current    []token
	)
	for _, item := range tokens {
		if item.kind == tokenOp && isSegmentSeparator(item.text) {
			segments = append(segments, current)
			separators = append(separators, item.text)
			current = nil
			continue
		}
		current = append(current, item)
	}
	segments = append(segments, current)
	return segments, separators
}

// isSegmentSeparator 判断操作符是否切分执行段.
func isSegmentSeparator(op string) bool {
	switch op {
	case "&&", "||", ";", "|", "&":
		return true
	default:
		return false
	}
}

// tokenize 把命令行切成词与操作符.
//
// 支持单引号, 双引号与反斜杠转义; 引号内的空格与操作符按字面处理.
func tokenize(input string) ([]token, error) {
	var (
		tokens  []token
		current strings.Builder
		// hasToken 表示 current 里已经累积了内容 (可能为空字符串, 如 '').
		hasToken bool
		quoted   bool
	)

	flush := func() {
		if hasToken {
			tokens = append(tokens, token{kind: tokenWord, text: current.String(), quoted: quoted})
			current.Reset()
			hasToken = false
			quoted = false
		}
	}

	runes := []rune(input)
	for index := 0; index < len(runes); index++ {
		ch := runes[index]

		switch ch {
		case ' ', '\t', '\n', '\r':
			flush()
			continue
		case '#':
			// 行首或空白后的 # 视为注释, 其余位置按字面处理.
			if !hasToken {
				return tokens, nil
			}
		case '\'':
			hasToken = true
			quoted = true
			index++
			for ; index < len(runes); index++ {
				if runes[index] == '\'' {
					break
				}
				current.WriteRune(runes[index])
			}
			if index >= len(runes) {
				return nil, fmt.Errorf("单引号没有闭合")
			}
			continue
		case '"':
			hasToken = true
			quoted = true
			index++
			closed := false
			for ; index < len(runes); index++ {
				if runes[index] == '\\' && index+1 < len(runes) {
					next := runes[index+1]
					// 双引号内只对这些字符做转义, 其余反斜杠保留.
					if next == '"' || next == '\\' || next == '$' || next == '`' {
						current.WriteRune(next)
						index++
						continue
					}
				}
				if runes[index] == '"' {
					closed = true
					break
				}
				current.WriteRune(runes[index])
			}
			if !closed {
				return nil, fmt.Errorf("双引号没有闭合")
			}
			continue
		case '\\':
			hasToken = true
			if index+1 < len(runes) {
				current.WriteRune(runes[index+1])
				index++
			}
			continue
		}

		// 操作符: 先尝试更长的形式, 避免把 && 拆成两个 &.
		if op, width := matchOperator(runes[index:]); width > 0 {
			flush()
			tokens = append(tokens, token{kind: tokenOp, text: op})
			index += width - 1
			continue
		}

		hasToken = true
		current.WriteRune(ch)
	}
	flush()

	if len(tokens) == 0 {
		return nil, nil
	}
	return tokens, nil
}

// matchOperator 在当前位置匹配操作符, 返回操作符与其占用的字符数.
func matchOperator(runes []rune) (string, int) {
	candidates := []string{"2>&1", "&&", "||", "2>>", ">>", "2>", "&>", "|", ";", "&", ">", "<"}
	text := string(runes)
	for _, op := range candidates {
		if strings.HasPrefix(text, op) {
			return op, len([]rune(op))
		}
	}
	return "", 0
}
