package natsmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	domain "github.com/clcsolutions/nats-tui/internal/domain/alerts"
	domaintopology "github.com/clcsolutions/nats-tui/internal/domain/topology"
	"github.com/nats-io/nats.go"
)

func (m *Monitor) HealthObservations(ctx context.Context) ([]domain.HealthObservation, error) {
	topology, err := m.Topology(ctx)
	if err != nil {
		return nil, fmt.Errorf("observe alert health topology: %w", err)
	}
	connections, err := m.ListConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("observe alert connection health: %w", err)
	}
	result := make([]domain.HealthObservation, 0, len(topology.Servers())*4+len(connections.Connections))
	for _, server := range topology.Servers() {
		observedAt := server.ObservedAt()
		if observedAt.IsZero() {
			observedAt = m.now()
		}
		resource, _ := domain.NewResource(domain.ResourceServer, server.ID().String(), "")
		result = appendObservation(result, domain.MetricSlowConsumers, float64(server.SlowConsumers()), resource, "Slow consumers detected", fmt.Sprintf("%s reports %d slow consumers", server.Name(), server.SlowConsumers()), observedAt)
		result = appendObservation(result, domain.MetricServerCPU, server.CPU(), resource, "High server CPU", fmt.Sprintf("%s CPU is %.1f%%", server.Name(), server.CPU()), observedAt)
		result = appendObservation(result, domain.MetricServerMemory, float64(server.Memory()), resource, "High server memory", fmt.Sprintf("%s memory usage is %d bytes", server.Name(), server.Memory()), observedAt)
		for _, link := range server.Links() {
			value := float64(0)
			if link.Health() == domaintopology.HealthDown {
				value = 1
			}
			result = appendObservation(result, domain.MetricLinkDown, value, resource, "NATS link down", fmt.Sprintf("%s link to %s is %s", link.Kind(), link.Remote(), link.Health()), observedAt)
		}
	}
	for _, connection := range connections.Connections {
		observedAt := connections.ObservedAt
		if observedAt.IsZero() {
			observedAt = m.now()
		}
		resource, _ := domain.NewResource(domain.ResourceConnection, connection.ServerID, strconv.FormatUint(connection.CID, 10))
		result = appendObservation(result, domain.MetricPendingBytes, float64(connection.PendingBytes), resource, "Connection pending bytes", fmt.Sprintf("connection %d on %s has %d pending bytes", connection.CID, connection.ServerName, connection.PendingBytes), observedAt)
	}
	return result, nil
}

func appendObservation(values []domain.HealthObservation, metric domain.Metric, value float64, resource domain.Resource, title, description string, at time.Time) []domain.HealthObservation {
	observation, err := domain.NewHealthObservation(metric, value, resource, title, description, at)
	if err != nil {
		return values
	}
	return append(values, observation)
}

const (
	jetStreamAdvisories = "$JS.EVENT.ADVISORY.>"
	consumerACKMetrics  = "$JS.EVENT.METRIC.CONSUMER.ACK.>"
)

type advisoryMessage struct {
	subject string
	payload []byte
}

type advisoryEnvelope struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Timestamp time.Time       `json:"timestamp"`
	Stream    string          `json:"stream"`
	Consumer  string          `json:"consumer"`
	Action    string          `json:"action"`
	Subject   string          `json:"subject"`
	Response  json.RawMessage `json:"response"`
}

// WatchAlerts translates the open-ended NATS advisory protocol into the
// application's stable alert language. A malformed advisory is reported on
// the error channel without terminating the subscription.
func (m *Monitor) WatchAlerts(ctx context.Context) (<-chan *domain.Alert, <-chan error, error) {
	incomingNATS := make(chan *nats.Msg, 512)
	subscriptions := make([]*nats.Subscription, 0, 2)
	for _, subject := range []string{jetStreamAdvisories, consumerACKMetrics} {
		subscription, err := m.connection.ChanSubscribe(subject, incomingNATS)
		if err != nil {
			unsubscribeAll(subscriptions)
			return nil, nil, fmt.Errorf("watch NATS alerts on %q: %w", subject, err)
		}
		subscriptions = append(subscriptions, subscription)
	}
	if err := m.connection.FlushWithContext(ctx); err != nil {
		unsubscribeAll(subscriptions)
		return nil, nil, fmt.Errorf("activate NATS alert watch: %w", err)
	}

	incoming := make(chan advisoryMessage, 512)
	events := make(chan *domain.Alert, 512)
	errors := make(chan error, 32)
	go func() {
		defer close(incoming)
		defer unsubscribeAll(subscriptions)
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-incomingNATS:
				if message == nil {
					continue
				}
				mapped := advisoryMessage{subject: message.Subject, payload: append([]byte(nil), message.Data...)}
				select {
				case incoming <- mapped:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go forwardAdvisories(ctx, incoming, events, errors, m.now)
	return events, errors, nil
}

func forwardAdvisories(ctx context.Context, incoming <-chan advisoryMessage, events chan<- *domain.Alert, errorEvents chan<- error, now func() time.Time) {
	defer close(events)
	defer close(errorEvents)
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-incoming:
			if !ok {
				return
			}
			alert, err := mapAdvisory(message.subject, message.payload, now())
			if err != nil {
				select {
				case errorEvents <- err:
				case <-ctx.Done():
					return
				}
				continue
			}
			select {
			case events <- alert:
			case <-ctx.Done():
				return
			}
		}
	}
}

