package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/clcsolutions/nats-tui/internal/config"
	domain "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type requestServices struct {
	requester CoreRequester
	history   RequestHistory
	templates RequestTemplates
}

type messageFlags struct {
	file       string
	headers    []string
	timeout    time.Duration
	maxReplies int
}

func newPublishCommand(values *viper.Viper, publisher CorePublisher, files FileSystem) *cobra.Command {
	var flags messageFlags
	command := &cobra.Command{
		Use: "publish SUBJECT [PAYLOAD]", Short: "Publish a Core NATS message", Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if publisher == nil {
				return errors.New("Core NATS publisher is not configured")
			}
			message, err := messageFromCLI(args, flags, files)
			if err != nil {
				return err
			}
			if err := publisher.PublishCoreMessage(cmd.Context(), config.Load(values), message); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "published %d bytes to %s\n", len(message.Payload()), message.Subject())
			return err
		},
	}
	bindMessageFlags(command, &flags, false)
	return command
}

func newRequestCommand(values *viper.Viper, services requestServices, files FileSystem) *cobra.Command {
	var flags messageFlags
	command := &cobra.Command{
		Use: "request SUBJECT [PAYLOAD]", Short: "Send a Core NATS request and collect replies", Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if services.requester == nil {
				return errors.New("Core NATS requester is not configured")
			}
			request, err := requestFromCLI(args, flags, files)
			if err != nil {
				return err
			}
			exchange, err := services.requester.SendCoreRequest(cmd.Context(), config.Load(values), request)
			if err != nil {
				return err
			}
			return writeExchange(cmd, exchange)
		},
	}
	bindMessageFlags(command, &flags, true)
	command.AddCommand(newRequestHistoryCommand(values, services))
	command.AddCommand(newRequestReplayCommand(values, services))
	command.AddCommand(newRequestTemplateCommand(values, services, files))
	return command
}

func bindMessageFlags(command *cobra.Command, flags *messageFlags, request bool) {
	command.Flags().StringVar(&flags.file, "file", "", "read payload from file")
	command.Flags().StringSliceVarP(&flags.headers, "header", "H", nil, "header name=value; repeat for multiple values")
	if request {
		command.Flags().DurationVar(&flags.timeout, "timeout", 2*time.Second, "time to collect replies")
		command.Flags().IntVar(&flags.maxReplies, "max-replies", 1, "maximum number of replies to collect")
	}
}

func messageFromCLI(args []string, flags messageFlags, files FileSystem) (domain.Message, error) {
	if flags.file != "" && len(args) == 2 {
		return domain.Message{}, errors.New("payload argument and --file are mutually exclusive")
	}
	payload := []byte(nil)
	if len(args) == 2 {
		payload = []byte(args[1])
	}
	if flags.file != "" {
		var err error
		payload, err = readPayloadFile(files, flags.file)
		if err != nil {
			return domain.Message{}, err
		}
	}
	headers, err := parseHeaders(flags.headers)
	if err != nil {
		return domain.Message{}, fmt.Errorf("header: %w", err)
	}
	return domain.NewMessage(args[0], payload, headers)
}

func readPayloadFile(files FileSystem, path string) ([]byte, error) {
	payload, err := files.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read payload file %q: %w", path, err)
	}
	return payload, nil
}

func requestFromCLI(args []string, flags messageFlags, files FileSystem) (domain.Request, error) {
	message, err := messageFromCLI(args, flags, files)
	if err != nil {
		return domain.Request{}, err
	}
	return domain.NewRequest(message, flags.timeout, flags.maxReplies)
}

func writeExchange(cmd *cobra.Command, exchange domain.Exchange) error {
	result := struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		Replies []struct {
			Latency string              `json:"latency"`
			Headers map[string][]string `json:"headers,omitempty"`
			Payload json.RawMessage     `json:"payload"`
		} `json:"replies"`
	}{ID: exchange.ID(), Subject: exchange.Request().Message().Subject().String()}
	for _, reply := range exchange.Replies() {
		payload := json.RawMessage(reply.Payload())
		if !json.Valid(payload) {
			encoded, _ := json.Marshal(string(reply.Payload()))
			payload = encoded
		}
		result.Replies = append(result.Replies, struct {
			Latency string              `json:"latency"`
			Headers map[string][]string `json:"headers,omitempty"`
			Payload json.RawMessage     `json:"payload"`
		}{Latency: reply.Latency().String(), Headers: reply.Headers(), Payload: payload})
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func newRequestHistoryCommand(values *viper.Viper, services requestServices) *cobra.Command {
	return &cobra.Command{Use: "history", Short: "List saved request history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if services.history == nil {
			return errors.New("request history is not configured")
		}
		history, err := services.history.ListRequestHistory(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		for _, exchange := range history {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%d replies\n", exchange.ID(), exchange.StartedAt().Format(time.RFC3339), exchange.Request().Message().Subject(), len(exchange.Replies()))
			if err != nil {
				return err
			}
		}
		return nil
	}}
}

