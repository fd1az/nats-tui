package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/messages"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type messageHistoryFlags struct {
	subject, startTime, payloadFilter string
	startSequence                     uint64
	limit                             int
}

func newMessageCommand(values *viper.Viper, manager MessageManager, streamManager StreamManager, publisher CorePublisher) *cobra.Command {
	command := &cobra.Command{Use: "message", Aliases: []string{"messages", "msg"}, Short: "Inspect and operate stored JetStream messages", Args: cobra.NoArgs}
	command.AddCommand(
		newMessageHistoryCommand(values, manager),
		newMessageExportCommand(values, manager),
		newMessageRepublishCommand(values, manager, publisher),
		newMessageDeleteCommand(values, streamManager),
	)
	return command
}

func bindMessageHistoryFlags(command *cobra.Command, flags *messageHistoryFlags) {
	command.Flags().StringVar(&flags.subject, "subject", ">", "subject filter; supports * and > wildcards")
	command.Flags().Uint64Var(&flags.startSequence, "start-sequence", 0, "inclusive lower stream sequence")
	command.Flags().StringVar(&flags.startTime, "start-time", "", "inclusive RFC3339 timestamp")
	command.Flags().IntVar(&flags.limit, "limit", 50, "maximum messages, newest first (1-10000)")
	command.Flags().StringVar(&flags.payloadFilter, "payload-filter", "", "JSONPath or JMESPath payload expression, optionally == value")
}

func (flags messageHistoryFlags) query(stream string) (messages.HistoryQuery, error) {
	query := messages.HistoryQuery{Stream: stream, Subject: flags.subject, StartSequence: flags.startSequence, Limit: flags.limit, PayloadExpression: flags.payloadFilter}
	if strings.TrimSpace(flags.startTime) != "" {
		parsed, err := time.Parse(time.RFC3339, flags.startTime)
		if err != nil {
			return messages.HistoryQuery{}, fmt.Errorf("start time must be RFC3339: %w", err)
		}
		query.StartTime = &parsed
	}
	if err := query.Validate(); err != nil {
		return messages.HistoryQuery{}, err
	}
	return query, nil
}

func newMessageHistoryCommand(values *viper.Viper, manager MessageManager) *cobra.Command {
	var flags messageHistoryFlags
	command := &cobra.Command{Use: "history STREAM", Aliases: []string{"list", "ls"}, Short: "Query stored message history with server-side bounds", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if manager == nil {
			return errors.New("message manager is not configured")
		}
		query, err := flags.query(args[0])
		if err != nil {
			return err
		}
		records, err := manager.BrowseStoredMessages(cmd.Context(), config.Load(values), query)
		if err != nil {
			return err
		}
		envelopes := make([]json.RawMessage, 0, len(records))
		for _, record := range records {
			complete, completeErr := messages.CompleteMessage(record)
			if completeErr != nil {
				return completeErr
			}
			envelopes = append(envelopes, json.RawMessage(complete))
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(envelopes)
	}}
	bindMessageHistoryFlags(command, &flags)
	return command
}

func newMessageExportCommand(values *viper.Viper, manager MessageManager) *cobra.Command {
	var flags messageHistoryFlags
	var format string
	var force bool
	command := &cobra.Command{Use: "export STREAM PATH", Short: "Export a bounded message query atomically", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if manager == nil {
			return errors.New("message manager is not configured")
		}
		query, err := flags.query(args[0])
		if err != nil {
			return err
		}
		result, err := manager.ExportStoredMessages(cmd.Context(), config.Load(values), messages.ExportRequest{Query: query, Path: args[1], Format: messages.ExportFormat(strings.ToLower(format)), Force: force})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "exported %d messages (%d bytes, %s) to %s\n", result.Messages, result.Bytes, result.Format, result.Path)
		return err
	}}
	bindMessageHistoryFlags(command, &flags)
	command.Flags().StringVar(&format, "format", string(messages.ExportJSONL), "output format: jsonl, json, or raw")
	command.Flags().BoolVar(&force, "force", false, "atomically replace an existing destination")
	return command
}

func newMessageRepublishCommand(values *viper.Viper, manager MessageManager, publisher CorePublisher) *cobra.Command {
	return &cobra.Command{Use: "republish STREAM SEQUENCE [SUBJECT]", Short: "Republish a stored message with its payload and headers", Args: cobra.RangeArgs(2, 3), RunE: func(cmd *cobra.Command, args []string) error {
		if manager == nil || publisher == nil {
			return errors.New("message manager and Core NATS publisher are required")
		}
		sequence, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || sequence == 0 {
			return errors.New("sequence must be a positive integer")
		}
		records, err := manager.BrowseStoredMessages(cmd.Context(), config.Load(values), messages.HistoryQuery{Stream: args[0], Subject: ">", StartSequence: sequence, EndSequence: sequence, Limit: 1})
		if err != nil {
			return err
		}
		var found *messages.Record
		for index := range records {
			if records[index].Sequence == sequence {
				found = &records[index]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("message %d was not found in stream %s", sequence, args[0])
		}
		subject := found.Subject
		if len(args) == 3 {
			subject = args[2]
		}
		message, err := domain.NewMessage(subject, found.Payload, found.Headers)
		if err != nil {
			return err
		}
		if err := publisher.PublishCoreMessage(cmd.Context(), config.Load(values), message); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "republished stream %s message %d to %s\n", args[0], sequence, message.Subject())
		return err
	}}
}

func newMessageDeleteCommand(values *viper.Viper, manager StreamManager) *cobra.Command {
	var secure, yes bool
	command := &cobra.Command{Use: "delete STREAM SEQUENCE", Aliases: []string{"rm"}, Short: "Permanently delete one stored message", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a stored message requires --yes")
		}
		if manager == nil {
			return errors.New("stream manager is not configured")
		}
		sequence, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || sequence == 0 {
			return errors.New("sequence must be a positive integer")
		}
		if err := manager.DeleteStoredStreamMessage(cmd.Context(), config.Load(values), streams.MessageDeleteCommand{Stream: args[0], Sequence: sequence, Secure: secure}); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted message %d from stream %s\n", sequence, args[0])
		return err
	}}
	command.Flags().BoolVar(&secure, "secure", true, "overwrite message data before deletion")
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}
