package ipc

import (
	"bufio"
	"encoding/json"
	"io"

	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// Request is a JSON-encoded action request sent over the Unix socket.
type Request struct {
	Action string   `json:"action"`
	Args   []string `json:"args"`
}

// Response is a JSON-encoded action result sent back to the client.
type Response struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func writeRequest(writer io.Writer, req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeSerializationFailed, "encoding IPC request")
	}

	_, err = writer.Write(append(data, '\n'))
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeIPCFailed, "writing IPC request")
	}

	return nil
}

func readRequest(r *bufio.Reader) (Request, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Request{}, derrors.Wrapf(err, derrors.CodeIPCFailed, "reading IPC request")
	}

	var req Request

	err = json.Unmarshal(line, &req)
	if err != nil {
		return Request{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"decoding IPC request",
		)
	}

	return req, nil
}

func writeResponse(writer io.Writer, resp Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeSerializationFailed, "encoding IPC response")
	}

	_, err = writer.Write(append(data, '\n'))
	if err != nil {
		return derrors.Wrapf(err, derrors.CodeIPCFailed, "writing IPC response")
	}

	return nil
}

func readResponse(r *bufio.Reader) (Response, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Response{}, derrors.Wrapf(err, derrors.CodeIPCFailed, "reading IPC response")
	}

	var resp Response

	err = json.Unmarshal(line, &resp)
	if err != nil {
		return Response{}, derrors.Wrapf(
			err,
			derrors.CodeSerializationFailed,
			"decoding IPC response",
		)
	}

	return resp, nil
}

func responseFromError(err error) Response {
	if err == nil {
		return Response{OK: true}
	}

	return Response{
		OK:      false,
		Code:    string(derrors.GetCode(err)),
		Message: err.Error(),
	}
}

func errorFromResponse(resp Response) error {
	if resp.OK {
		return nil
	}

	code := derrors.Code(resp.Code)
	if code == "" {
		code = derrors.CodeIPCFailed
	}

	return derrors.New(code, resp.Message)
}
