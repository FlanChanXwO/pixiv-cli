// Package diagnostics renders safe typed events for the CLI-owned stderr
// presenter. Event definitions and emission remain in shared/diagnostics.
package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	core "github.com/FlanChanXwO/pixiv-cli/internal/shared/diagnostics"
)

const (
	formatText = "text"
	formatJSON = "json"
)

// Presenter renders typed events as safe text or one-line JSON records.
type Presenter struct {
	writer io.Writer
	format string
	now    func() time.Time

	mu  sync.Mutex
	err error
}

// NewPresenter creates a text presenter using the local clock.
func NewPresenter(writer io.Writer) *Presenter {
	return NewPresenterWithFormat(writer, formatText, time.Now)
}

// NewPresenterWithClock is deterministic-test friendly while preserving the
// production text rendering path.
func NewPresenterWithClock(writer io.Writer, now func() time.Time) *Presenter {
	return NewPresenterWithFormat(writer, formatText, now)
}

// NewPresenterWithFormat creates a presenter for the configured stderr format.
func NewPresenterWithFormat(writer io.Writer, format string, now func() time.Time) *Presenter {
	if writer == nil {
		writer = io.Discard
	}
	if now == nil {
		now = time.Now
	}
	if format != formatJSON {
		format = formatText
	}
	return &Presenter{writer: writer, format: format, now: now}
}

// Emit renders and writes one complete record. The first writer error is kept;
// business execution is never cancelled by a diagnostic sink failure.
func (p *Presenter) Emit(event core.Event) {
	if p == nil {
		return
	}
	line, err := p.render(event)
	if err != nil {
		p.recordError(err)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := io.WriteString(p.writer, line); err != nil && p.err == nil {
		p.err = err
	}
}

// Err returns the first writer or rendering error observed by the presenter.
func (p *Presenter) Err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *Presenter) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		p.err = err
	}
}

func (p *Presenter) render(event core.Event) (string, error) {
	if p.format == formatJSON {
		record := jsonRecord{
			Time:      p.now().Format(time.RFC3339Nano),
			Level:     "DEBUG",
			Module:    safeText(string(event.Module)),
			Kind:      safeText(string(event.Kind)),
			Operation: safeText(event.Operation),
			Resource:  safeText(event.Resource),
			Route:     safeText(event.Route),
			Proxy:     safeAddress(event.Proxy),
			Reason:    safeText(string(event.Reason)),
			Status:    event.Status,
			Count:     event.Count,
			RequestID: event.RequestID,
		}
		if event.Duration != 0 {
			record.DurationMS = event.Duration.Milliseconds()
		}
		body, err := json.Marshal(record)
		if err != nil {
			return "", err
		}
		return string(body) + "\n", nil
	}
	return fmt.Sprintf("[%s] %s %s\n", event.Module, p.now().Format("15:04:05"), narrative(event)), nil
}

type jsonRecord struct {
	Time       string `json:"time"`
	Level      string `json:"level"`
	Module     string `json:"module"`
	Kind       string `json:"kind"`
	Operation  string `json:"operation,omitempty"`
	Resource   string `json:"resource,omitempty"`
	Route      string `json:"route,omitempty"`
	Proxy      string `json:"proxy,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Status     int    `json:"status,omitempty"`
	Count      int    `json:"count,omitempty"`
	RequestID  uint64 `json:"request_id,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
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
		if proxy := safeAddress(event.Proxy); proxy != "" {
			verb += " via proxy " + proxy
		}
		if request != "" {
			return fmt.Sprintf("Request %s is %s.", request, verb)
		}
		return "Request is " + verb + "."
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

func capitalize(value string) string { return capitalizeFirst(value) }

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
	if value == "" {
		return ""
	}
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
