package errors

type Http struct {
	Code    int
	Message string
}

func (e *Http) Error() string {
	return e.Message
}
