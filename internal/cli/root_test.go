package cli

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/clcsolutions/nats-tui/internal/app/kvstore"
	"github.com/clcsolutions/nats-tui/internal/app/objectstore"
	"github.com/clcsolutions/nats-tui/internal/config"
)

func TestVersionCommandWritesBuildInformation(t *testing.T) {
	t.Parallel()

	root := NewRoot(Dependencies{Version: BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-08-07"}})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetErr(output)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "nats-tui 1.2.3 (commit: abc123, built: 2026-08-07)\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestKVCreateCommandUsesSharedDefinitionAndResolvedConnection(t *testing.T) {
	t.Parallel()

	creator := &stubKeyValueCreator{}
	root := NewRoot(Dependencies{KeyValueCreator: creator})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{
		"--server", "nats://demo:4222", "kv", "create", "SESSIONS",
		"--description", "active sessions", "--storage", "memory", "--history", "5",
		"--ttl", "1h", "--max-bytes", "2048", "--max-value-size", "512", "--replicas", "3", "--compression", "--mirror", "PRIMARY",
	})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if creator.settings.Server != "nats://demo:4222" {
		t.Fatalf("server = %q", creator.settings.Server)
	}
	want := kvstore.Definition{
		Name: "SESSIONS", Description: "active sessions", Storage: kvstore.StorageMemory,
		History: 5, TTL: time.Hour, MaxBytes: 2048, MaxValueSize: 512, Replicas: 3, Compression: true, Mirror: "PRIMARY",
	}
	if !reflect.DeepEqual(creator.definition, want) {
		t.Fatalf("definition = %#v, want %#v", creator.definition, want)
	}
	if output.String() != "created KV bucket SESSIONS\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestKVCreateRejectsInvalidStorageBeforeCallingService(t *testing.T) {
	t.Parallel()

	creator := &stubKeyValueCreator{}
	root := NewRoot(Dependencies{KeyValueCreator: creator})
	root.SetArgs([]string{"kv", "create", "SESSIONS", "--storage", "disk"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("invalid storage was accepted")
	}
	if creator.called {
		t.Fatal("creator called for invalid storage")
	}
}

func TestKVEditCommandSendsOnlyChangedFlags(t *testing.T) {
	t.Parallel()

	updater := &stubKeyValueUpdater{}
	root := NewRoot(Dependencies{KeyValueUpdater: updater})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"--server", "nats://demo:4222", "kv", "edit", "SESSIONS", "--history", "7", "--compression=false"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if updater.name != "SESSIONS" || updater.changes.History == nil || *updater.changes.History != 7 || updater.changes.Compression == nil || *updater.changes.Compression {
		t.Fatalf("update = %q %#v", updater.name, updater.changes)
	}
	if updater.changes.Description != nil || updater.changes.TTL != nil || updater.changes.Replicas != nil {
		t.Fatalf("unchanged flags leaked into patch: %#v", updater.changes)
	}
	if output.String() != "updated KV bucket SESSIONS\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestKVDeleteRequiresYesAndThenCallsService(t *testing.T) {
	t.Parallel()

	deleter := &stubKeyValueDeleter{}
	unsafe := NewRoot(Dependencies{KeyValueDeleter: deleter})
	unsafe.SetArgs([]string{"kv", "delete", "SESSIONS"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil {
		t.Fatal("delete without --yes succeeded")
	}
	if deleter.name != "" {
		t.Fatal("deleter called without confirmation")
	}

	confirmed := NewRoot(Dependencies{KeyValueDeleter: deleter})
	output := new(bytes.Buffer)
	confirmed.SetOut(output)
	confirmed.SetArgs([]string{"kv", "delete", "SESSIONS", "--yes"})
	if err := confirmed.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("confirmed delete error = %v", err)
	}
	if deleter.name != "SESSIONS" || output.String() != "deleted KV bucket SESSIONS\n" {
		t.Fatalf("delete name=%q output=%q", deleter.name, output.String())
	}
}

func TestKVKeysPrintsFilterablePlainOutput(t *testing.T) {
	t.Parallel()

	lister := &stubKeyValueKeyLister{keys: []string{"session.alpha", "session.beta", "tenant.config"}}
	root := NewRoot(Dependencies{KeyValueKeyLister: lister})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "keys", "SESSIONS", "--filter", "BETA"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("keys error = %v", err)
	}
	if lister.bucket != "SESSIONS" || output.String() != "session.beta\n" {
		t.Fatalf("bucket=%q output=%q", lister.bucket, output.String())
	}
}

func TestKVGetWritesExactValueForPipes(t *testing.T) {
	t.Parallel()

	reader := &stubKeyValueEntryReader{entry: kvstore.Entry{Value: []byte("binary\x00payload")}}
	root := NewRoot(Dependencies{KeyValueEntryReader: reader})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "get", "SESSIONS", "session.42"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("get error = %v", err)
	}
	if reader.bucket != "SESSIONS" || reader.key != "session.42" || output.String() != "binary\x00payload" {
		t.Fatalf("bucket=%q key=%q output=%q", reader.bucket, reader.key, output.String())
	}
}

func TestKVPutAcceptsLiteralValueAndExpectedRevision(t *testing.T) {
	t.Parallel()

	writer := &stubKeyValueEntryWriter{revision: 8}
	root := NewRoot(Dependencies{KeyValueEntryWriter: writer})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "put", "CONFIG", "feature.api", `{"enabled":true}`, "--revision", "7"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("put error = %v", err)
	}
	if writer.command.Bucket != "CONFIG" || writer.command.Key != "feature.api" || string(writer.command.Value) != `{"enabled":true}` || writer.command.ExpectedRevision == nil || *writer.command.ExpectedRevision != 7 || output.String() != "stored CONFIG/feature.api at revision 8\n" {
		t.Fatalf("command=%#v output=%q", writer.command, output.String())
	}
}

func TestKVPutReadsExactBytesFromStdin(t *testing.T) {
	t.Parallel()

	writer := &stubKeyValueEntryWriter{revision: 1}
	root := NewRoot(Dependencies{KeyValueEntryWriter: writer})
	root.SetIn(bytes.NewBufferString("binary\x00payload\n"))
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"kv", "put", "FILES", "artifact", "--file", "-"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("put stdin error = %v", err)
	}
	if string(writer.command.Value) != "binary\x00payload\n" {
		t.Fatalf("value = %q", writer.command.Value)
	}
}

