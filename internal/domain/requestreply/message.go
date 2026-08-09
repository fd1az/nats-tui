// Package requestreply models validated Core NATS publish and request/reply values.
package requestreply

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Subject struct{ value string }

func NewSubject(value string) (Subject, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Subject{}, fmt.Errorf("subject is required")
	}
	if strings.ContainsAny(value, "*> \t\r\n") || strings.Contains(value, "..") || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return Subject{}, fmt.Errorf("subject must be concrete and contain no wildcards, whitespace, or empty tokens")
	}
	return Subject{value: value}, nil
}

func (s Subject) String() string { return s.value }
func (s Subject) IsZero() bool   { return s.value == "" }

type Message struct {
	subject Subject
	payload []byte
	headers map[string][]string
}

func NewMessage(subject string, payload []byte, headers map[string][]string) (Message, error) {
	validatedSubject, err := NewSubject(subject)
	if err != nil {
		return Message{}, err
	}
	validatedHeaders, err := cloneHeaders(headers)
	if err != nil {
		return Message{}, err
	}
	return Message{subject: validatedSubject, payload: append([]byte(nil), payload...), headers: validatedHeaders}, nil
}

func (m Message) Subject() Subject             { return m.subject }
func (m Message) Payload() []byte              { return append([]byte(nil), m.payload...) }
func (m Message) Headers() map[string][]string { headers, _ := cloneHeaders(m.headers); return headers }
func (m Message) IsZero() bool                 { return m.subject.IsZero() }

func cloneHeaders(headers map[string][]string) (map[string][]string, error) {
	if headers == nil {
		return nil, nil
	}
	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, ": \t\r\n") {
			return nil, fmt.Errorf("header name %q is invalid", name)
		}
		result[name] = make([]string, len(values))
		for index, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				return nil, fmt.Errorf("header %q contains a line break", name)
			}
			result[name][index] = value
		}
	}
	return result, nil
}

type Request struct {
	message    Message
	timeout    time.Duration
	maxReplies int
}

func NewRequest(message Message, timeout time.Duration, maxReplies int) (Request, error) {
	if message.IsZero() {
		return Request{}, fmt.Errorf("request message is required")
	}
	if timeout <= 0 {
		return Request{}, fmt.Errorf("request timeout must be positive")
	}
	if maxReplies < 1 || maxReplies > 1000 {
		return Request{}, fmt.Errorf("max replies must be between 1 and 1000")
	}
	return Request{message: message, timeout: timeout, maxReplies: maxReplies}, nil
}

func (r Request) Message() Message       { return r.message }
func (r Request) Timeout() time.Duration { return r.timeout }
func (r Request) MaxReplies() int        { return r.maxReplies }
func (r Request) IsZero() bool           { return r.message.IsZero() }

type Reply struct {
	payload    []byte
	headers    map[string][]string
	receivedAt time.Time
	latency    time.Duration
}

func NewReply(payload []byte, headers map[string][]string, receivedAt time.Time, latency time.Duration) (Reply, error) {
	validatedHeaders, err := cloneHeaders(headers)
	if err != nil {
		return Reply{}, err
	}
	if receivedAt.IsZero() || latency < 0 {
		return Reply{}, fmt.Errorf("reply timing is invalid")
	}
	return Reply{payload: append([]byte(nil), payload...), headers: validatedHeaders, receivedAt: receivedAt, latency: latency}, nil
}

func (r Reply) Payload() []byte              { return append([]byte(nil), r.payload...) }
func (r Reply) Headers() map[string][]string { headers, _ := cloneHeaders(r.headers); return headers }
func (r Reply) ReceivedAt() time.Time        { return r.receivedAt }
func (r Reply) Latency() time.Duration       { return r.latency }

var templateNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type Template struct {
	name    string
	request Request
}

func NewTemplate(name string, request Request) (Template, error) {
	name = strings.TrimSpace(name)
	if !templateNamePattern.MatchString(name) {
		return Template{}, fmt.Errorf("template name must contain only letters, digits, dashes, or underscores")
	}
	if request.IsZero() {
		return Template{}, fmt.Errorf("template request is required")
	}
	return Template{name: name, request: request}, nil
}

func (t Template) Name() string     { return t.name }
func (t Template) Request() Request { return t.request }
