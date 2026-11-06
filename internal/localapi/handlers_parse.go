package localapi

import (
	"net/http"

	"github.com/azazo1/scherun/internal/cmdparse"
)

// handleParseCommand 把一条命令行解析成任务字段.
//
// 只在新建任务时用于预填表单, 不创建任何任务, 也不执行命令.
func (s *Server) handleParseCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}

	var payload struct {
		Command string `json:"command"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "请求体格式不正确: "+err.Error())
		return
	}

	result, err := cmdparse.Parse(payload.Command)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
