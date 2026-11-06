package localapi

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/azazo1/scherun/internal/pathutil"
)

// fsEntry 是目录浏览返回的条目.
type fsEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	Executable bool   `json:"executable"`
	// Selectable 表示该条目可以被选为程序路径 (可执行文件或 macOS 的 .app).
	Selectable bool  `json:"selectable"`
	Size       int64 `json:"size,omitempty"`
}

// fsResponse 是目录浏览响应.
type fsResponse struct {
	// Path 是当前目录.
	Path string `json:"path"`
	// Parent 是上级目录, 到达根时为空.
	Parent string `json:"parent,omitempty"`
	// Entries 是当前目录下的条目.
	Entries []fsEntry `json:"entries"`
	// Home 是用户家目录, 供界面快速跳转.
	Home string `json:"home"`
	// Roots 是可以快速跳转的根目录.
	Roots []string `json:"roots"`
	// ShowHidden 表示当前是否列出了隐藏条目.
	ShowHidden bool `json:"show_hidden"`
	// Selected 是请求路径本身指向的文件 (若请求的是文件).
	// 界面据此把它标为已选中, 用户再点 "选择" 即可.
	Selected string `json:"selected,omitempty"`
}

// handleFSList 浏览本机目录.
//
// 浏览器无法取得本地绝对路径, 因此用服务端目录浏览代替原生文件选择器.
// 服务只监听回环地址且请求已通过鉴权, 因此允许浏览整个文件系统.
func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 GET")
		return
	}

	home, _ := os.UserHomeDir()
	query := r.URL.Query()
	requested := query.Get("path")
	if requested == "" {
		requested = home
	}
	// show_hidden 控制是否列出以点开头的条目.
	// 默认不显示, 避免列表被配置与缓存目录淹没; 但 ~/.local/bin 这类
	// 目录本身就在隐藏路径下, 因此必须允许显式打开.
	showHidden := query.Get("show_hidden") == "1"
	// 用户会在输入框里写 ~, 先展开再判断是否为绝对路径.
	requested = pathutil.ExpandHome(requested)
	requested = filepath.Clean(requested)
	if !filepath.IsAbs(requested) {
		writeProblem(w, http.StatusBadRequest, "路径必须是绝对路径, 也可以用 ~ 开头")
		return
	}

	info, err := os.Stat(requested)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "目录不存在: "+requested)
		return
	}
	// 指向文件时改为列出它所在目录, 并把该文件标为已选中.
	selectFile := ""
	if !info.IsDir() {
		selectFile = requested
		requested = filepath.Dir(requested)
	}

	dirents, err := os.ReadDir(requested)
	if err != nil {
		writeProblem(w, http.StatusForbidden, "无法读取目录: "+err.Error())
		return
	}

	entries := make([]fsEntry, 0, len(dirents))
	for _, dirent := range dirents {
		name := dirent.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(requested, name)
		entry := fsEntry{Name: name, Path: full, IsDir: dirent.IsDir()}
		if info, err := dirent.Info(); err == nil {
			entry.Size = info.Size()
			if !dirent.IsDir() {
				entry.Executable = isExecutable(info)
				entry.Selectable = entry.Executable
			}
		}
		if dirent.IsDir() && runtime.GOOS == "darwin" && strings.HasSuffix(strings.ToLower(name), ".app") {
			entry.Selectable = true
		}
		entries = append(entries, entry)
	}

	// 目录在前, 同类按名称排序.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(requested)
	if parent == requested {
		parent = ""
	}

	writeJSON(w, http.StatusOK, fsResponse{
		Path:       requested,
		Parent:     parent,
		Entries:    entries,
		Home:       home,
		Roots:      volumeRoots(home),
		ShowHidden: showHidden,
		Selected:   selectFile,
	})
}

// isExecutable 判断文件是否可执行.
func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(info.Name()))
		return ext == ".exe" || ext == ".bat" || ext == ".cmd" || ext == ".com"
	}
	return info.Mode()&0o111 != 0
}

// volumeRoots 返回可快速跳转的根目录.
func volumeRoots(home string) []string {
	var roots []string
	if runtime.GOOS == "windows" {
		for drive := 'C'; drive <= 'Z'; drive++ {
			path := string(drive) + ":\\"
			if _, err := os.Stat(path); err == nil {
				roots = append(roots, path)
			}
		}
		if home != "" {
			roots = append(roots, home)
		}
		return roots
	}
	roots = []string{"/"}
	if home != "" {
		roots = append(roots, home)
	}
	// macOS 的卷挂在 /Volumes 下.
	if runtime.GOOS == "darwin" {
		if entries, err := os.ReadDir("/Volumes"); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					roots = append(roots, filepath.Join("/Volumes", entry.Name()))
				}
			}
		}
	}
	return roots
}
