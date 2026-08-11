package messages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ExportFormat string

const (
	ExportJSON  ExportFormat = "json"
	ExportJSONL ExportFormat = "jsonl"
	ExportRaw   ExportFormat = "raw"
)

type ExportRequest struct {
	Query   HistoryQuery
	Path    string
	Format  ExportFormat
	Force   bool
	Records []Record
}

type ExportResult struct {
	Path, Format string
	Messages     int
	Bytes        int
}

type FileWriter interface {
	WriteMessageFile(context.Context, string, []byte, bool) error
}

type ExportHandler struct {
	browser Browser
	writer  FileWriter
}

func NewExportHandler(browser Browser, writer FileWriter) ExportHandler {
	if writer == nil {
		panic("nil message export writer")
	}
	return ExportHandler{browser: browser, writer: writer}
}

func (h ExportHandler) Handle(ctx context.Context, request ExportRequest) (ExportResult, error) {
	if strings.TrimSpace(request.Path) == "" {
		return ExportResult{}, fmt.Errorf("export path is required")
	}
	if request.Format != ExportJSON && request.Format != ExportJSONL && request.Format != ExportRaw {
		return ExportResult{}, fmt.Errorf("unsupported message export format %q", request.Format)
	}
	records := append([]Record(nil), request.Records...)
	if len(records) == 0 {
		if h.browser == nil {
			return ExportResult{}, fmt.Errorf("message browser is required when export records are not provided")
		}
		var err error
		records, err = NewBrowseHandler(h.browser).Handle(ctx, request.Query)
		if err != nil {
			return ExportResult{}, err
		}
	}
	content, err := encodeExport(records, request.Format)
	if err != nil {
		return ExportResult{}, err
	}
	if err := h.writer.WriteMessageFile(ctx, request.Path, content, request.Force); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: request.Path, Format: string(request.Format), Messages: len(records), Bytes: len(content)}, nil
}

func encodeExport(records []Record, format ExportFormat) ([]byte, error) {
	if format == ExportRaw {
		if len(records) != 1 {
			return nil, fmt.Errorf("raw export requires exactly one message")
		}
		return append([]byte(nil), records[0].Payload...), nil
	}
	envelopes := make([]json.RawMessage, 0, len(records))
	for _, record := range records {
		complete, err := CompleteMessage(record)
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, json.RawMessage(complete))
	}
	if format == ExportJSON {
		content, err := json.MarshalIndent(envelopes, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode message export: %w", err)
		}
		return append(content, '\n'), nil
	}
	var output strings.Builder
	for _, envelope := range envelopes {
		var compact bytes.Buffer
		if err := json.Compact(&compact, envelope); err != nil {
			return nil, fmt.Errorf("encode JSONL message: %w", err)
		}
		output.WriteString(compact.String())
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}
