package mdlib

import (
	"encoding/json"
	"fmt"
)

type ErrorUnexpectedResponse struct {
	StatusCode int
	URL        string
	Content    []byte
}

func (e ErrorUnexpectedResponse) Error() string {
	return fmt.Sprintf("Unexpected response from %s, expected 200 or 201 but got %d", e.URL, e.StatusCode)
}

func (e ErrorUnexpectedResponse) Parse(data interface{}) error {
	return json.Unmarshal(e.Content, data)
}

type ErrorInvalidContent struct {
	Content    []byte
	ParseError error
}

func (e ErrorInvalidContent) Error() string {
	return e.ParseError.Error()
}
