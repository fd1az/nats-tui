package commandbar

import (
	"sort"
	"strings"

	domain "github.com/fd1az/nats-tui/internal/domain/commandbar"
)

type Entry struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Group       string            `json:"group,omitempty"`
	Action      string            `json:"action"`
	Risk        domain.Risk       `json:"risk"`
	Definition  domain.Definition `json:"-"`
}

func EntryFrom(definition domain.Definition) Entry {
	return Entry{ID: definition.ID(), Title: definition.Title(), Description: definition.Description(), Group: definition.Group(), Action: definition.Action(), Risk: definition.Risk(), Definition: definition}
}

type indexedEntry struct {
	entry    Entry
	haystack string
	index    int
}
type Index struct{ values []indexedEntry }

func NewIndex(values []Entry) *Index {
	indexed := make([]indexedEntry, len(values))
	for i, value := range values {
		indexed[i] = indexedEntry{entry: value, haystack: normalize(strings.Join([]string{value.Title, value.ID, value.Description, value.Group, value.Action}, " ")), index: i}
	}
	return &Index{values: indexed}
}
func Search(values []Entry, query string, limit int) []Entry {
	return NewIndex(values).Search(query, limit)
}
func (index *Index) Search(query string, limit int) []Entry {
	query = normalize(query)
	type ranked struct {
		entry Entry
		score int
		index int
	}
	if index == nil {
		return nil
	}
	matches := make([]ranked, 0, len(index.values))
	for _, value := range index.values {
		score := matchScore(query, value.haystack)
		if query == "" {
			score = 1
		}
		if score > 0 {
			matches = append(matches, ranked{value.entry, score, value.index})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].index < matches[j].index
		}
		return matches[i].score > matches[j].score
	})
	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	result := make([]Entry, limit)
	for i := range result {
		result[i] = matches[i].entry
	}
	return result
}
func normalize(value string) string { return strings.ToLower(strings.Join(strings.Fields(value), " ")) }
func matchScore(query, value string) int {
	if query == "" {
		return 1
	}
	if index := strings.Index(value, query); index >= 0 {
		return 1000 - len(value) - index*2
	}
	score, last, streak := 0, -1, 0
	for _, character := range query {
		position := strings.IndexRune(value[last+1:], character)
		if position < 0 {
			return 0
		}
		position += last + 1
		if position == last+1 {
			streak++
			score += 20 + streak*5
		} else {
			streak = 0
			score += 5
		}
		last = position
	}
	return score - len(value)/10
}

type PreviewView struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Action               string            `json:"action"`
	Risk                 domain.Risk       `json:"risk"`
	Arguments            map[string]string `json:"arguments,omitempty"`
	Equivalent           []string          `json:"equivalent_argv,omitempty"`
	Command              string            `json:"equivalent_command,omitempty"`
	RequiresConfirmation bool              `json:"requires_confirmation"`
}

func Preview(definition domain.Definition, values map[string]string) (PreviewView, error) {
	invocation, err := definition.Resolve(values)
	if err != nil {
		return PreviewView{}, err
	}
	equivalent := invocation.Equivalent()
	return PreviewView{ID: invocation.ID(), Title: invocation.Title(), Action: invocation.Action(), Risk: invocation.Risk(), Arguments: invocation.Arguments(), Equivalent: equivalent, Command: shellDisplay(equivalent), RequiresConfirmation: invocation.Risk() != domain.RiskRead}, nil
}
func shellDisplay(parts []string) string {
	quoted := make([]string, len(parts))
	for i, part := range parts {
		if part != "" && strings.IndexFunc(part, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-._/:@", r))
		}) < 0 {
			quoted[i] = part
		} else {
			quoted[i] = "'" + strings.ReplaceAll(part, "'", "'\\''") + "'"
		}
	}
	return strings.Join(quoted, " ")
}