func TestKVPutRejectsLiteralAndFileTogether(t *testing.T) {
	t.Parallel()

	root := NewRoot(Dependencies{KeyValueEntryWriter: &stubKeyValueEntryWriter{}})
	root.SetArgs([]string{"kv", "put", "CONFIG", "key", "value", "--file", "value.json"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("put accepted literal and file")
	}
}

func TestKVDeleteKeyRequiresConfirmationAndSupportsCAS(t *testing.T) {
	t.Parallel()

	remover := &stubKeyValueEntryRemover{}
	unconfirmed := NewRoot(Dependencies{KeyValueEntryRemover: remover})
	unconfirmed.SetArgs([]string{"kv", "delete-key", "CONFIG", "feature.api"})
	if err := unconfirmed.ExecuteContext(context.Background()); err == nil || remover.called {
		t.Fatal("delete-key ran without --yes")
	}

	root := NewRoot(Dependencies{KeyValueEntryRemover: remover})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "delete-key", "CONFIG", "feature.api", "--revision", "9", "--yes"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("delete-key error = %v", err)
	}
	if remover.command.Action != kvstore.RemoveDelete || remover.command.ExpectedRevision == nil || *remover.command.ExpectedRevision != 9 || output.String() != "deleted CONFIG/feature.api\n" {
		t.Fatalf("command=%#v output=%q", remover.command, output.String())
	}
}

