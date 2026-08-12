package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fd1az/nats-tui/internal/app/objectstore"
	"github.com/fd1az/nats-tui/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type objectServices struct {
	transfer      ObjectTransfer
	editor        ObjectEditor
	deleter       ObjectDeleter
	linkCreator   ObjectLinkCreator
	bucketCreator ObjectBucketCreator
	bucketUpdater ObjectBucketUpdater
	bucketDeleter ObjectBucketDeleter
}

func newObjectCommand(values *viper.Viper, services objectServices, clock Clock) *cobra.Command {
	command := &cobra.Command{Use: "object", Short: "Transfer JetStream Object Store objects", Args: cobra.NoArgs}
	command.AddCommand(newObjectPutCommand(values, services.transfer, clock))
	command.AddCommand(newObjectGetCommand(values, services.transfer, clock))
	command.AddCommand(newObjectEditCommand(values, services.editor))
	command.AddCommand(newObjectDeleteCommand(values, services.deleter))
	command.AddCommand(newObjectLinkCommand(values, services.linkCreator))
	command.AddCommand(newObjectBucketCommand(values, services))
	return command
}

func newObjectLinkCommand(values *viper.Viper, creator ObjectLinkCreator) *cobra.Command {
	return &cobra.Command{
		Use: "link DESTINATION_BUCKET LINK_NAME TARGET_BUCKET [TARGET_OBJECT]", Short: "Create an object or bucket link", Args: cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			if creator == nil {
				return errors.New("object link creator service is not configured")
			}
			definition := objectstore.LinkDefinition{Bucket: args[0], Name: args[1], TargetBucket: args[2]}
			if len(args) == 4 {
				definition.TargetObject = args[3]
			}
			result, err := creator.CreateObjectLink(cmd.Context(), config.Load(values), definition)
			if err != nil {
				return err
			}
			kind := "bucket"
			if !definition.IsBucketLink() {
				kind = "object"
			}
			target := definition.TargetBucket
			if definition.TargetObject != "" {
				target += "/" + definition.TargetObject
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created %s link %s/%s -> %s\n", kind, definition.Bucket, result.Name, target)
			return err
		},
	}
}

func newObjectEditCommand(values *viper.Viper, editor ObjectEditor) *cobra.Command {
	var newName, description string
	var metadata, headers []string
	var clearMetadata, clearHeaders bool
	command := &cobra.Command{
		Use: "edit BUCKET NAME", Short: "Rename an object or update its metadata", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if editor == nil {
				return errors.New("object editor service is not configured")
			}
			patch := objectstore.EditObject{Bucket: args[0], Name: args[1]}
			if cmd.Flags().Changed("name") {
				patch.NewName = &newName
			}
			if cmd.Flags().Changed("description") {
				patch.Description = &description
			}
			if clearMetadata && cmd.Flags().Changed("metadata") {
				return errors.New("--clear-metadata and --metadata are mutually exclusive")
			}
			if clearHeaders && cmd.Flags().Changed("header") {
				return errors.New("--clear-headers and --header are mutually exclusive")
			}
			if clearMetadata || cmd.Flags().Changed("metadata") {
				parsed, err := parseProperties(metadata)
				if err != nil {
					return fmt.Errorf("metadata: %w", err)
				}
				if clearMetadata {
					parsed = map[string]string{}
				}
				patch.Metadata = &parsed
			}
			if clearHeaders || cmd.Flags().Changed("header") {
				parsed, err := parseHeaders(headers)
				if err != nil {
					return fmt.Errorf("header: %w", err)
				}
				if clearHeaders {
					parsed = map[string][]string{}
				}
				patch.Headers = &parsed
			}
			if patch.NewName == nil && patch.Description == nil && patch.Metadata == nil && patch.Headers == nil {
				return errors.New("at least one change is required")
			}
			result, err := editor.EditObject(cmd.Context(), config.Load(values), patch)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated object %s/%s\n", args[0], result.Name)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&newName, "name", "", "new object name")
	flags.StringVar(&description, "description", "", "new description; empty clears it")
	flags.StringSliceVar(&metadata, "metadata", nil, "replacement metadata key=value")
	flags.StringSliceVar(&headers, "header", nil, "replacement header name=value")
	flags.BoolVar(&clearMetadata, "clear-metadata", false, "remove all metadata")
	flags.BoolVar(&clearHeaders, "clear-headers", false, "remove all headers")
	return command
}

func newObjectDeleteCommand(values *viper.Viper, deleter ObjectDeleter) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use: "delete BUCKET NAME", Short: "Permanently delete an object", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return errors.New("object deletion is destructive; pass --yes to confirm")
			}
			if deleter == nil {
				return errors.New("object deleter service is not configured")
			}
			if err := deleter.DeleteObject(cmd.Context(), config.Load(values), args[0], args[1]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted object %s/%s\n", args[0], args[1])
			return err
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}

