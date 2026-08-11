package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/config"
)

type consumerManagerStub struct {
	items         []consumers.Summary
	configCommand consumers.ConfigCommand
	command       consumers.Command
}

func (s *consumerManagerStub) ListConsumerCatalog(context.Context, config.Settings) ([]consumers.Summary, error) {
	return s.items, nil
}
func (s *consumerManagerStub) OperateConsumerConfig(_ context.Context, _ config.Settings, command consumers.ConfigCommand) error {
	s.configCommand = command
	return nil
}
func (s *consumerManagerStub) OperateConsumer(_ context.Context, _ config.Settings, command consumers.Command) error {
	s.command = command
	return nil
}

func TestConsumerCreateSupportsCompleteDeliveryConfiguration(t *testing.T) {
	service := new(consumerManagerStub)
	root := NewRoot(Dependencies{Consumers: service})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"consumer", "create", "EVENTS", "worker", "--filter", "events.us.*,events.eu.*", "--deliver", "deliver.worker", "--deliver-group", "workers", "--deliver-policy", "new", "--ack-wait", "30s", "--max-deliver", "3", "--backoff", "1s,5s", "--flow-control", "--heartbeat", "5s", "--metadata", "team=payments"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := service.configCommand.Definition
	if service.configCommand.Action != consumers.ConfigCreate || d.Stream != "EVENTS" || d.Name != "worker" || len(d.FilterSubjects) != 2 || d.DeliverSubject != "deliver.worker" || d.AckWait != 30*time.Second || len(d.Backoff) != 2 || !d.FlowControl || d.Metadata["team"] != "payments" {
		t.Fatalf("definition=%+v", d)
	}
}

func TestConsumerEditPreservesOmittedConfiguration(t *testing.T) {
	base := consumers.DefaultDefinition("EVENTS", "worker")
	base.FilterSubjects = []string{"events.>"}
	base.Metadata = map[string]string{"team": "payments"}
	base.InactiveThreshold = time.Hour
	service := &consumerManagerStub{items: []consumers.Summary{{Stream: "EVENTS", Name: "worker", Config: base}}}
	root := NewRoot(Dependencies{Consumers: service})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"consumer", "edit", "EVENTS", "worker", "--description", "settlement worker", "--max-ack-pending", "250"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := service.configCommand.Definition
	if service.configCommand.Action != consumers.ConfigUpdate || d.Description != "settlement worker" || d.MaxAckPending != 250 || d.FilterSubjects[0] != "events.>" || d.Metadata["team"] != "payments" || d.InactiveThreshold != time.Hour {
		t.Fatalf("definition=%+v", d)
	}
}

func TestConsumerDeleteRequiresConfirmation(t *testing.T) {
	service := new(consumerManagerStub)
	unsafe := NewRoot(Dependencies{Consumers: service})
	unsafe.SetArgs([]string{"consumer", "delete", "EVENTS", "worker"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("delete without --yes succeeded")
	}
	safe := NewRoot(Dependencies{Consumers: service})
	safe.SetOut(new(bytes.Buffer))
	safe.SetArgs([]string{"consumer", "delete", "EVENTS", "worker", "--yes"})
	if err := safe.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if service.command.Action != consumers.ActionDelete || service.command.Consumer.Stream() != "EVENTS" || service.command.Consumer.Name() != "worker" {
		t.Fatalf("command=%+v", service.command)
	}
}