func TestKVPurgeKeyUsesDestructiveAction(t *testing.T) {
	t.Parallel()

	remover := &stubKeyValueEntryRemover{}
	root := NewRoot(Dependencies{KeyValueEntryRemover: remover})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "purge-key", "CONFIG", "feature.api", "--yes"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("purge-key error = %v", err)
	}
	if remover.command.Action != kvstore.RemovePurge || output.String() != "purged CONFIG/feature.api\n" {
		t.Fatalf("command=%#v output=%q", remover.command, output.String())
	}
}

func TestKVHistoryPrintsStableTSVTimeline(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 7, 18, 30, 0, 0, time.UTC)
	reader := &stubKeyValueHistoryReader{entries: []kvstore.Entry{
		{Revision: 7, Operation: kvstore.OperationPut, Created: created, Delta: 1, Value: []byte(`{"enabled":true}`)},
		{Revision: 8, Operation: kvstore.OperationDelete, Created: created.Add(time.Minute), Delta: 0},
	}}
	root := NewRoot(Dependencies{KeyValueHistoryReader: reader})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "history", "CONFIG", "feature.api"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("history error = %v", err)
	}
	want := "REVISION\tOPERATION\tCREATED\tDELTA\tSIZE\n7\tPUT\t2026-08-07T18:30:00Z\t1\t16\n8\tDELETE\t2026-08-07T18:31:00Z\t0\t0\n"
	if reader.bucket != "CONFIG" || reader.key != "feature.api" || output.String() != want {
		t.Fatalf("reader=(%q,%q) output=%q", reader.bucket, reader.key, output.String())
	}
}

func TestKVRestoreRequiresSourceRevision(t *testing.T) {
	t.Parallel()

	restorer := &stubKeyValueRestorer{result: kvstore.RestoredEntry{Revision: 9}}
	root := NewRoot(Dependencies{KeyValueRestorer: restorer})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "restore", "CONFIG", "feature.api", "--revision", "7"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("restore error = %v", err)
	}
	if restorer.command.Revision != 7 || output.String() != "restored CONFIG/feature.api at revision 9\n" {
		t.Fatalf("command=%#v output=%q", restorer.command, output.String())
	}
}

func TestKVExportPreservesRawBytesAndRefusesOverwrite(t *testing.T) {
	t.Parallel()

	reader := &stubKeyValueEntryReader{entry: kvstore.Entry{Value: []byte("binary\x00payload")}}
	destination := t.TempDir() + "/value.bin"
	root := NewRoot(Dependencies{KeyValueEntryReader: reader})
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"kv", "export", "FILES", "artifact", "--output", destination})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("export error = %v", err)
	}
	value, err := os.ReadFile(destination)
	if err != nil || string(value) != "binary\x00payload" {
		t.Fatalf("exported value=%q error=%v", value, err)
	}
	overwrite := NewRoot(Dependencies{KeyValueEntryReader: reader})
	overwrite.SetArgs([]string{"kv", "export", "FILES", "artifact", "--output", destination})
	if err := overwrite.ExecuteContext(context.Background()); err == nil {
		t.Fatal("export overwrote an existing destination without --force")
	}
}

func TestKVWatchPrintsStableBase64TSV(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 8, 7, 18, 30, 0, 123, time.UTC)
	events := make(chan kvstore.Entry, 1)
	events <- kvstore.Entry{Key: "feature.api", Revision: 8, Operation: kvstore.OperationPut, Created: created, Value: []byte("binary\x00")}
	close(events)
	watcher := &stubKeyValueWatcher{events: events}
	root := NewRoot(Dependencies{KeyValueWatcher: watcher})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"kv", "watch", "CONFIG", "feature.api"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("watch error = %v", err)
	}
	want := "8\tPUT\t2026-08-07T18:30:00.000000123Z\tfeature.api\tYmluYXJ5AA==\n"
	if watcher.bucket != "CONFIG" || watcher.key != "feature.api" || output.String() != want {
		t.Fatalf("watch=(%q,%q) output=%q", watcher.bucket, watcher.key, output.String())
	}
}

