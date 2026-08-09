package messages

import (
	"strconv"
	"strings"
)

type searchEntry struct {
	record   Record
	haystack string
}

type SearchIndex struct {
	entries []searchEntry
}

func NewSearchIndex(records []Record) SearchIndex {
	entries := make([]searchEntry, len(records))
	for index, record := range records {
		entries[index] = newSearchEntry(record)
	}
	return SearchIndex{entries: entries}
}

func (i SearchIndex) Search(query string) []Record {
	return i.SearchInto(nil, query)
}

func (i SearchIndex) SearchInto(result []Record, query string) []Record {
	terms := strings.Fields(strings.ToLower(query))
	result = result[:0]
	for _, entry := range i.entries {
		if matchesAll(entry.haystack, terms) {
			result = append(result, entry.record)
		}
	}
	return result
}

func (i *SearchIndex) Prepend(record Record, limit int) {
	i.PrependBatch([]Record{record}, limit)
}

func (i *SearchIndex) PrependBatch(records []Record, limit int) {
	if limit <= 0 {
		i.entries = nil
		return
	}
	batchSize := min(len(records), limit)
	entries := make([]searchEntry, batchSize, min(len(i.entries)+batchSize, limit))
	for index := range batchSize {
		entries[index] = newSearchEntry(records[len(records)-1-index])
	}
	remaining := min(len(i.entries), limit-batchSize)
	entries = append(entries, i.entries[:remaining]...)
	i.entries = entries
}

func newSearchEntry(record Record) searchEntry {
	var value strings.Builder
	value.WriteString(record.Subject)
	value.WriteByte(' ')
	value.Write(record.Payload)
	value.WriteByte(' ')
	value.WriteString(strconv.FormatUint(record.Sequence, 10))
	for name, values := range record.Headers {
		value.WriteByte(' ')
		value.WriteString(name)
		value.WriteByte(' ')
		value.WriteString(strings.Join(values, " "))
	}
	return searchEntry{record: record, haystack: strings.ToLower(value.String())}
}

func matchesAll(haystack string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}
