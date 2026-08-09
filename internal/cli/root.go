package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/config"
	domainrequestreply "github.com/clcsolutions/nats-tui/internal/domain/requestreply"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Runner interface {
	Run(context.Context, config.Settings) error
}

type KeyValueCreator interface {
	CreateKeyValueBucket(context.Context, config.Settings, kvstore.Definition) error
}

type KeyValueUpdater interface {
	UpdateKeyValueBucket(context.Context, config.Settings, string, kvstore.Changes) error
}

type KeyValueDeleter interface {
	DeleteKeyValueBucket(context.Context, config.Settings, string) error
}

type KeyValueKeyLister interface {
	ListKeyValueKeys(context.Context, config.Settings, string) ([]string, error)
}

type KeyValueEntryReader interface {
	GetKeyValueEntry(context.Context, config.Settings, string, string) (kvstore.Entry, error)
}

type KeyValueEntryWriter interface {
	PutKeyValueEntry(context.Context, config.Settings, kvstore.WriteEntry) (uint64, error)
}

type KeyValueEntryRemover interface {
	RemoveKeyValueEntry(context.Context, config.Settings, kvstore.RemoveEntry) error
}

type KeyValueHistoryReader interface {
	ListKeyValueHistory(context.Context, config.Settings, string, string) ([]kvstore.Entry, error)
}

type KeyValueRestorer interface {
	RestoreKeyValueEntry(context.Context, config.Settings, kvstore.RestoreEntry) (kvstore.RestoredEntry, error)
}

type KeyValueWatcher interface {
	WatchKeyValue(context.Context, config.Settings, string, string) (<-chan kvstore.Entry, error)
}

type ObjectTransfer interface {
	UploadObjectFile(context.Context, config.Settings, objectstore.UploadFile, objectstore.ProgressFunc) (objectstore.ObjectSummary, error)
	DownloadObjectFile(context.Context, config.Settings, objectstore.DownloadFile, objectstore.ProgressFunc) (objectstore.DownloadResult, error)
}

type ObjectEditor interface {
	EditObject(context.Context, config.Settings, objectstore.EditObject) (objectstore.ObjectSummary, error)
}

type ObjectDeleter interface {
	DeleteObject(context.Context, config.Settings, string, string) error
}

type ObjectLinkCreator interface {
	CreateObjectLink(context.Context, config.Settings, objectstore.LinkDefinition) (objectstore.ObjectSummary, error)
}

type ObjectBucketCreator interface {
	CreateObjectStoreBucket(context.Context, config.Settings, objectstore.Definition) error
}

type ObjectBucketUpdater interface {
	UpdateObjectStoreBucket(context.Context, config.Settings, string, objectstore.Changes) error
}

type ObjectBucketDeleter interface {
	DeleteObjectStoreBucket(context.Context, config.Settings, string) error
}

type CorePublisher interface {
	PublishCoreMessage(context.Context, config.Settings, domainrequestreply.Message) error
}

type CoreRequester interface {
	SendCoreRequest(context.Context, config.Settings, domainrequestreply.Request) (domainrequestreply.Exchange, error)
}

type RequestHistory interface {
	ListRequestHistory(context.Context) ([]domainrequestreply.Exchange, error)
	ReplayCoreRequest(context.Context, config.Settings, string) (domainrequestreply.Exchange, error)
}

