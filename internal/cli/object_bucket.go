package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newObjectBucketCommand(values *viper.Viper, services objectServices) *cobra.Command {
	command := &cobra.Command{Use: "bucket", Short: "Manage Object Store buckets", Args: cobra.NoArgs}
	command.AddCommand(newObjectBucketCreateCommand(values, services.bucketCreator))
	command.AddCommand(newObjectBucketEditCommand(values, services.bucketUpdater))
	command.AddCommand(newObjectBucketDeleteCommand(values, services.bucketDeleter))
	return command
}

func newObjectBucketCreateCommand(values *viper.Viper, creator ObjectBucketCreator) *cobra.Command {
	var description, storage string
	var ttl time.Duration
	var maxBytes int64
	var replicas int
	var compression bool
	var metadata []string
	var mirror string
	var sources []string
	command := &cobra.Command{
		Use: "create NAME", Short: "Create an Object Store bucket", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if creator == nil {
				return errors.New("object-store bucket creator is not configured")
			}
			parsedStorage, err := parseObjectStorage(storage)
			if err != nil {
				return err
			}
			parsedMetadata, err := parseProperties(metadata)
			if err != nil {
				return fmt.Errorf("metadata: %w", err)
			}
			definition := objectstore.Definition{
				Name: args[0], Description: description, Storage: parsedStorage, TTL: ttl,
				MaxBytes: maxBytes, Replicas: replicas, Compression: compression, Metadata: parsedMetadata,
				Mirror: mirror, Sources: sources,
			}
			if err := definition.Validate(); err != nil {
				return err
			}
			if err := creator.CreateObjectStoreBucket(cmd.Context(), config.Load(values), definition); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created Object Store bucket %s\n", definition.Name)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&description, "description", "", "bucket description")
	flags.StringVar(&storage, "storage", "file", "storage backend: file or memory")
	flags.DurationVar(&ttl, "ttl", 0, "maximum object age; 0 disables expiration")
	flags.Int64Var(&maxBytes, "max-bytes", -1, "maximum bucket bytes; -1 is unlimited")
	flags.IntVar(&replicas, "replicas", 1, "storage replicas from 1 to 5")
	flags.BoolVar(&compression, "compression", false, "enable stream compression")
	flags.StringSliceVar(&metadata, "metadata", nil, "bucket metadata key=value")
	flags.StringVar(&mirror, "mirror", "", "mirror another Object Store bucket (exclusive with --source)")
	flags.StringSliceVar(&sources, "source", nil, "source Object Store bucket; repeat or pass comma-separated names")
	return command
}

func newObjectBucketEditCommand(values *viper.Viper, updater ObjectBucketUpdater) *cobra.Command {
	var description string
	var ttl time.Duration
	var maxBytes int64
	var replicas int
	var compression bool
	var metadata []string
	var clearMetadata bool
	command := &cobra.Command{
		Use: "edit NAME", Short: "Edit mutable Object Store bucket settings", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if updater == nil {
				return errors.New("object-store bucket updater is not configured")
			}
			changes := objectstore.Changes{}
			if cmd.Flags().Changed("description") {
				changes.Description = &description
			}
			if cmd.Flags().Changed("ttl") {
				changes.TTL = &ttl
			}
			if cmd.Flags().Changed("max-bytes") {
				changes.MaxBytes = &maxBytes
			}
			if cmd.Flags().Changed("replicas") {
				changes.Replicas = &replicas
			}
			if cmd.Flags().Changed("compression") {
				changes.Compression = &compression
			}
			if clearMetadata && cmd.Flags().Changed("metadata") {
				return errors.New("--clear-metadata and --metadata are mutually exclusive")
			}
			if clearMetadata || cmd.Flags().Changed("metadata") {
				parsed, err := parseProperties(metadata)
				if err != nil {
					return fmt.Errorf("metadata: %w", err)
				}
				if clearMetadata {
					parsed = map[string]string{}
				}
				changes.Metadata = &parsed
			}
			if changes == (objectstore.Changes{}) {
				return errors.New("at least one change is required")
			}
			if err := changes.Validate(); err != nil {
				return err
			}
			if err := updater.UpdateObjectStoreBucket(cmd.Context(), config.Load(values), args[0], changes); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "updated Object Store bucket %s\n", args[0])
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&description, "description", "", "new description; empty clears it")
	flags.DurationVar(&ttl, "ttl", 0, "maximum object age; 0 disables expiration")
	flags.Int64Var(&maxBytes, "max-bytes", -1, "maximum bucket bytes; -1 is unlimited")
	flags.IntVar(&replicas, "replicas", 1, "storage replicas from 1 to 5")
	flags.BoolVar(&compression, "compression", false, "enable or disable compression")
	flags.StringSliceVar(&metadata, "metadata", nil, "replacement metadata key=value")
	flags.BoolVar(&clearMetadata, "clear-metadata", false, "remove all bucket metadata")
	return command
}

func newObjectBucketDeleteCommand(values *viper.Viper, deleter ObjectBucketDeleter) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use: "delete NAME", Short: "Permanently delete an Object Store bucket and all objects", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("bucket deletion removes every object; pass --yes to confirm")
			}
			if deleter == nil {
				return errors.New("object-store bucket deleter is not configured")
			}
			if err := deleter.DeleteObjectStoreBucket(cmd.Context(), config.Load(values), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted Object Store bucket %s\n", args[0])
			return err
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}

func parseObjectStorage(value string) (objectstore.Storage, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(objectstore.StorageFile):
		return objectstore.StorageFile, nil
	case string(objectstore.StorageMemory):
		return objectstore.StorageMemory, nil
	default:
		return "", fmt.Errorf("storage must be file or memory")
	}
}
