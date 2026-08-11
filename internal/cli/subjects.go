package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/clcsolutions/nats-tui/internal/app/subjects"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newSubjectCommand(values *viper.Viper, explorer SubjectExplorer, publisher CorePublisher, requester CoreRequester, files FileSystem) *cobra.Command {
	command := &cobra.Command{Use: "subject", Aliases: []string{"subjects", "sub"}, Short: "Explore subject ownership, consumers, and permissions", Args: cobra.NoArgs}
	command.AddCommand(
		newSubjectListCommand(values, explorer),
		newSubjectInspectCommand(values, explorer),
		newSubjectPublishCommand(values, publisher, files),
		newSubjectRequestCommand(values, requester, files),
	)
	return command
}

func newSubjectListCommand(values *viper.Viper, explorer SubjectExplorer) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List the enriched configured subject tree", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if explorer == nil {
			return errors.New("subject explorer is not configured")
		}
		nodes, err := explorer.ExploreSubjects(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		return writeSubjectNodes(cmd, nodes, output)
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "output: table, json, or plain")
	return command
}

func newSubjectInspectCommand(values *viper.Viper, explorer SubjectExplorer) *cobra.Command {
	return &cobra.Command{Use: "inspect SUBJECT", Aliases: []string{"get"}, Short: "Inspect one subject branch or configured pattern", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if explorer == nil {
			return errors.New("subject explorer is not configured")
		}
		nodes, err := explorer.ExploreSubjects(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		for _, node := range nodes {
			if node.Subject == args[0] {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(node)
			}
		}
		return fmt.Errorf("subject %q was not found", args[0])
	}}
}

func newSubjectPublishCommand(values *viper.Viper, publisher CorePublisher, files FileSystem) *cobra.Command {
	var flags messageFlags
	command := &cobra.Command{Use: "publish SUBJECT [PAYLOAD]", Short: "Publish directly from the Subject Explorer workflow", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
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
	}}
	bindMessageFlags(command, &flags, false)
	return command
}

func newSubjectRequestCommand(values *viper.Viper, requester CoreRequester, files FileSystem) *cobra.Command {
	var flags messageFlags
	command := &cobra.Command{Use: "request SUBJECT [PAYLOAD]", Short: "Send request/reply directly from a selected subject", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		if requester == nil {
			return errors.New("Core NATS requester is not configured")
		}
		request, err := requestFromCLI(args, flags, files)
		if err != nil {
			return err
		}
		exchange, err := requester.SendCoreRequest(cmd.Context(), config.Load(values), request)
		if err != nil {
			return err
		}
		return writeExchange(cmd, exchange)
	}}
	bindMessageFlags(command, &flags, true)
	return command
}

func writeSubjectNodes(cmd *cobra.Command, nodes []subjects.Node, output string) error {
	switch strings.ToLower(output) {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(nodes)
	case "plain":
		for _, node := range nodes {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), node.Subject); err != nil {
				return err
			}
		}
		return nil
	case "table":
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "SUBJECT\tTYPE\tSTREAMS\tCONSUMERS\tPERMISSIONS"); err != nil {
			return err
		}
		for _, node := range nodes {
			kind := "branch"
			if node.Configured {
				kind = "configured"
			} else if node.Observed {
				kind = "observed"
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s%s\t%s\t%s\t%s\t%d\n", strings.Repeat("  ", node.Depth), node.Subject, kind, strings.Join(node.Streams, ","), strings.Join(node.Consumers, ","), len(node.Permissions)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported output %q; use table, json, or plain", output)
	}
}