func TestObjectPutPassesFileMetadataAndKeepsProgressOnStderr(t *testing.T) {
	t.Parallel()
	transfer := &stubObjectTransfer{uploadResult: objectstore.ObjectSummary{Name: "release.tar", Size: 7, Digest: "SHA-256=abc"}}
	root := NewRoot(Dependencies{ObjectTransfer: transfer})
	output, diagnostics := new(bytes.Buffer), new(bytes.Buffer)
	root.SetOut(output)
	root.SetErr(diagnostics)
	root.SetArgs([]string{"object", "put", "ARTIFACTS", "/tmp/release.tar", "--name", "release.tar", "--description", "production", "--metadata", "environment=prod", "--header", "Content-Type=application/tar"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("object put error = %v", err)
	}
	if transfer.upload.Bucket != "ARTIFACTS" || transfer.upload.Name != "release.tar" || transfer.upload.Metadata["environment"] != "prod" || transfer.upload.Headers["Content-Type"][0] != "application/tar" {
		t.Fatalf("upload = %#v", transfer.upload)
	}
	if output.String() != "uploaded ARTIFACTS/release.tar (7 bytes, SHA-256=abc)\n" || !strings.Contains(diagnostics.String(), "7/7") {
		t.Fatalf("stdout=%q stderr=%q", output.String(), diagnostics.String())
	}
}

func TestObjectGetRequiresOutputAndPassesForce(t *testing.T) {
	t.Parallel()
	transfer := &stubObjectTransfer{downloadResult: objectstore.DownloadResult{Object: objectstore.ObjectSummary{Name: "release.tar", Size: 7}, Verified: true}}
	root := NewRoot(Dependencies{ObjectTransfer: transfer})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"object", "get", "ARTIFACTS", "release.tar", "--output", "/tmp/release.tar", "--force"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("object get error = %v", err)
	}
	if transfer.download.Path != "/tmp/release.tar" || !transfer.download.Force || output.String() != "downloaded ARTIFACTS/release.tar to /tmp/release.tar (verified)\n" {
		t.Fatalf("download=%#v output=%q", transfer.download, output.String())
	}
}

func TestObjectEditBuildsAnExplicitPatch(t *testing.T) {
	t.Parallel()
	editor := &stubObjectEditor{result: objectstore.ObjectSummary{Name: "stable.tar"}}
	root := NewRoot(Dependencies{ObjectEditor: editor})
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"object", "edit", "ARTIFACTS", "release.tar", "--name", "stable.tar", "--description", "stable", "--metadata", "env=prod", "--clear-headers"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("object edit error = %v", err)
	}
	if editor.patch.NewName == nil || *editor.patch.NewName != "stable.tar" || editor.patch.Description == nil ||
		editor.patch.Metadata == nil || (*editor.patch.Metadata)["env"] != "prod" || editor.patch.Headers == nil || len(*editor.patch.Headers) != 0 {
		t.Fatalf("patch = %#v", editor.patch)
	}
	if output.String() != "updated object ARTIFACTS/stable.tar\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestObjectDeleteRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()
	deleter := &stubObjectDeleter{}
	unsafe := NewRoot(Dependencies{ObjectDeleter: deleter})
	unsafe.SetArgs([]string{"object", "delete", "ARTIFACTS", "release.tar"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil || deleter.name != "" {
		t.Fatalf("unsafe delete error=%v name=%q", err, deleter.name)
	}
	confirmed := NewRoot(Dependencies{ObjectDeleter: deleter})
	output := new(bytes.Buffer)
	confirmed.SetOut(output)
	confirmed.SetArgs([]string{"object", "delete", "ARTIFACTS", "release.tar", "--yes"})
	if err := confirmed.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("confirmed delete error = %v", err)
	}
	if deleter.bucket != "ARTIFACTS" || deleter.name != "release.tar" || output.String() != "deleted object ARTIFACTS/release.tar\n" {
		t.Fatalf("delete=%s/%s output=%q", deleter.bucket, deleter.name, output.String())
	}
}