type RequestTemplates interface {
	SaveRequestTemplate(context.Context, domainrequestreply.Template) error
	ListRequestTemplates(context.Context) ([]domainrequestreply.Template, error)
	GetRequestTemplate(context.Context, string) (domainrequestreply.Template, error)
	DeleteRequestTemplate(context.Context, string) error
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Clock interface{ Now() time.Time }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type FileSystem interface {
	ReadFile(string) ([]byte, error)
	OpenFile(string, int, os.FileMode) (io.WriteCloser, error)
}

type osFileSystem struct{}

func (osFileSystem) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (osFileSystem) OpenFile(name string, flags int, mode os.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(name, flags, mode)
}

type Dependencies struct {
	Runner                 Runner
	KeyValueCreator        KeyValueCreator
	KeyValueUpdater        KeyValueUpdater
	KeyValueDeleter        KeyValueDeleter
	KeyValueKeyLister      KeyValueKeyLister
	KeyValueEntryReader    KeyValueEntryReader
	KeyValueEntryWriter    KeyValueEntryWriter
	KeyValueEntryRemover   KeyValueEntryRemover
	KeyValueHistoryReader  KeyValueHistoryReader
	KeyValueRestorer       KeyValueRestorer
	KeyValueWatcher        KeyValueWatcher
	ObjectTransfer         ObjectTransfer
	ObjectEditor           ObjectEditor
	ObjectDeleter          ObjectDeleter
	ObjectLinkCreator      ObjectLinkCreator
	ObjectBucketCreator    ObjectBucketCreator
	ObjectBucketUpdater    ObjectBucketUpdater
	ObjectBucketDeleter    ObjectBucketDeleter
	CorePublisher          CorePublisher
	CoreRequester          CoreRequester
	RequestHistory         RequestHistory
	RequestTemplates       RequestTemplates
	TopologyReader         TopologyReader
	TopologyOperator       TopologyOperator
	ConnectionReader       ConnectionReader
	ConnectionCloser       ConnectionCloser
	OperatorReader         OperatorReader
	OperatorAccountCreator OperatorAccountCreator
	OperatorUserCreator    OperatorUserCreator
	OperatorMutator        OperatorMutator
	Clock                  Clock
	FileSystem             FileSystem
	Version                BuildInfo
}

func NewRoot(deps Dependencies) *cobra.Command {
	values := viper.New()
	var configFile string

	root := &cobra.Command{
		Use:           "nats-tui",
		Short:         "A beautiful terminal workspace for NATS JetStream",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return config.Configure(values, configFile)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.Runner == nil {
				return errors.New("TUI runner is not configured")
			}
			return deps.Runner.Run(cmd.Context(), config.Load(values))
		},
	}

	flags := root.PersistentFlags()
	flags.StringVar(&configFile, "config", "", "config file")
	flags.String("profile", "local", "connection profile name")
	flags.String("server", "nats://127.0.0.1:4222", "NATS server URL")
	flags.String("monitor-url", "", "optional NATS HTTP monitoring URL (for example http://127.0.0.1:8222)")
	flags.String("credentials", "", "NATS credentials file")
	flags.String("token", "", "NATS authentication token")
	flags.String("authority-dir", "", "private directory for locally managed account and user credentials")
	flags.String("authority-operator-seed", "", "operator seed file enabling account mutations")
	flags.Duration("refresh", 2*time.Second, "dashboard refresh interval")

	for _, name := range []string{"profile", "server", "monitor-url", "credentials", "token", "authority-dir", "authority-operator-seed", "refresh"} {
		if err := values.BindPFlag(name, flags.Lookup(name)); err != nil {
			panic(fmt.Sprintf("bind flag %s: %v", name, err))
		}
	}

	root.AddCommand(newVersionCommand(deps.Version))
	root.AddCommand(newCompletionCommand(root))
	files := deps.FileSystem
	if files == nil {
		files = osFileSystem{}
	}
	root.AddCommand(newKeyValueCommand(values, deps.KeyValueCreator, deps.KeyValueUpdater, deps.KeyValueDeleter, deps.KeyValueKeyLister, deps.KeyValueEntryReader, deps.KeyValueEntryWriter, deps.KeyValueEntryRemover, deps.KeyValueHistoryReader, deps.KeyValueRestorer, deps.KeyValueWatcher, files))
	clock := deps.Clock
	if clock == nil {
		clock = systemClock{}
	}
	root.AddCommand(newObjectCommand(values, objectServices{
		transfer: deps.ObjectTransfer, editor: deps.ObjectEditor, deleter: deps.ObjectDeleter, linkCreator: deps.ObjectLinkCreator,
		bucketCreator: deps.ObjectBucketCreator, bucketUpdater: deps.ObjectBucketUpdater, bucketDeleter: deps.ObjectBucketDeleter,
	}, clock))
	root.AddCommand(newPublishCommand(values, deps.CorePublisher, files))
	root.AddCommand(newRequestCommand(values, requestServices{requester: deps.CoreRequester, history: deps.RequestHistory, templates: deps.RequestTemplates}, files))
	root.AddCommand(newTopologyCommand(values, deps.TopologyReader, deps.TopologyOperator))
	root.AddCommand(newConnectionsCommand(values, deps.ConnectionReader, deps.ConnectionCloser))
	root.AddCommand(newAccountsCommand(values, deps.OperatorReader, deps.OperatorAccountCreator, deps.OperatorUserCreator, deps.OperatorMutator))
	return root
}

func newVersionCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(
				cmd.OutOrStdout(),
				"nats-tui %s (commit: %s, built: %s)\n",
				valueOr(info.Version, "dev"),
				valueOr(info.Commit, "unknown"),
				valueOr(info.Date, "unknown"),
			)
			return err
		},
	}
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return root.GenPowerShellCompletion(cmd.OutOrStdout())
			}
		},
	}
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
