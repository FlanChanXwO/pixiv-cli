// Package diagnostics contains the opt-in, protocol-neutral debug event
// contract shared by CLI and MCP adapters. It deliberately accepts typed
// fields only; raw URLs, headers, bodies, cookies and arbitrary log text do
// not belong in this package. Narrative rendering is owned by the CLI and
// lives in internal/cli/diagnostics.
package diagnostics

import (
	"context"
	"time"
)

// Module is the stable product and subsystem prefix shown by the presenter.
type Module string

const (
	ModulePixivCLI       Module = "Pixiv CLI"
	ModuleFanboxCLI      Module = "FANBOX CLI"
	ModulePixivMCP       Module = "Pixiv MCP server"
	ModuleFanboxMCP      Module = "FANBOX MCP server"
	ModulePixivAccount   Module = "Pixiv account pool"
	ModulePixivAuth      Module = "Pixiv authentication"
	ModuleFanboxAuth     Module = "FANBOX authentication"
	ModulePixivAuthDB    Module = "Pixiv authentication database"
	ModuleFanboxAuthDB   Module = "FANBOX authentication database"
	ModulePixivConfig    Module = "Pixiv configuration"
	ModuleFanboxConfig   Module = "FANBOX configuration"
	ModulePixivNetwork   Module = "Pixiv network"
	ModuleFanboxNetwork  Module = "FANBOX network"
	ModuleFanboxSolver   Module = "FANBOX FlareSolverr"
	ModulePixivDownload  Module = "Pixiv download"
	ModuleFanboxDownload Module = "FANBOX download"

	// Descriptive aliases keep call sites readable when they distinguish the
	// server subsystem from the product name.
	ModulePixivMCPServer  = ModulePixivMCP
	ModuleFanboxMCPServer = ModuleFanboxMCP
)

// EventKind is a closed set of diagnostic lifecycle stages. Event data is
// intentionally finite and safe; callers cannot supply preformatted output.
type EventKind string

const (
	EventStarted         EventKind = "started"
	EventCompleted       EventKind = "completed"
	EventFailed          EventKind = "failed"
	EventNetworkRequest  EventKind = "network_request"
	EventChallenge       EventKind = "challenge"
	EventSolverStarted   EventKind = "solver_started"
	EventSolverCompleted EventKind = "solver_completed"
	EventReplay          EventKind = "replay"
	EventDownload        EventKind = "download"
	EventAccount         EventKind = "account"
	EventConfiguration   EventKind = "configuration"
)

// Reason is a stable, non-secret explanation selected by the emitting layer.
// Raw Go errors do not belong in Event; the command boundary still returns the
// original business error through its normal error path.
type Reason string

const (
	ReasonNone                 Reason = ""
	ReasonCommandFailed        Reason = "command failed"
	ReasonToolFailed           Reason = "tool failed"
	ReasonChallenge            Reason = "challenge required"
	ReasonSolverUnavailable    Reason = "solver unavailable"
	ReasonSolverFailed         Reason = "solver failed"
	ReasonMalformedSolver      Reason = "malformed solver response"
	ReasonAccountFrozen        Reason = "account frozen"
	ReasonAccountExhausted     Reason = "account pool exhausted"
	ReasonConfigurationFailed  Reason = "configuration failed"
	ReasonAuthenticationFailed Reason = "authentication failed"
)

// Event is the complete safe diagnostic payload. Resource and Target are
// non-secret identifiers or local destinations supplied by trusted adapters;
// the presenter removes URL userinfo, query and fragment before rendering.
type Event struct {
	Module    Module
	Kind      EventKind
	Operation string
	Resource  string
	Route     string
	Target    string
	Proxy     string
	UserAgent string
	Reason    Reason
	Status    int
	Count     int
	RequestID uint64
	Duration  time.Duration
}

// Sink receives one typed event at a time.
type Sink interface {
	Emit(Event)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(Event)

func (f SinkFunc) Emit(event Event) {
	if f != nil {
		f(event)
	}
}

type nopSink struct{}

func (nopSink) Emit(Event) {}

// Nop returns the sink used when diagnostics are disabled.
func Nop() Sink { return nopSink{} }

type scopeContextKey struct{}

// Scope binds a sink, module and optional MCP-local request number to a
// context. A zero request ID is reserved for a top-level CLI operation.
type Scope struct {
	sink      Sink
	module    Module
	requestID uint64
}

// WithScope installs an explicit diagnostic scope. A nil sink is converted to
// a no-op sink so ordinary SDK callers remain silent and nil-safe.
func WithScope(ctx context.Context, sink Sink, module Module, requestID uint64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if sink == nil {
		sink = Nop()
	}
	return context.WithValue(ctx, scopeContextKey{}, Scope{sink: sink, module: module, requestID: requestID})
}

// WithChildScope changes only the module and request number while preserving
// the parent sink. Without a parent scope it deliberately remains a no-op;
// this keeps public SDK usage quiet outside the CLI/MCP bootstrap.
func WithChildScope(ctx context.Context, module Module, requestID uint64) context.Context {
	scope, ok := ScopeFromContext(ctx)
	if !ok {
		return ctx
	}
	return WithScope(ctx, scope.sink, module, requestID)
}

// ScopeFromContext returns the current scope for tests and controlled adapter
// composition. The sink itself is intentionally not exported as a field.
func ScopeFromContext(ctx context.Context) (Scope, bool) {
	if ctx == nil {
		return Scope{}, false
	}
	scope, ok := ctx.Value(scopeContextKey{}).(Scope)
	return scope, ok
}

// Emit sends an event when the context carries a diagnostic scope.
func Emit(ctx context.Context, event Event) {
	scope, ok := ScopeFromContext(ctx)
	if !ok {
		return
	}
	if event.Module == "" {
		event.Module = scope.module
	}
	event.RequestID = scope.requestID
	scope.sink.Emit(event)
}

// Emit makes Scope implement a convenient direct sink-like API.
func (scope Scope) Emit(event Event) {
	if scope.sink == nil {
		return
	}
	if event.Module == "" {
		event.Module = scope.module
	}
	event.RequestID = scope.requestID
	scope.sink.Emit(event)
}
