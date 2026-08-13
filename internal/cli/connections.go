package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	app "github.com/fd1az/nats-tui/internal/app/connections"
	"github.com/fd1az/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"strconv"
	"strings"
)

type ConnectionReader interface {
	ListNATSConnections(context.Context, config.Settings) (app.Snapshot, error)
}
type ConnectionCloser interface {
	CloseNATSConnection(context.Context, config.Settings, string, uint64) error
}

func newConnectionsCommand(values *viper.Viper, reader ConnectionReader, closer ConnectionCloser) *cobra.Command {
	var filter, sortBy, groupBy string
	var slow, jsonOutput bool
	command := &cobra.Command{Use: "connections", Aliases: []string{"conns"}, Short: "Inspect account-wide NATS connections", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !validConnectionChoice(sortBy, "name", "user", "ip", "language", "version", "rtt", "traffic", "pending") {
			return fmt.Errorf("unsupported connection sort %q", sortBy)
		}
		if !validConnectionChoice(groupBy, "none", "name", "user", "server", "language") {
			return fmt.Errorf("unsupported connection group %q", groupBy)
		}
		if reader == nil {
			return errors.New("connection reader is not configured")
		}
		snapshot, err := reader.ListNATSConnections(cmd.Context(), config.Load(values))
		if err != nil {
			return err
		}
		items := app.Query{Filter: filter, Sort: app.Sort(sortBy), OnlySlow: slow}.Apply(snapshot.Connections)
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		}
		return writeConnections(cmd.OutOrStdout(), items, app.Group(groupBy))
	}}
	flags := command.Flags()
	flags.StringVar(&filter, "filter", "", "filter by name, user, IP, language, version, or server")
	flags.StringVar(&sortBy, "sort", string(app.SortName), "sort by name, user, ip, language, version, rtt, traffic, or pending")
	flags.StringVar(&groupBy, "group", string(app.GroupNone), "group by name, user, server, or language")
	flags.BoolVar(&slow, "slow", false, "show only slow consumers")
	flags.BoolVar(&jsonOutput, "json", false, "print JSON")
	command.AddCommand(newCloseConnectionCommand(values, closer))
	return command
}
func newCloseConnectionCommand(values *viper.Viper, closer ConnectionCloser) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "close SERVER_ID CID", Short: "Close a client connection", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("closing a connection requires --yes")
		}
		if closer == nil {
			return errors.New("connection closer is not configured")
		}
		cid, err := strconv.ParseUint(args[1], 10, 64)
		if err != nil || cid == 0 {
			return errors.New("CID must be a positive integer")
		}
		return closer.CloseNATSConnection(cmd.Context(), config.Load(values), args[0], cid)
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm connection close")
	return command
}
func writeConnections(output io.Writer, items []app.Connection, group app.Group) error {
	groups := app.GroupBy(items, group)
	for _, bucket := range groups {
		if group != app.GroupNone {
			if _, err := fmt.Fprintf(output, "\n[%s] %d\n", bucket.Name, len(bucket.Connections)); err != nil {
				return err
			}
		}
		for _, item := range bucket.Connections {
			slow := ""
			if item.SlowConsumer() {
				slow = " SLOW"
			}
			if _, err := fmt.Fprintf(output, "%s:%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s%s\n", item.ServerID, item.CID, valueOr(item.Name, "(unnamed)"), valueOr(item.User, "-"), item.IP, valueOr(item.Lang, "-"), item.RTT, humanCLIBytes(nonNegativeCLIBytes(item.Traffic())), humanCLIBytes(nonNegativeCLIBytes(item.PendingBytes)), slow); err != nil {
				return err
			}
		}
	}
	return nil
}
func validConnectionChoice(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}