func newObjectPutCommand(values *viper.Viper, transfer ObjectTransfer, clock Clock) *cobra.Command {
	var name, description string
	var metadata, headers []string
	command := &cobra.Command{
		Use: "put BUCKET FILE", Short: "Upload a local file to an Object Store", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if transfer == nil {
				return errors.New("object transfer service is not configured")
			}
			parsedMetadata, err := parseProperties(metadata)
			if err != nil {
				return fmt.Errorf("metadata: %w", err)
			}
			parsedHeaders, err := parseHeaders(headers)
			if err != nil {
				return fmt.Errorf("header: %w", err)
			}
			reporter := newProgressReporter(cmd.ErrOrStderr(), clock)
			result, err := transfer.UploadObjectFile(cmd.Context(), config.Load(values), objectstore.UploadFile{
				Bucket: args[0], Path: args[1], Name: name, Description: description, Metadata: parsedMetadata, Headers: parsedHeaders,
			}, reporter.Report)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "uploaded %s/%s (%d bytes, %s)\n", args[0], result.Name, result.Size, result.Digest)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&name, "name", "", "object name; defaults to the file basename")
	flags.StringVar(&description, "description", "", "object description")
	flags.StringSliceVar(&metadata, "metadata", nil, "metadata key=value; repeat for multiple values")
	flags.StringSliceVar(&headers, "header", nil, "header name=value; repeat for multiple values")
	return command
}

func newObjectGetCommand(values *viper.Viper, transfer ObjectTransfer, clock Clock) *cobra.Command {
	var output string
	var force bool
	command := &cobra.Command{
		Use: "get BUCKET NAME", Short: "Download and verify an Object Store object", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if transfer == nil {
				return errors.New("object transfer service is not configured")
			}
			reporter := newProgressReporter(cmd.ErrOrStderr(), clock)
			result, err := transfer.DownloadObjectFile(cmd.Context(), config.Load(values), objectstore.DownloadFile{
				Bucket: args[0], Name: args[1], Path: output, Force: force,
			}, reporter.Report)
			if err != nil {
				return err
			}
			verified := "unverified"
			if result.Verified {
				verified = "verified"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "downloaded %s/%s to %s (%s)\n", args[0], args[1], output, verified)
			return err
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "destination file")
	command.Flags().BoolVar(&force, "force", false, "replace an existing destination after verification")
	_ = command.MarkFlagRequired("output")
	return command
}

func parseProperties(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		name, content, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("expected key=value, got %q", value)
		}
		result[name] = content
	}
	return result, nil
}

func parseHeaders(values []string) (map[string][]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string][]string, len(values))
	for _, value := range values {
		name, content, ok := strings.Cut(value, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("expected name=value, got %q", value)
		}
		result[name] = append(result[name], content)
	}
	return result, nil
}

type progressReporter struct {
	output io.Writer
	last   time.Time
	clock  Clock
}

func newProgressReporter(output io.Writer, clock Clock) *progressReporter {
	return &progressReporter{output: output, clock: clock}
}

func (r *progressReporter) Report(progress objectstore.Progress) {
	now := r.clock.Now()
	if progress.Total > 0 && progress.Transferred < progress.Total && now.Sub(r.last) < 100*time.Millisecond {
		return
	}
	r.last = now
	_, _ = fmt.Fprintf(r.output, "%d/%d bytes\n", progress.Transferred, progress.Total)
}
