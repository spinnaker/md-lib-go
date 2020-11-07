package mdlib

import (
	"encoding/json"
	"fmt"
)

// ErrorUnexpectedResponse will capture request details upon error.
type ErrorUnexpectedResponse struct {
	StatusCode int
	URL        string
	Content    []byte
}

// Error returns the error message
func (e ErrorUnexpectedResponse) Error() string {
	return fmt.Sprintf("Unexpected response from %s, expected 200 or 201 but got %d", e.URL, e.StatusCode)
}

// Parse will attempt to populate the data from the content of the failed request.
func (e ErrorUnexpectedResponse) Parse(data interface{}) error {
	return json.Unmarshal(e.Content, data)
}

// ErrorInvalidContent will occur when content is unable to be parsed.
type ErrorInvalidContent struct {
	Content    []byte
	ParseError error
}

// Error returns the parse error message.
func (e ErrorInvalidContent) Error() string {
	return e.ParseError.Error()
}
