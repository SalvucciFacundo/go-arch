package hooks

import "context"

// ---------------------------------------------------------------------------
// FakeRunner — a CommandRunner that records calls and returns configured
// responses. Defined in a non-test file so external test packages (scaffold,
// mcp) can inject it.
// ---------------------------------------------------------------------------

// FakeCall records one invocation of Run.
type FakeCall struct {
	Name string
	Args []string
	Opts RunOpts
}

// FakeResponse represents a single canned response from FakeRunner.
type FakeResponse struct {
	ExitCode int
	RunErr   error
}

// FakeRunner implements CommandRunner for testing.
//
// When Responses is non-empty, each Run call consumes one response from
// the queue. When empty, the single ExitCode/RunErr is used for every
// call (backward-compatible default).
type FakeRunner struct {
	Calls     []FakeCall
	ExitCode  int
	RunErr    error
	Responses []FakeResponse
}

// Run records the call in f.Calls and returns the next configured response.
func (f *FakeRunner) Run(_ context.Context, name string, args []string, opts RunOpts) (int, error) {
	f.Calls = append(f.Calls, FakeCall{Name: name, Args: append([]string(nil), args...), Opts: opts})

	if len(f.Responses) > 0 {
		resp := f.Responses[0]
		f.Responses = f.Responses[1:]
		return resp.ExitCode, resp.RunErr
	}
	return f.ExitCode, f.RunErr
}
