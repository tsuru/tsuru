package json

import (
	"bytes"
	"encoding/json"
)

func Marshal(v interface{}) ([]byte, error) {
	var (
		buf = &bytes.Buffer{}
		enc = json.NewEncoder(buf)
	)

	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
