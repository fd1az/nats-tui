package consumers

import (
	"context"
	"testing"
	"time"
)

type configOperatorStub struct {
	command ConfigCommand
	err     error
}

func (s *configOperatorStub) OperateConsumerConfig(_ context.Context, command ConfigCommand) error {
	s.command = command
	return s.err
}

func TestDefinitionValidatesCrossFieldInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"missing identity", func(value *Definition) { value.Name = "" }},
		{"start sequence", func(value *Definition) { value.DeliverPolicy = DeliverByStartSequence }},
		{"start time", func(value *Definition) { value.DeliverPolicy = DeliverByStartTime }},
		{"push fields on pull", func(value *Definition) { value.FlowControl = true; value.IdleHeartbeat = time.Second }},
		{"flow control heartbeat", func(value *Definition) { value.DeliverSubject = "deliver.worker"; value.FlowControl = true }},
		{"backoff exceeds deliveries", func(value *Definition) {
			value.MaxDeliver = 1
			value.Backoff = []time.Duration{time.Second, time.Minute}
		}},
		{"invalid sample", func(value *Definition) { value.SampleFrequency = "101%" }},
		{"duplicate filters", func(value *Definition) { value.FilterSubjects = []string{"events.*", "events.*"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := DefaultDefinition("EVENTS", "worker")
			test.mutate(&definition)
			if err := definition.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestDefinitionAcceptsAdvancedPushConsumer(t *testing.T) {
	start := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	definition := DefaultDefinition("EVENTS", "worker")
	definition.DeliverPolicy = DeliverByStartTime
	definition.StartTime = &start
	definition.DeliverSubject = "deliver.worker"
	definition.DeliverGroup = "workers"
	definition.FlowControl = true
	definition.IdleHeartbeat = 5 * time.Second
	definition.MaxDeliver = 3
	definition.Backoff = []time.Duration{time.Second, 5 * time.Second}
	definition.FilterSubjects = []string{"events.us.*", "events.eu.*"}
	definition.SampleFrequency = "25%"
	definition.Metadata = map[string]string{"team": "payments"}
	if err := definition.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigureHandlerValidatesBeforeCallingPort(t *testing.T) {
	operator := &configOperatorStub{}
	handler := NewConfigureHandler(operator)
	if err := handler.Handle(context.Background(), ConfigCommand{Action: ConfigCreate, Definition: DefaultDefinition("EVENTS", "worker")}); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if operator.command.Definition.Name != "worker" {
		t.Fatalf("command = %#v", operator.command)
	}
	if err := handler.Handle(context.Background(), ConfigCommand{Action: ConfigCreate, Definition: Definition{}}); err == nil {
		t.Fatal("invalid definition reached operator")
	}
}

func TestSummaryDefinitionReturnsDetachedConfiguration(t *testing.T) {
	summary := Summary{Stream: "EVENTS", Name: "worker", Config: Definition{FilterSubjects: []string{"events.>"}, Metadata: map[string]string{"team": "edge"}}}
	definition := summary.Definition()
	definition.FilterSubjects[0] = "changed"
	definition.Metadata["team"] = "changed"
	if summary.Config.FilterSubjects[0] != "events.>" || summary.Config.Metadata["team"] != "edge" {
		t.Fatal("Definition() leaked mutable configuration")
	}
}
