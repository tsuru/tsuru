package webserver

type HttpError struct {
	code    int
	message string
}

func Error(code int, message string) *HttpError {
	return &HttpError{code, message}
}

func (e *HttpError) Error() string {
	return e.message
}
