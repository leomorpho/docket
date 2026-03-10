package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Request struct {
	ID     interface{}            `json:"id,omitempty"`
	Action string                 `json:"action"`
	Args   map[string]interface{} `json:"args,omitempty"`
}

type Response struct {
	ID     interface{} `json:"id,omitempty"`
	OK     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

func ServeMCP(in io.Reader, out io.Writer, repoRoot string) error {
	s := bufio.NewScanner(in)
	w := bufio.NewWriter(out)
	defer w.Flush()

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := Response{OK: false, Error: fmt.Sprintf("invalid json: %v", err)}
			if err := writeResponse(w, resp); err != nil {
				return err
			}
			continue
		}

		result, err := Dispatch(req.Action, req.Args, repoRoot)
		resp := Response{ID: req.ID}
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
		} else {
			resp.OK = true
			resp.Result = result
		}
		if err := writeResponse(w, resp); err != nil {
			return err
		}
	}

	if err := s.Err(); err != nil {
		return err
	}
	return nil
}

func writeResponse(w *bufio.Writer, resp Response) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return err
	}
	return w.Flush()
}
