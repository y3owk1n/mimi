package ipc

import (
	"bufio"
	"encoding/json"
	"io"

	"github.com/y3owk1n/mimi/internal/action"
	derrors "github.com/y3owk1n/mimi/internal/errors"
)

// ProtocolVersion is the version of the request envelope this build speaks.
//
// It is carried on every request and compared for strict equality by the
// daemon. Bump it whenever the encoded shape of a request changes in a way an
// older daemon would read wrongly — a renamed or removed field, or a field
// whose meaning changed. TestRequest_EncodesTheGoldenBytes is what makes such
// a change visible.
const ProtocolVersion = 1

// Request is the envelope one command travels in over the daemon's Unix
// socket.
//
// The command is carried typed rather than flattened into command-line
// strings, so the daemon runs the same value the CLI built instead of
// re-parsing one (see docs/adr/0001-typed-versioned-daemon-wire.md). The
// action's name lives on the command and nowhere else, so no two fields can
// disagree about which action this is.
type Request struct {
	Version int            `json:"version"`
	Command action.Command `json:"command"`
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

	// A request on another version of the envelope decodes without complaint —
	// unknown fields are ignored and absent ones take their zero value — so
	// this comparison, not the decoder, is what catches a CLI and a daemon
	// built either side of a wire change. It lives here rather than at the
	// caller so no request can be acted on without having passed it.
	//
	// Strict equality in both directions: skew breaks a newer daemon reading
	// an older client's request just as badly as the reverse, and a version
	// absent altogether decodes to zero and fails the same comparison, which
	// is the right answer for any CLI built before the typed wire.
	if req.Version != ProtocolVersion {
		return Request{}, derrors.Newf(
			derrors.CodeProtocolMismatch,
			"daemon speaks request protocol version %d, client sent %d",
			ProtocolVersion,
			req.Version,
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

	// The code travels in its own field and errorFromResponse rebuilds the
	// error from the pair, so the message must not carry the code as well:
	// the daemon path would report "[INVALID_INPUT] [INVALID_INPUT] …" where
	// the direct path reports it once, for the same command.
	return Response{
		OK:      false,
		Code:    string(derrors.GetCode(err)),
		Message: derrors.Message(err),
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
