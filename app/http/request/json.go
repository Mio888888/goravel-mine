package request

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// BindJSONBody supports the application's legacy top-level array payloads in
// addition to JSON objects accepted by the framework request binder.
func BindJSONBody(request *http.Request, dest any) error {
	if request == nil || request.Body == nil {
		return nil
	}

	body, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}

	return json.Unmarshal(body, dest)
}
