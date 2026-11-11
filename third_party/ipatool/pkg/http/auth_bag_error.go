package http

// AuthenticationBagAttempt records one request for public Apple configuration.
// The bridge renders only the fixed stage, transport and numeric status.
type AuthenticationBagAttempt struct {
	Transport  string
	Stage      string
	StatusCode int
	Cause      error
}

type AuthenticationBagError struct {
	Attempts []AuthenticationBagAttempt
}

func (e *AuthenticationBagError) Error() string {
	return "read Apple authentication configuration"
}
