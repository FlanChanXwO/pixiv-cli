// Package diagnostics contains the opt-in, protocol-neutral debug event
// contract shared by CLI and MCP adapters. It deliberately accepts typed
// fields only; raw URLs, headers, bodies, cookies and arbitrary log text do
// not belong in this package.
package diagnostics

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

// Presenter renders typed events as synchronized English narrative lines.
type Presenter struct {
	writer io.Writer
	now    func() time.Time

	mu  sync.Mutex
	err error
}

// NewPresenter creates a real-time presenter using the local clock.
func NewPresenter(writer io.Writer) *Presenter {
	return NewPresenterWithClock(writer, time.Now)
}

// NewPresenterWithClock is deterministic-test friendly while preserving the
// same production rendering path.
func NewPresenterWithClock(writer io.Writer, now func() time.Time) *Presenter {
	if writer == nil {
		writer = io.Discard
	}
	if now == nil {
		now = time.Now
	}
	return &Presenter{writer: writer, now: now}
}

// Emit renders and writes one complete line. The first writer error is kept;
// business execution is never cancelled by a diagnostic sink failure.
func (p *Presenter) Emit(event Event) {
	if p == nil {
		return
	}
	line := fmt.Sprintf("[%s] %s %s\n", event.Module, p.now().Format("15:04:05"), narrative(event))
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := io.WriteString(p.writer, line); err != nil && p.err == nil {
		p.err = err
	}
}

// Err returns the first writer error observed by the presenter.
func (p *Presenter) Err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func narrative(event Event) string {
	operation := safeText(event.Operation)
	resource := safeText(event.Resource)
	route := safeText(event.Route)
	request := requestLabel(event.RequestID)

	switch event.Kind {
	case EventStarted:
		if request != "" {
			return fmt.Sprintf("Started request %s for %s.", request, operationOr(operation, "the operation"))
		}
		return fmt.Sprintf("Started %s.", operationOr(operation, "the operation"))
	case EventCompleted:
		if request != "" {
			if event.Duration > 0 {
				return fmt.Sprintf("Request %s completed successfully in %s.", request, formatDuration(event.Duration))
			}
			return fmt.Sprintf("Request %s completed successfully.", request)
		}
		if event.Duration > 0 {
			return fmt.Sprintf("%s completed successfully in %s.", operationOr(operation, "The operation"), formatDuration(event.Duration))
		}
		return fmt.Sprintf("%s completed successfully.", capitalize(operationOr(operation, "the operation")))
	case EventFailed:
		failure := reasonText(event.Reason)
		if request != "" {
			return fmt.Sprintf("Request %s failed during %s%s.", request, operationOr(operation, "the operation"), failure)
		}
		return fmt.Sprintf("%s failed%s.", capitalize(operationOr(operation, "the operation")), failure)
	case EventNetworkRequest:
		verb := operationOr(operation, "the request")
		if resource != "" {
			verb += " " + resource
		}
		if event.Status > 0 {
			verb = fmt.Sprintf("received HTTP %d while %s", event.Status, verb)
		}
		if route != "" {
			verb += " through the " + route
		}
		if event.Proxy != "" {
			if proxy := safeAddress(event.Proxy); proxy != "" {
				verb += " via proxy " + proxy
			}
		}
		if event.UserAgent != "" {
			if agent := safeHeader(event.UserAgent); agent != "" {
				verb += " with User-Agent " + agent
			}
		}
		if request != "" {
			return fmt.Sprintf("Request %s is %s.", request, verb)
		}
		return capitalizeFirst(verb) + "."
	case EventChallenge:
		if event.Status > 0 && request != "" {
			return fmt.Sprintf("Cloudflare challenged request %s with HTTP %d.", request, event.Status)
		}
		if request != "" {
			return fmt.Sprintf("Cloudflare challenged request %s.", request)
		}
		return "Cloudflare issued a challenge."
	case EventSolverStarted:
		if request != "" {
			return fmt.Sprintf("Request %s requires fresh Cloudflare clearance.", request)
		}
		return "Fresh Cloudflare clearance is required."
	case EventSolverCompleted:
		if request != "" {
			return fmt.Sprintf("Clearance was acquired; request %s will be replayed natively.", request)
		}
		return "Clearance was acquired for native replay."
	case EventReplay:
		if request != "" {
			if route != "" {
				return fmt.Sprintf("Request %s is replaying through the %s.", request, route)
			}
			return fmt.Sprintf("Request %s is replaying natively.", request)
		}
		return "The request is replaying natively."
	case EventDownload:
		download := "Download operation"
		if operation != "" {
			download = capitalize(operation)
		}
		if event.Count > 0 {
			download += fmt.Sprintf(" discovered %d resources", event.Count)
		}
		if target := safeText(event.Target); target != "" {
			download += " at " + target
		}
		return download + "."
	case EventAccount:
		message := operationOr(operation, "Account pool")
		if resource != "" {
			message += " " + resource
		}
		if event.Count > 0 {
			message += fmt.Sprintf(" found %d accounts", event.Count)
		}
		return capitalize(message) + "."
	case EventConfiguration:
		return capitalize(operationOr(operation, "Configuration")) + "."
	default:
		return "A diagnostic event occurred."
	}
}

func requestLabel(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

func operationOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func reasonText(reason Reason) string {
	switch reason {
	case ReasonCommandFailed:
		return ": the command returned an error"
	case ReasonToolFailed:
		return ": the tool returned an error"
	case ReasonChallenge:
		return ": a challenge is still required"
	case ReasonSolverUnavailable:
		return ": the solver is unavailable"
	case ReasonSolverFailed:
		return ": the solver failed"
	case ReasonMalformedSolver:
		return ": the solver response is malformed"
	case ReasonAccountFrozen:
		return ": the selected account is frozen"
	case ReasonAccountExhausted:
		return ": the account pool is exhausted"
	case ReasonConfigurationFailed:
		return ": configuration failed"
	case ReasonAuthenticationFailed:
		return ": authentication failed"
	default:
		return ""
	}
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return strconv.FormatFloat(duration.Seconds(), 'f', 1, 64) + " seconds"
}

func capitalize(value string) string {
	return capitalizeFirst(value)
}

func capitalizeFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func safeText(value string) string {
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	if strings.ContainsAny(value, "?#") {
		if parsed, err := url.Parse(value); err == nil {
			parsed.User = nil
			parsed.RawQuery = ""
			parsed.ForceQuery = false
			parsed.Fragment = ""
			return parsed.String()
		}
		if index := strings.IndexAny(value, "?#"); index >= 0 {
			return value[:index]
		}
	}
	return value
}

func safeAddress(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return safeText(value)
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func safeHeader(value string) string {
	if strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}
