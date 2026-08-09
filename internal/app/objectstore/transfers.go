package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrDigestMismatch reports that downloaded bytes do not match object metadata.
var ErrDigestMismatch = errors.New("object digest mismatch")

// Progress describes a transfer checkpoint.
type Progress struct {
	Transferred uint64
	Total       uint64
}

// ProgressFunc receives synchronous transfer checkpoints.
type ProgressFunc func(Progress)

// UploadCommand describes an object upload stream.
type UploadCommand struct {
	Bucket      string
	Name        string
	Description string
	Headers     map[string][]string
	Metadata    map[string]string
	Reader      io.Reader
	Size        uint64
}

// DownloadCommand describes an object download destination.
type DownloadCommand struct {
	Bucket string
	Name   string
	Writer io.Writer
}

// UploadFile describes a local file selected for upload.
type UploadFile struct {
	Bucket      string
	Path        string
	Name        string
	Description string
	Headers     map[string][]string
	Metadata    map[string]string
}

// DownloadFile describes a safe local download destination.
type DownloadFile struct {
	Bucket string
	Name   string
	Path   string
	Force  bool
}

// DownloadResult reports the downloaded object and digest verification.
type DownloadResult struct {
	Object   ObjectSummary
	Verified bool
}

// TransferEvent carries progress or terminal state to interactive clients.
type TransferEvent struct {
	Progress    Progress
	Object      ObjectSummary
	Destination string
	Verified    bool
	Err         error
	Done        bool
}

// DownloadSource is a readable object with provider verification.
type DownloadSource interface {
	io.ReadCloser
	Info() ObjectSummary
	Verify() error
}

// TransferStore provides streaming object operations.
type TransferStore interface {
	PutObject(context.Context, UploadCommand) (ObjectSummary, error)
	OpenObject(context.Context, string, string) (DownloadSource, error)
}

// UploadHandler validates and streams an object upload.
type UploadHandler struct{ store TransferStore }

// NewUploadHandler constructs an upload command handler.
func NewUploadHandler(store TransferStore) UploadHandler {
	if store == nil {
		panic("nil object transfer store")
	}
	return UploadHandler{store: store}
}

// Handle uploads bytes while reporting progress.
func (h UploadHandler) Handle(ctx context.Context, command UploadCommand, progress ProgressFunc) (ObjectSummary, error) {
	if err := validateTransfer(command.Bucket, command.Name); err != nil {
		return ObjectSummary{}, err
	}
	if command.Reader == nil {
		return ObjectSummary{}, fmt.Errorf("upload reader is required")
	}
	command.Reader = &progressReader{ctx: ctx, reader: command.Reader, total: command.Size, progress: progress}
	object, err := h.store.PutObject(ctx, command)
	if err != nil {
		return ObjectSummary{}, err
	}
	completed := object.Size
	if completed == 0 {
		completed = command.Size
	}
	reportProgress(progress, completed, completed)
	return object, nil
}

// DownloadHandler validates, streams, and verifies an object download.
type DownloadHandler struct{ store TransferStore }

// NewDownloadHandler constructs a download command handler.
func NewDownloadHandler(store TransferStore) DownloadHandler {
	if store == nil {
		panic("nil object transfer store")
	}
	return DownloadHandler{store: store}
}

// Handle downloads an object and verifies both provider and SHA-256 digests.
func (h DownloadHandler) Handle(ctx context.Context, command DownloadCommand, progress ProgressFunc) (DownloadResult, error) {
	if err := validateTransfer(command.Bucket, command.Name); err != nil {
		return DownloadResult{}, err
	}
	if command.Writer == nil {
		return DownloadResult{}, fmt.Errorf("download writer is required")
	}
	source, err := h.store.OpenObject(ctx, command.Bucket, command.Name)
	if err != nil {
		return DownloadResult{}, err
	}
	info := source.Info()
	hash := sha256.New()
	destination := &progressWriter{writer: io.MultiWriter(command.Writer, hash), total: info.Size, progress: progress}
	_, copyErr := io.Copy(destination, &contextReader{ctx: ctx, reader: source})
	if copyErr != nil {
		return DownloadResult{}, errors.Join(fmt.Errorf("download object: %w", copyErr), source.Close())
	}
	if err := source.Verify(); err != nil {
		return DownloadResult{}, errors.Join(fmt.Errorf("verify object download: %w", err), source.Close())
	}
	if err := verifyDigest(info.Digest, hash.Sum(nil)); err != nil {
		return DownloadResult{}, errors.Join(err, source.Close())
	}
	if err := source.Close(); err != nil {
		return DownloadResult{}, fmt.Errorf("close object download: %w", err)
	}
	reportProgress(progress, info.Size, info.Size)
	return DownloadResult{Object: info, Verified: true}, nil
}

func validateTransfer(bucket, name string) error {
	if strings.TrimSpace(bucket) == "" {
		return fmt.Errorf("object-store bucket is required")
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("object name is required")
	}
	return nil
}

func verifyDigest(expected string, actual []byte) error {
	if expected == "" {
		return fmt.Errorf("object digest is missing")
	}
	if digestForBytes(actual) != expected {
		return fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, expected, digestForBytes(actual))
	}
	return nil
}

func digestFor(value []byte) string {
	sum := sha256.Sum256(value)
	return digestForBytes(sum[:])
}

func digestForBytes(sum []byte) string {
	return "SHA-256=" + base64.URLEncoding.EncodeToString(sum)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(buffer)
	}
}

type progressReader struct {
	ctx         context.Context
	reader      io.Reader
	total       uint64
	transferred uint64
	progress    ProgressFunc
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	read, err := (&contextReader{ctx: r.ctx, reader: r.reader}).Read(buffer)
	r.transferred += uint64(read)
	reportProgress(r.progress, r.transferred, r.total)
	return read, err
}

type progressWriter struct {
	writer      io.Writer
	total       uint64
	transferred uint64
	progress    ProgressFunc
}

func (w *progressWriter) Write(buffer []byte) (int, error) {
	written, err := w.writer.Write(buffer)
	w.transferred += uint64(written)
	reportProgress(w.progress, w.transferred, w.total)
	return written, err
}

func reportProgress(progress ProgressFunc, transferred, total uint64) {
	if progress != nil {
		progress(Progress{Transferred: transferred, Total: total})
	}
}
