package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/clcsolutions/nats-tui/internal/bootstrap"
	"github.com/clcsolutions/nats-tui/internal/cli"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := bootstrap.NewRunner()
	root := cli.NewRoot(cli.Dependencies{
		Runner:                 runner,
		KeyValueCreator:        runner,
		KeyValueUpdater:        runner,
		KeyValueDeleter:        runner,
		KeyValueKeyLister:      runner,
		KeyValueEntryReader:    runner,
		KeyValueEntryWriter:    runner,
		KeyValueEntryRemover:   runner,
		KeyValueHistoryReader:  runner,
		KeyValueRestorer:       runner,
		KeyValueWatcher:        runner,
		ObjectTransfer:         runner,
		ObjectEditor:           runner,
		ObjectDeleter:          runner,
		ObjectLinkCreator:      runner,
		ObjectBucketCreator:    runner,
		ObjectBucketUpdater:    runner,
		ObjectBucketDeleter:    runner,
		Streams:                runner,
		Messages:               runner,
		Subjects:               runner,
		Profiles:               runner,
		Consumers:              runner,
		CorePublisher:          runner,
		CoreRequester:          runner,
		RequestHistory:         runner,
		RequestTemplates:       runner,
		TopologyReader:         runner,
		TopologyOperator:       runner,
		ConnectionReader:       runner,
		ConnectionCloser:       runner,
		OperatorReader:         runner,
		OperatorAccountCreator: runner,
		OperatorUserCreator:    runner,
		OperatorMutator:        runner,
		Alerts:                 runner,
		Commands:               runner,
		Bookmarks:              runner,
		Dashboard:              runner,
		Version: cli.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		},
	})

	if err := root.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(root.ErrOrStderr(), err)
		os.Exit(1)
	}
}
