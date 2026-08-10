package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	app "github.com/clcsolutions/nats-tui/internal/app/bookmarks"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

type BookmarkManager interface {
	AddBookmark(context.Context, config.Settings, app.AddCommand) (app.View, error)
	ListBookmarks(context.Context, config.Settings, []app.TargetView) ([]app.View, error)
	DeleteBookmark(context.Context, config.Settings, string) error
	RetargetBookmark(context.Context, config.Settings, string, string, string) error
	SaveRecentSearch(context.Context, config.Settings, string, string) error
	ListRecentSearches(context.Context, config.Settings, string, int) ([]app.RecentSearch, error)
}

func newBookmarksCommand(values *viper.Viper, service BookmarkManager) *cobra.Command {
	var output string
	command := &cobra.Command{Use: "bookmarks", Aliases: []string{"bookmark", "bm"}, Short: "Manage profile-scoped resource bookmarks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("bookmark manager is not configured")
		}
		items, err := service.ListBookmarks(cmd.Context(), config.Load(values), nil)
		if err != nil {
			return err
		}
		if output == "json" {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		}
		if output != "table" {
			return fmt.Errorf("unsupported output %q (use table or json)", output)
		}
		for _, item := range items {
			target := strings.Trim(strings.Join([]string{item.Primary, item.Secondary}, "/"), "/")
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Kind, target, item.Label, item.Status); err != nil {
				return err
			}
		}
		return nil
	}}
	command.Flags().StringVarP(&output, "output", "o", "table", "table or json")
	command.AddCommand(newBookmarkAdd(values, service), newBookmarkDelete(values, service), newBookmarkRetarget(values, service), newRecentSearches(values, service))
	return command
}
func newBookmarkAdd(values *viper.Viper, service BookmarkManager) *cobra.Command {
	var label string
	command := &cobra.Command{Use: "add KIND PRIMARY [SECONDARY]", Short: "Bookmark a stream, consumer, subject, KV, or object resource", Args: cobra.RangeArgs(2, 3), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("bookmark manager is not configured")
		}
		secondary := ""
		if len(args) == 3 {
			secondary = args[2]
		}
		item, err := service.AddBookmark(cmd.Context(), config.Load(values), app.AddCommand{Kind: args[0], Primary: args[1], Secondary: secondary, Label: label})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), item.ID)
		return err
	}}
	command.Flags().StringVar(&label, "label", "", "optional display label")
	return command
}
func newBookmarkDelete(values *viper.Viper, service BookmarkManager) *cobra.Command {
	var yes bool
	command := &cobra.Command{Use: "delete ID", Short: "Delete a bookmark", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return errors.New("deleting a bookmark requires --yes")
		}
		if service == nil {
			return errors.New("bookmark manager is not configured")
		}
		return service.DeleteBookmark(cmd.Context(), config.Load(values), args[0])
	}}
	command.Flags().BoolVar(&yes, "yes", false, "confirm bookmark deletion")
	return command
}
func newBookmarkRetarget(values *viper.Viper, service BookmarkManager) *cobra.Command {
	return &cobra.Command{Use: "retarget ID PRIMARY [SECONDARY]", Short: "Point a stale bookmark at a renamed resource of the same kind", Args: cobra.RangeArgs(2, 3), RunE: func(cmd *cobra.Command, args []string) error {
		if service == nil {
			return errors.New("bookmark manager is not configured")
		}
		secondary := ""
		if len(args) == 3 {
			secondary = args[2]
		}
		return service.RetargetBookmark(cmd.Context(), config.Load(values), args[0], args[1], secondary)
	}}
}
func newRecentSearches(values *viper.Viper, service BookmarkManager) *cobra.Command {
	var scope string
	var limit int
	command := &cobra.Command{Use: "searches", Short: "List persistent recent searches", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if service == nil {
			return errors.New("bookmark manager is not configured")
		}
		items, err := service.ListRecentSearches(cmd.Context(), config.Load(values), scope, limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
	}}
	command.Flags().StringVar(&scope, "scope", "", "optional workspace scope")
	command.Flags().IntVarP(&limit, "limit", "n", 20, "maximum results")
	return command
}