func TestObjectLinkCreatesObjectAndBucketLinks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       []string
		wantTarget string
		wantOutput string
	}{
		{name: "object", args: []string{"object", "link", "ROOT", "latest", "RELEASES", "v1.tar"}, wantTarget: "v1.tar", wantOutput: "created object link ROOT/latest -> RELEASES/v1.tar\n"},
		{name: "bucket", args: []string{"object", "link", "ROOT", "releases", "RELEASES"}, wantOutput: "created bucket link ROOT/releases -> RELEASES\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			creator := &stubObjectLinkCreator{result: objectstore.ObjectSummary{Name: test.args[3]}}
			root := NewRoot(Dependencies{ObjectLinkCreator: creator})
			output := new(bytes.Buffer)
			root.SetOut(output)
			root.SetArgs(test.args)
			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("object link error = %v", err)
			}
			if creator.definition.Bucket != "ROOT" || creator.definition.TargetBucket != "RELEASES" || creator.definition.TargetObject != test.wantTarget || output.String() != test.wantOutput {
				t.Fatalf("definition=%#v output=%q", creator.definition, output.String())
			}
		})
	}
}

func TestObjectBucketCreateEditAndDeleteCommands(t *testing.T) {
	t.Parallel()
	creator := &stubObjectBucketCreator{}
	create := NewRoot(Dependencies{ObjectBucketCreator: creator})
	create.SetOut(new(bytes.Buffer))
	create.SetArgs([]string{"object", "bucket", "create", "ARTIFACTS", "--storage", "memory", "--ttl", "1h", "--max-bytes", "4096", "--replicas", "3", "--compression", "--metadata", "owner=platform", "--source", "REGION_A", "--source", "REGION_B"})
	if err := create.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("create error = %v", err)
	}
	if creator.definition.Storage != objectstore.StorageMemory || creator.definition.TTL != time.Hour || creator.definition.MaxBytes != 4096 || creator.definition.Replicas != 3 || !creator.definition.Compression || creator.definition.Metadata["owner"] != "platform" || !reflect.DeepEqual(creator.definition.Sources, []string{"REGION_A", "REGION_B"}) {
		t.Fatalf("definition = %#v", creator.definition)
	}

	updater := &stubObjectBucketUpdater{}
	edit := NewRoot(Dependencies{ObjectBucketUpdater: updater})
	edit.SetOut(new(bytes.Buffer))
	edit.SetArgs([]string{"object", "bucket", "edit", "ARTIFACTS", "--ttl", "2h", "--compression=false", "--clear-metadata"})
	if err := edit.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("edit error = %v", err)
	}
	if updater.changes.TTL == nil || *updater.changes.TTL != 2*time.Hour || updater.changes.Compression == nil || *updater.changes.Compression || updater.changes.Metadata == nil || len(*updater.changes.Metadata) != 0 || updater.changes.Description != nil {
		t.Fatalf("changes = %#v", updater.changes)
	}

	deleter := &stubObjectBucketDeleter{}
	unsafe := NewRoot(Dependencies{ObjectBucketDeleter: deleter})
	unsafe.SetArgs([]string{"object", "bucket", "delete", "ARTIFACTS"})
	if err := unsafe.ExecuteContext(context.Background()); err == nil || deleter.name != "" {
		t.Fatalf("unsafe delete error=%v name=%q", err, deleter.name)
	}
	confirmed := NewRoot(Dependencies{ObjectBucketDeleter: deleter})
	confirmed.SetArgs([]string{"object", "bucket", "delete", "ARTIFACTS", "--yes"})
	if err := confirmed.ExecuteContext(context.Background()); err != nil || deleter.name != "ARTIFACTS" {
		t.Fatalf("confirmed delete error=%v name=%q", err, deleter.name)
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestProgressReporterUsesInjectedClock(t *testing.T) {
	t.Parallel()
	output := new(bytes.Buffer)
	reporter := newProgressReporter(output, fixedClock{now: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)})
	reporter.Report(objectstore.Progress{Transferred: 1, Total: 10})
	reporter.Report(objectstore.Progress{Transferred: 2, Total: 10})
	reporter.Report(objectstore.Progress{Transferred: 10, Total: 10})
	if output.String() != "1/10 bytes\n10/10 bytes\n" {
		t.Fatalf("progress output = %q", output.String())
	}
}

