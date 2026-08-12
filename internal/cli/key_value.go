package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fd1az/nats-tui/internal/app/kvstore"
	"github.com/fd1az/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newKeyValueCommand(values *viper.Viper, creator KeyValueCreator, updater KeyValueUpdater, deleter KeyValueDeleter, keyLister KeyValueKeyLister, entryReader KeyValueEntryReader, entryWriter KeyValueEntryWriter, entryRemover KeyValueEntryRemover, historyReader KeyValueHistoryReader, restorer KeyValueRestorer, watcher KeyValueWatcher, files FileSystem) *cobra.Command {
	command := &cobra.Command{Use: "kv", Short: "Manage JetStream Key-Value stores", Args: cobra.NoArgs}
	command.AddCommand(newKeyValueCreateCommand(values, creator))
	command.AddCommand(newKeyValueEditCommand(values, updater))
	command.AddCommand(newKeyValueDeleteCommand(values, deleter))
	command.AddCommand(newKeyValueKeysCommand(values, keyLister))
	command.AddCommand(newKeyValueGetCommand(values, entryReader))
	command.AddCommand(newKeyValueExportCommand(values, entryReader, files))
	command.AddCommand(newKeyValuePutCommand(values, entryWriter, files))
	command.AddCommand(newKeyValueRemoveCommand(values, entryRemover, kvstore.RemoveDelete))
	command.AddCommand(newKeyValueRemoveCommand(values, entryRemover, kvstore.RemovePurge))
	command.AddCommand(newKeyValueHistoryCommand(values, historyReader))
	command.AddCommand(newKeyValueRestoreCommand(values, restorer))
	command.AddCommand(newKeyValueWatchCommand(values, watcher))
	return command
}

func newKeyValueExportCommand(values *viper.Viper, reader KeyValueEntryReader, files FileSystem) *cobra.Command {
	var output string
	var force bool
	command := &cobra.Command{
		Use: "export BUCKET KEY", Short: "Export a Key-Value value without altering its bytes", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reader == nil {
				return errors.New("Key-Value entry reader is not configured")
			}
			entry, err := reader.GetKeyValueEntry(cmd.Context(), config.Load(values), args[0], args[1])
			if err != nil {
				return err
			}
			if output == "-" {
				_, err = cmd.OutOrStdout().Write(entry.Value)
				return err
			}
			flags := os.O_WRONLY | os.O_CREATE
			if force {
				flags |= os.O_TRUNC
			} else {
				flags |= os.O_EXCL
			}
			file, err := files.OpenFile(output, flags, 0o600)
			if err != nil {
				return fmt.Errorf("create export file %q: %w", output, err)
			}
			if _, err = file.Write(entry.Value); err != nil {
				_ = file.Close()
				return fmt.Errorf("write export file %q: %w", output, err)
			}
			if err = file.Close(); err != nil {
				return fmt.Errorf("close export file %q: %w", output, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "exported %s/%s to %s\n", args[0], args[1], output)
			return err
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "-", "destination file; - writes raw bytes to stdout")
	command.Flags().BoolVar(&force, "force", false, "overwrite an existing destination")
	return command
}

func newKeyValueWatchCommand(values *viper.Viper, watcher KeyValueWatcher) *cobra.Command {
	return &cobra.Command{
		Use: "watch BUCKET [KEY]", Short: "Watch live Key-Value mutations", Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if watcher == nil {
				return errors.New("Key-Value watch service is not configured")
			}
			key := ">"
			if len(args) == 2 {
				key = args[1]
			}
			events, err := watcher.WatchKeyValue(cmd.Context(), config.Load(values), args[0], key)
			if err != nil {
				return err
			}
			for entry := range events {
				payload := base64.StdEncoding.EncodeToString(entry.Value)
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\n", entry.Revision, entry.Operation, entry.Created.UTC().Format(time.RFC3339Nano), entry.Key, payload); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newKeyValueRestoreCommand(values *viper.Viper, restorer KeyValueRestorer) *cobra.Command {
	var revision uint64
	command := &cobra.Command{
		Use: "restore BUCKET KEY", Short: "Restore a retained PUT revision as the latest value", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if restorer == nil {
				return errors.New("Key-Value restore service is not configured")
			}
			result, err := restorer.RestoreKeyValueEntry(cmd.Context(), config.Load(values), kvstore.RestoreEntry{Bucket: args[0], Key: args[1], Revision: revision})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "restored %s/%s at revision %d\n", args[0], args[1], result.Revision)
			return err
		},
	}
	command.Flags().Uint64Var(&revision, "revision", 0, "retained PUT revision to restore")
	_ = command.MarkFlagRequired("revision")
	return command
}

func newKeyValueHistoryCommand(values *viper.Viper, reader KeyValueHistoryReader) *cobra.Command {
	return &cobra.Command{
		Use: "history BUCKET KEY", Short: "List retained revisions for a Key-Value entry", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reader == nil {
				return errors.New("Key-Value history reader is not configured")
			}
			entries, err := reader.ListKeyValueHistory(cmd.Context(), config.Load(values), args[0], args[1])
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), "REVISION\tOPERATION\tCREATED\tDELTA\tSIZE"); err != nil {
				return err
			}
			for _, entry := range entries {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%d\t%d\n", entry.Revision, entry.Operation, entry.Created.UTC().Format(time.RFC3339Nano), entry.Delta, len(entry.Value)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newKeyValueRemoveCommand(values *viper.Viper, remover KeyValueEntryRemover, action kvstore.RemoveAction) *cobra.Command {
	verb := "delete"
	use := "delete-key BUCKET KEY"
	short := "Delete a key while retaining its revision history"
	if action == kvstore.RemovePurge {
		verb = "purge"
		use = "purge-key BUCKET KEY"
		short = "Permanently purge a key and its revision history"
	}
	var confirmed bool
	var expectedRevision uint64
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return fmt.Errorf("refusing to %s key without --yes", verb)
			}
			if remover == nil {
				return errors.New("Key-Value entry removal is not configured")
			}
			remove := kvstore.RemoveEntry{Bucket: args[0], Key: args[1], Action: action}
			if cmd.Flags().Changed("revision") {
				remove.ExpectedRevision = &expectedRevision
			}
			if err := remover.RemoveKeyValueEntry(cmd.Context(), config.Load(values), remove); err != nil {
				return err
			}
			past := "deleted"
			if action == kvstore.RemovePurge {
				past = "purged"
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s/%s\n", past, args[0], args[1])
			return err
		},
	}
	command.Flags().BoolVar(&confirmed, "yes", false, "confirm key removal")
	command.Flags().Uint64Var(&expectedRevision, "revision", 0, "only remove if the latest revision matches")
	return command
}