func mapAdvisory(subject string, payload []byte, receivedAt time.Time) (*domain.Alert, error) {
	var envelope advisoryEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode NATS alert %q: %w", subject, err)
	}

	id := strings.TrimSpace(envelope.ID)
	if id == "" {
		hash := sha256.Sum256(append(append([]byte(subject), 0), payload...))
		id = "nats-" + hex.EncodeToString(hash[:12])
	}
	occurredAt := envelope.Timestamp
	if occurredAt.IsZero() {
		occurredAt = receivedAt
	}
	// Advisory timestamps originate at the server. Preserve the occurrence
	// time and tolerate positive clock skew at the client boundary.
	if receivedAt.Before(occurredAt) {
		receivedAt = occurredAt
	}

	classification := classifyAdvisory(subject, envelope)
	return domain.NewAlert(domain.AlertInput{
		ID:          id,
		Kind:        envelope.Type,
		Title:       classification.title,
		Description: classification.description,
		Source:      subject,
		Category:    classification.category,
		Severity:    classification.severity,
		Resource:    classification.resource,
		OccurredAt:  occurredAt,
		ReceivedAt:  receivedAt,
		Payload:     payload,
	})
}

type advisoryClassification struct {
	title, description string
	category           domain.Category
	severity           domain.Severity
	resource           domain.Resource
}

func classifyAdvisory(subject string, envelope advisoryEnvelope) advisoryClassification {
	upper := strings.ToUpper(subject)
	result := advisoryClassification{
		title:       "NATS advisory",
		description: firstNonEmpty(envelope.Action, envelope.Type, subject),
		category:    domain.CategoryUnknown,
		severity:    domain.SeverityInfo,
	}

	switch {
	case subject == "$JS.EVENT.ADVISORY.API":
		result.category, result.title = domain.CategoryAudit, "JetStream API request"
		if description, ok := responseError(envelope.Response); ok {
			result.severity, result.title, result.description = domain.SeverityWarning, "JetStream API error", description
		}
	case strings.Contains(upper, ".CONSUMER."):
		result.category, result.title = domain.CategoryConsumer, consumerAdvisoryTitle(upper)
		result.severity = consumerAdvisorySeverity(upper)
		result.resource = consumerResource(subject, envelope)
	case strings.Contains(upper, ".STREAM."):
		result.category, result.title = domain.CategoryStream, streamAdvisoryTitle(upper)
		result.severity = streamAdvisorySeverity(upper)
		result.resource = streamResource(subject, envelope)
	case strings.Contains(upper, ".API.LIMIT_REACHED"):
		result.category, result.severity, result.title = domain.CategoryJetStream, domain.SeverityWarning, "JetStream API limit reached"
	}
	return result
}

func responseError(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err == nil {
			raw = []byte(encoded)
		}
	}
	var response struct {
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Error == nil {
		return "", false
	}
	return firstNonEmpty(response.Error.Description, "JetStream API request failed"), true
}

func consumerAdvisorySeverity(subject string) domain.Severity {
	if strings.Contains(subject, "MAX_DELIVER") || strings.Contains(subject, "MSG_TERMINATED") || strings.Contains(subject, "QUORUM_LOST") {
		return domain.SeverityCritical
	}
	if strings.Contains(subject, "MSG_NAKED") {
		return domain.SeverityWarning
	}
	return domain.SeverityInfo
}

func consumerAdvisoryTitle(subject string) string {
	switch {
	case strings.Contains(subject, "MAX_DELIVER"):
		return "Consumer max deliveries reached"
	case strings.Contains(subject, "MSG_TERMINATED"):
		return "Consumer delivery terminated"
	case strings.Contains(subject, "MSG_NAKED"):
		return "Consumer message NAK"
	case strings.Contains(subject, ".ACK."):
		return "Consumer acknowledgement"
	default:
		return "Consumer advisory"
	}
}

func streamAdvisorySeverity(subject string) domain.Severity {
	if strings.Contains(subject, "QUORUM_LOST") {
		return domain.SeverityCritical
	}
	return domain.SeverityInfo
}

func streamAdvisoryTitle(subject string) string {
	if strings.Contains(subject, "QUORUM_LOST") {
		return "Stream quorum lost"
	}
	return "Stream advisory"
}

func consumerResource(subject string, envelope advisoryEnvelope) domain.Resource {
	stream, consumer := strings.TrimSpace(envelope.Stream), strings.TrimSpace(envelope.Consumer)
	tokens := strings.Split(subject, ".")
	if consumer == "" && len(tokens) > 0 {
		consumer = tokens[len(tokens)-1]
	}
	if stream == "" && len(tokens) > 1 {
		stream = tokens[len(tokens)-2]
	}
	resource, err := domain.NewResource(domain.ResourceConsumer, stream, consumer)
	if err != nil {
		return domain.Resource{}
	}
	return resource
}

func streamResource(subject string, envelope advisoryEnvelope) domain.Resource {
	stream := strings.TrimSpace(envelope.Stream)
	if stream == "" {
		tokens := strings.Split(subject, ".")
		stream = tokens[len(tokens)-1]
	}
	resource, err := domain.NewResource(domain.ResourceStream, stream, "")
	if err != nil {
		return domain.Resource{}
	}
	return resource
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "NATS event"
}
