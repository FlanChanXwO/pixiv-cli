// Package diagnostics renders safe events from the diagnostics core as
// synchronized English narrative lines for CLI --debug output. It is owned by
// the CLI and never defines event/reason/sink semantics itself.
package diagnostics

import (
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	core "github.com/FlanChanXwO/pixiv-cli/internal/diagnostics"
)

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
func (p *Presenter) Emit(event core.Event) {
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

func narrative(event core.Event) string {
	operation := safeText(event.Operation)
	resource := safeText(event.Resource)
	route := safeText(event.Route)
	request := requestLabel(event.RequestID)

	switch event.Kind {
	case core.EventStarted:
		if request != "" {
			return fmt.Sprintf("Started request %s for %s.", request, operationOr(operation, "the operation"))
		}
		return fmt.Sprintf("Started %s.", operationOr(operation, "the operation"))
	case core.EventCompleted:
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
	case core.EventFailed:
		failure := reasonText(event.Reason)
		if request != "" {
			return fmt.Sprintf("Request %s failed during %s%s.", request, operationOr(operation, "the operation"), failure)
		}
		return fmt.Sprintf("%s failed%s.", capitalize(operationOr(operation, "the operation")), failure)
	case core.EventNetworkRequest:
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
	case core.EventChallenge:
		if event.Status > 0 && request != "" {
			return fmt.Sprintf("Cloudflare challenged request %s with HTTP %d.", request, event.Status)
		}
		if request != "" {
			return fmt.Sprintf("Cloudflare challenged request %s.", request)
		}
		return "Cloudflare issued a challenge."
	case core.EventSolverStarted:
		if request != "" {
			return fmt.Sprintf("Request %s requires fresh Cloudflare clearance.", request)
		}
		return "Fresh Cloudflare clearance is required."
	case core.EventSolverCompleted:
		if request != "" {
			return fmt.Sprintf("Clearance was acquired; request %s will be replayed natively.", request)
		}
		return "Clearance was acquired for native replay."
	case core.EventReplay:
		if request != "" {
			if route != "" {
				return fmt.Sprintf("Request %s is replaying through the %s.", request, route)
			}
			return fmt.Sprintf("Request %s is replaying natively.", request)
		}
		return "The request is replaying natively."
	case core.EventDownload:
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
	case core.EventAccount:
		message := operationOr(operation, "Account pool")
		if resource != "" {
			message += " " + resource
		}
		if event.Count > 0 {
			message += fmt.Sprintf(" found %d accounts", event.Count)
		}
		return capitalize(message) + "."
	case core.EventConfiguration:
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

func reasonText(reason core.Reason) string {
	switch reason {
	case core.ReasonCommandFailed:
		return ": the command returned an error"
	case core.ReasonToolFailed:
		return ": the tool returned an error"
	case core.ReasonChallenge:
		return ": a challenge is still required"
	case core.ReasonSolverUnavailable:
		return ": the solver is unavailable"
	case core.ReasonSolverFailed:
		return ": the solver failed"
	case core.ReasonMalformedSolver:
		return ": the solver response is malformed"
	case core.ReasonAccountFrozen:
		return ": the selected account is frozen"
	case core.ReasonAccountExhausted:
		return ": the account pool is exhausted"
	case core.ReasonConfigurationFailed:
		return ": configuration failed"
	case core.ReasonAuthenticationFailed:
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