func TestRootRunsTUIWithResolvedConfig(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{}
	root := NewRoot(Dependencies{Runner: runner})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"--server", "nats://demo:4222", "--monitor-url", "http://demo:8222", "--profile", "demo"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if runner.settings.Server != "nats://demo:4222" {
		t.Fatalf("server = %q, want %q", runner.settings.Server, "nats://demo:4222")
	}
	if runner.settings.Profile != "demo" {
		t.Fatalf("profile = %q, want %q", runner.settings.Profile, "demo")
	}
	if runner.settings.MonitorURL != "http://demo:8222" {
		t.Fatalf("monitor URL = %q", runner.settings.MonitorURL)
	}
}

type stubRunner struct {
	settings config.Settings
}

func TestRootPassesUXSettingsToRunner(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{}
	root := NewRoot(Dependencies{Runner: runner})
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"--mouse", "--theme", "accessible", "--key-help", "h", "--key-activity", "ctrl+a"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !runner.settings.Mouse || runner.settings.Theme != "accessible" || runner.settings.KeyHelp != "h" || runner.settings.KeyActivity != "ctrl+a" {
		t.Fatalf("settings = %#v", runner.settings)
	}
}

type stubKeyValueCreator struct {
	settings   config.Settings
	definition kvstore.Definition
	called     bool
}

type stubKeyValueUpdater struct {
	name    string
	changes kvstore.Changes
}

type stubKeyValueDeleter struct{ name string }

func (s *stubKeyValueDeleter) DeleteKeyValueBucket(_ context.Context, _ config.Settings, name string) error {
	s.name = name
	return nil
}

type stubKeyValueKeyLister struct {
	bucket string
	keys   []string
}

type stubKeyValueEntryReader struct {
	bucket string
	key    string
	entry  kvstore.Entry
}

type stubKeyValueEntryWriter struct {
	command  kvstore.WriteEntry
	revision uint64
}

type stubKeyValueEntryRemover struct {
	command kvstore.RemoveEntry
	called  bool
}

type stubKeyValueHistoryReader struct {
	bucket  string
	key     string
	entries []kvstore.Entry
}

type stubKeyValueRestorer struct {
	command kvstore.RestoreEntry
	result  kvstore.RestoredEntry
}

type stubKeyValueWatcher struct {
	bucket string
	key    string
	events <-chan kvstore.Entry
}

type stubObjectTransfer struct {
	upload         objectstore.UploadFile
	download       objectstore.DownloadFile
	uploadResult   objectstore.ObjectSummary
	downloadResult objectstore.DownloadResult
}

type stubObjectEditor struct {
	patch  objectstore.EditObject
	result objectstore.ObjectSummary
}

func (s *stubObjectEditor) EditObject(_ context.Context, _ config.Settings, patch objectstore.EditObject) (objectstore.ObjectSummary, error) {
	s.patch = patch
	return s.result, nil
}

type stubObjectDeleter struct{ bucket, name string }

func (s *stubObjectDeleter) DeleteObject(_ context.Context, _ config.Settings, bucket, name string) error {
	s.bucket, s.name = bucket, name
	return nil
}

type stubObjectLinkCreator struct {
	definition objectstore.LinkDefinition
	result     objectstore.ObjectSummary
}

