package http

// AuthenticationWireError marks a failure at the HTTP boundary.
type AuthenticationWireError struct {
	ReadingResponse bool
	Err             error
}

func (e *AuthenticationWireError) Error() string {
	return "Apple authentication HTTP request: " + e.Err.Error()
}
func (e *AuthenticationWireError) Unwrap() error { return e.Err }

// AuthenticationFrameworkError marks a failure during local Store processing.
type AuthenticationFrameworkError struct{ Err error }

func (e *AuthenticationFrameworkError) Error() string {
	return "Store authentication framework: " + e.Err.Error()
}
func (e *AuthenticationFrameworkError) Unwrap() error { return e.Err }