func newKeyValuePutCommand(values *viper.Viper, writer KeyValueEntryWriter, files FileSystem) *cobra.Command {
	var file string
	var expectedRevision uint64
	command := &cobra.Command{
		Use: "put BUCKET KEY [VALUE]", Short: "Create or update a Key-Value entry", Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if writer == nil {
				return errors.New("Key-Value entry writer is not configured")
			}
			value, err := keyValueInput(cmd, args, file, files)
			if err != nil {
				return err
			}
			write := kvstore.WriteEntry{Bucket: args[0], Key: args[1], Value: value}
			if cmd.Flags().Changed("revision") {
				write.ExpectedRevision = &expectedRevision
			}
			revision, err := writer.PutKeyValueEntry(cmd.Context(), config.Load(values), write)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "stored %s/%s at revision %d\n", args[0], args[1], revision)
			return err
		},
	}
	command.Flags().StringVarP(&file, "file", "f", "", "read value from file; use - for stdin")
	command.Flags().Uint64Var(&expectedRevision, "revision", 0, "only write if the latest revision matches")
	return command
}

func keyValueInput(cmd *cobra.Command, args []string, file string, files FileSystem) ([]byte, error) {
	if len(args) == 3 && file != "" {
		return nil, errors.New("VALUE and --file are mutually exclusive")
	}
	if len(args) == 3 {
		return []byte(args[2]), nil
	}
	if file == "" {
		return nil, errors.New("provide VALUE or --file")
	}
	if file == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	value, err := files.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read value file %q: %w", file, err)
	}
	return value, nil
}

func newKeyValueGetCommand(values *viper.Viper, reader KeyValueEntryReader) *cobra.Command {
	return &cobra.Command{
		Use: "get BUCKET KEY", Short: "Print the latest value of a Key-Value entry", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reader == nil {
				return errors.New("Key-Value entry reader is not configured")
			}
			entry, err := reader.GetKeyValueEntry(cmd.Context(), config.Load(values), args[0], args[1])
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(entry.Value)
			return err
		},
	}
}

func newKeyValueKeysCommand(values *viper.Viper, lister KeyValueKeyLister) *cobra.Command {
	var filter string
	command := &cobra.Command{
		Use: "keys BUCKET", Short: "List keys in a Key-Value bucket", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lister == nil {
				return errors.New("Key-Value key catalog is not configured")
			}
			keys, err := lister.ListKeyValueKeys(cmd.Context(), config.Load(values), args[0])
			if err != nil {
				return err
			}
			query := strings.ToLower(strings.TrimSpace(filter))
			for _, key := range keys {
				if query != "" && !strings.Contains(strings.ToLower(key), query) {
					continue
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), key); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&filter, "filter", "", "case-insensitive key substring")
	return command
}