type stubObjectBucketCreator struct{ definition objectstore.Definition }

func (s *stubObjectBucketCreator) CreateObjectStoreBucket(_ context.Context, _ config.Settings, definition objectstore.Definition) error {
	s.definition = definition
	return nil
}

type stubObjectBucketUpdater struct {
	name    string
	changes objectstore.Changes
}

func (s *stubObjectBucketUpdater) UpdateObjectStoreBucket(_ context.Context, _ config.Settings, name string, changes objectstore.Changes) error {
	s.name, s.changes = name, changes
	return nil
}

type stubObjectBucketDeleter struct{ name string }

func (s *stubObjectBucketDeleter) DeleteObjectStoreBucket(_ context.Context, _ config.Settings, name string) error {
	s.name = name
	return nil
}

func (s *stubObjectLinkCreator) CreateObjectLink(_ context.Context, _ config.Settings, definition objectstore.LinkDefinition) (objectstore.ObjectSummary, error) {
	s.definition = definition
	return s.result, nil
}

func (s *stubObjectTransfer) UploadObjectFile(_ context.Context, _ config.Settings, command objectstore.UploadFile, progress objectstore.ProgressFunc) (objectstore.ObjectSummary, error) {
	s.upload = command
	progress(objectstore.Progress{Transferred: s.uploadResult.Size, Total: s.uploadResult.Size})
	return s.uploadResult, nil
}

func (s *stubObjectTransfer) DownloadObjectFile(_ context.Context, _ config.Settings, command objectstore.DownloadFile, progress objectstore.ProgressFunc) (objectstore.DownloadResult, error) {
	s.download = command
	progress(objectstore.Progress{Transferred: s.downloadResult.Object.Size, Total: s.downloadResult.Object.Size})
	return s.downloadResult, nil
}

func (s *stubKeyValueWatcher) WatchKeyValue(_ context.Context, _ config.Settings, bucket, key string) (<-chan kvstore.Entry, error) {
	s.bucket, s.key = bucket, key
	return s.events, nil
}

func (s *stubKeyValueRestorer) RestoreKeyValueEntry(_ context.Context, _ config.Settings, command kvstore.RestoreEntry) (kvstore.RestoredEntry, error) {
	s.command = command
	return s.result, nil
}

func (s *stubKeyValueHistoryReader) ListKeyValueHistory(_ context.Context, _ config.Settings, bucket, key string) ([]kvstore.Entry, error) {
	s.bucket, s.key = bucket, key
	return s.entries, nil
}

func (s *stubKeyValueEntryRemover) RemoveKeyValueEntry(_ context.Context, _ config.Settings, command kvstore.RemoveEntry) error {
	s.command = command
	s.called = true
	return nil
}

func (s *stubKeyValueEntryWriter) PutKeyValueEntry(_ context.Context, _ config.Settings, command kvstore.WriteEntry) (uint64, error) {
	s.command = command
	return s.revision, nil
}

func (s *stubKeyValueEntryReader) GetKeyValueEntry(_ context.Context, _ config.Settings, bucket, key string) (kvstore.Entry, error) {
	s.bucket, s.key = bucket, key
	return s.entry, nil
}

func (s *stubKeyValueKeyLister) ListKeyValueKeys(_ context.Context, _ config.Settings, bucket string) ([]string, error) {
	s.bucket = bucket
	return s.keys, nil
}

func (s *stubKeyValueUpdater) UpdateKeyValueBucket(_ context.Context, _ config.Settings, name string, changes kvstore.Changes) error {
	s.name, s.changes = name, changes
	return nil
}

func (s *stubKeyValueCreator) CreateKeyValueBucket(_ context.Context, settings config.Settings, definition kvstore.Definition) error {
	s.settings = settings
	s.definition = definition
	s.called = true
	return nil
}

func (r *stubRunner) Run(_ context.Context, settings config.Settings) error {
	r.settings = settings
	return nil
}