func newRequestReplayCommand(values *viper.Viper, services requestServices) *cobra.Command {
	return &cobra.Command{Use: "replay ID", Short: "Replay a saved request", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if services.history == nil {
			return errors.New("request history is not configured")
		}
		exchange, err := services.history.ReplayCoreRequest(cmd.Context(), config.Load(values), args[0])
		if err != nil {
			return err
		}
		return writeExchange(cmd, exchange)
	}}
}

func newRequestTemplateCommand(values *viper.Viper, services requestServices, files FileSystem) *cobra.Command {
	command := &cobra.Command{Use: "template", Short: "Manage reusable request templates", Args: cobra.NoArgs}
	command.AddCommand(newSaveRequestTemplateCommand(values, services, files))
	command.AddCommand(newListRequestTemplatesCommand(values, services))
	command.AddCommand(newRunRequestTemplateCommand(values, services))
	command.AddCommand(newDeleteRequestTemplateCommand(values, services))
	return command
}

func newSaveRequestTemplateCommand(values *viper.Viper, services requestServices, files FileSystem) *cobra.Command {
	var flags messageFlags
	command := &cobra.Command{Use: "save NAME SUBJECT [PAYLOAD]", Short: "Save or replace a request template", Args: cobra.RangeArgs(2, 3), RunE: func(cmd *cobra.Command, args []string) error {
		if services.templates == nil {
			return errors.New("request templates are not configured")
		}
		request, err := requestFromCLI(args[1:], flags, files)
		if err != nil {
			return err
		}
		template, err := domain.NewTemplate(args[0], request)
		if err != nil {
			return err
		}
		if err := services.templates.SaveRequestTemplate(cmd.Context(), config.Load(values), template); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "saved request template %s\n", template.Name())
		return err
	}}
	bindMessageFlags(command, &flags, true)
	return command
}

func newListRequestTemplatesCommand(values *viper.Viper, services requestServices) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List request templates", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if services.templates == nil {
			return errors.New("request templates are not configured")
		}
		templates, err := services.templates.ListRequestTemplates(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		sort.Slice(templates, func(i, j int) bool { return templates[i].Name() < templates[j].Name() })
		for _, template := range templates {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", template.Name(), template.Request().Message().Subject())
			if err != nil {
				return err
			}
		}
		return nil
	}}
}

func newRunRequestTemplateCommand(values *viper.Viper, services requestServices) *cobra.Command {
	return &cobra.Command{Use: "run NAME", Short: "Run a saved request template", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if services.templates == nil || services.requester == nil {
			return errors.New("request template execution is not configured")
		}
		template, err := services.templates.GetRequestTemplate(cmd.Context(), config.Load(values), args[0])
		if err != nil {
			return err
		}
		exchange, err := services.requester.SendCoreRequest(cmd.Context(), config.Load(values), template.Request())
		if err != nil {
			return err
		}
		return writeExchange(cmd, exchange)
	}}
}

func newDeleteRequestTemplateCommand(values *viper.Viper, services requestServices) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete NAME", Short: "Delete a request template", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("pass --yes to confirm template deletion")
		}
		if services.templates == nil {
			return errors.New("request templates are not configured")
		}
		if err := services.templates.DeleteRequestTemplate(cmd.Context(), config.Load(values), args[0]); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted request template %s\n", args[0])
		return err
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm deletion")
	return command
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\r\n'\"$`\\!&|;()<>*?[]{}") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func CLIEquivalent(request domain.Request) string {
	message := request.Message()
	parts := []string{"nats-tui", "request", shellQuote(message.Subject().String()), shellQuote(string(message.Payload()))}
	names := make([]string, 0, len(message.Headers()))
	for name := range message.Headers() {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range message.Headers()[name] {
			parts = append(parts, "-H", shellQuote(name+"="+value))
		}
	}
	parts = append(parts, "--timeout", request.Timeout().String(), "--max-replies", fmt.Sprintf("%d", request.MaxReplies()))
	return strings.Join(parts, " ")
}
