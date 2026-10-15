package commands

// ExitCodeError carries a specific process exit code through cobra's error
// return. The root command maps it to os.Exit.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error { return e.Err }

// WithExitCode wraps err so the process exits with code when the command fails.
func WithExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitCodeError{Code: code, Err: err}
}
