package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// writeJSON prints v as JSON.
func (a *app) writeJSON(v any) error {
	enc := json.NewEncoder(a.out)
	enc.SetEscapeHTML(false)
	if !a.compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

// writeBody prints a response body: JSON is re-indented (or compacted), and
// anything else is printed as it is.
func (a *app) writeBody(body []byte) error {
	var buf bytes.Buffer
	var err error
	if a.compact {
		err = json.Compact(&buf, body)
	} else {
		err = json.Indent(&buf, body, "", "  ")
	}
	if err != nil {
		buf.Reset()
		buf.Write(body)
	}
	if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	_, err = a.out.Write(buf.Bytes())
	return err
}

// decode reads JSON into v, keeping numbers exact.
func decode(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(v)
}

// readBody reads a JSON request body given as literal JSON, "@file" or "-" for stdin.
func (a *app) readBody(arg string) ([]byte, error) {
	var data []byte
	var err error
	switch {
	case arg == "-":
		data, err = io.ReadAll(a.in)
	case strings.HasPrefix(arg, "@"):
		data, err = os.ReadFile(arg[1:])
	default:
		data = []byte(arg)
	}
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if !json.Valid(data) {
		return nil, errors.New("the request body is not valid JSON")
	}
	return data, nil
}

// checkIndex rejects index arguments that would change the request path.
func checkIndex(index string) error {
	if index == "" || strings.ContainsAny(index, "/?# \t\r\n") {
		return fmt.Errorf("%q is not a valid index name or pattern", index)
	}
	return nil
}