func newKeyValueDeleteCommand(values *viper.Viper, deleter KeyValueDeleter) *cobra.Command {
	var confirmed bool
	command := &cobra.Command{
		Use: "delete NAME", Short: "Permanently delete a Key-Value bucket", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !confirmed {
				return errors.New("refusing to delete bucket without --yes")
			}
			if deleter == nil {
				return errors.New("Key-Value deletion service is not configured")
			}
			if err := deleter.DeleteKeyValueBucket(cmd.Context(), config.Load(values), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted KV bucket %s\n", args[0])
			return err
		},
	}
	command.Flags().BoolVar(&confirmed, "yes", false, "confirm permanent deletion")
	return command
}

func newKeyValueEditCommand(values *viper.Viper, updater KeyValueUpdater) *cobra.Command {
	var description string
	var history int
	var ttl time.Duration
	var maxBytes int64
	var maxValueSize int32
	var replicas int
	var compression bool
	command := &cobra.Command{
		Use: "edit NAME", Short: "Edit mutable Key-Value bucket settings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if updater == nil {
				return errors.New("Key-Value update service is not configured")
			}
			changes := kvstore.Changes{}
			flags := cmd.Flags()
			if flags.Changed("description") {
				changes.Description = &description
			}
			if flags.Changed("history") {
				changes.History = &history
			}
			if flags.Changed("ttl") {
				changes.TTL = &ttl
			}
			if flags.Changed("max-bytes") {
				changes.MaxBytes = &maxBytes
			}
			if flags.Changed("max-value-size") {
				changes.MaxValueSize = &maxValueSize
			}
			if flags.Changed("replicas") {
				changes.Replicas = &replicas
			}
			if flags.Changed("compression") {
				changes.Compression = &compression
			}
			if changes == (kvstore.Changes{}) {
				return errors.New("at least one setting must be provided")
			}
			if err := changes.Validate(); err != nil {
				return err
			}
			if err := updater.UpdateKeyValueBucket(cmd.Context(), config.Load(values), args[0], changes); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "updated KV bucket %s\n", args[0])
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&description, "description", "", "bucket description")
	flags.IntVar(&history, "history", 1, "revisions retained per key (1-64)")
	flags.DurationVar(&ttl, "ttl", 0, "entry TTL; 0 disables expiry")
	flags.Int64Var(&maxBytes, "max-bytes", -1, "maximum bucket bytes; -1 is unlimited")
	flags.Int32Var(&maxValueSize, "max-value-size", -1, "maximum value size; -1 is unlimited")
	flags.IntVar(&replicas, "replicas", 1, "number of replicas (1-5)")
	flags.BoolVar(&compression, "compression", false, "enable or disable storage compression")
	return command
}

func newKeyValueCreateCommand(values *viper.Viper, creator KeyValueCreator) *cobra.Command {
	var definition kvstore.Definition
	var storage string
	command := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a Key-Value bucket",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if creator == nil {
				return errors.New("Key-Value creation service is not configured")
			}
			parsedStorage, err := parseKeyValueStorage(storage)
			if err != nil {
				return err
			}
			definition.Name = args[0]
			definition.Storage = parsedStorage
			if err := definition.Validate(); err != nil {
				return err
			}
			if err := creator.CreateKeyValueBucket(cmd.Context(), config.Load(values), definition); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created KV bucket %s\n", definition.Name)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&definition.Description, "description", "", "bucket description")
	flags.StringVar(&storage, "storage", "file", "storage medium: file or memory")
	flags.IntVar(&definition.History, "history", 1, "revisions retained per key (1-64)")
	flags.DurationVar(&definition.TTL, "ttl", 0*time.Second, "entry TTL; 0 disables expiry")
	flags.Int64Var(&definition.MaxBytes, "max-bytes", -1, "maximum bucket bytes; -1 is unlimited")
	flags.Int32Var(&definition.MaxValueSize, "max-value-size", -1, "maximum value size; -1 is unlimited")
	flags.IntVar(&definition.Replicas, "replicas", 1, "number of replicas (1-5)")
	flags.BoolVar(&definition.Compression, "compression", false, "enable storage compression")
	flags.StringVar(&definition.Mirror, "mirror", "", "mirror another KV bucket (exclusive with --source)")
	flags.StringSliceVar(&definition.Sources, "source", nil, "source KV bucket; repeat or pass comma-separated names")
	return command
}

func parseKeyValueStorage(value string) (kvstore.Storage, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "file":
		return kvstore.StorageFile, nil
	case "memory":
		return kvstore.StorageMemory, nil
	default:
		return "", fmt.Errorf("storage must be file or memory")
	}
}
