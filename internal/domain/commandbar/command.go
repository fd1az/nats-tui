package commandbar

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidDefinition = errors.New("invalid command definition")
	ErrMissingParameter  = errors.New("required command parameter is missing")
	ErrHistoryCompleted  = errors.New("command history entry is already complete")
	identifierPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
	placeholderPattern   = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_-]*)\s*\}\}`)
)

type Risk string

const (
	RiskRead        Risk = "read"
	RiskMutation    Risk = "mutation"
	RiskDestructive Risk = "destructive"
)

type ParameterInput struct {
	Name, Help, Default string
	Required            bool
}
type Parameter struct {
	name, help, defaultValue string
	required                 bool
}

func (p Parameter) Name() string    { return p.name }
func (p Parameter) Help() string    { return p.help }
func (p Parameter) Default() string { return p.defaultValue }
func (p Parameter) Required() bool  { return p.required }

type DefinitionInput struct {
	ID, Title, Description, Group, Action string
	Risk                                  Risk
	Parameters                            []ParameterInput
	Arguments                             map[string]string
	Equivalent                            []string
	Custom                                bool
}
type Definition struct {
	id, title, description, group, action string
	risk                                  Risk
	parameters                            []Parameter
	arguments                             map[string]string
	equivalent                            []string
	custom                                bool
}

func NewDefinition(input DefinitionInput) (Definition, error) {
	input.ID = strings.ToLower(strings.TrimSpace(input.ID))
	input.Title = strings.TrimSpace(input.Title)
	input.Action = strings.TrimSpace(input.Action)
	if !identifierPattern.MatchString(input.ID) || input.Title == "" || !identifierPattern.MatchString(input.Action) {
		return Definition{}, fmt.Errorf("%w: ID, title, and typed action are required", ErrInvalidDefinition)
	}
	if input.Risk == "" {
		input.Risk = RiskRead
	}
	if input.Risk != RiskRead && input.Risk != RiskMutation && input.Risk != RiskDestructive {
		return Definition{}, fmt.Errorf("%w: unknown risk %q", ErrInvalidDefinition, input.Risk)
	}
	parameters := make([]Parameter, 0, len(input.Parameters))
	known := map[string]struct{}{}
	for _, raw := range input.Parameters {
		name := strings.TrimSpace(raw.Name)
		if !identifierPattern.MatchString(name) || strings.Contains(name, ".") {
			return Definition{}, fmt.Errorf("%w: bad parameter %q", ErrInvalidDefinition, name)
		}
		if _, ok := known[name]; ok {
			return Definition{}, fmt.Errorf("%w: duplicate parameter %q", ErrInvalidDefinition, name)
		}
		known[name] = struct{}{}
		parameters = append(parameters, Parameter{name: name, help: strings.TrimSpace(raw.Help), defaultValue: raw.Default, required: raw.Required})
	}
	arguments := make(map[string]string, len(input.Arguments))
	for name, template := range input.Arguments {
		if strings.TrimSpace(name) == "" {
			return Definition{}, fmt.Errorf("%w: argument name is required", ErrInvalidDefinition)
		}
		if err := validatePlaceholders(template, known); err != nil {
			return Definition{}, err
		}
		arguments[name] = template
	}
	for _, template := range input.Equivalent {
		if err := validatePlaceholders(template, known); err != nil {
			return Definition{}, err
		}
	}
	return Definition{id: input.ID, title: input.Title, description: strings.TrimSpace(input.Description), group: strings.TrimSpace(input.Group), action: input.Action, risk: input.Risk, parameters: parameters, arguments: arguments, equivalent: append([]string(nil), input.Equivalent...), custom: input.Custom}, nil
}
func validatePlaceholders(template string, known map[string]struct{}) error {
	for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		if _, ok := known[match[1]]; !ok {
			return fmt.Errorf("%w: unknown placeholder %q", ErrInvalidDefinition, match[1])
		}
	}
	remaining := placeholderPattern.ReplaceAllString(template, "")
	if strings.Contains(remaining, "{{") || strings.Contains(remaining, "}}") {
		return fmt.Errorf("%w: malformed placeholder", ErrInvalidDefinition)
	}
	return nil
}

func (d Definition) Resolve(values map[string]string) (Invocation, error) {
	resolved := make(map[string]string, len(d.parameters))
	known := make(map[string]struct{}, len(d.parameters))
	for _, parameter := range d.parameters {
		known[parameter.name] = struct{}{}
	}
	for name := range values {
		if _, ok := known[name]; !ok {
			return Invocation{}, fmt.Errorf("unknown command parameter %q", name)
		}
	}
	for _, parameter := range d.parameters {
		value, ok := values[parameter.name]
		if !ok || value == "" {
			value = parameter.defaultValue
		}
		if parameter.required && strings.TrimSpace(value) == "" {
			return Invocation{}, fmt.Errorf("%w: %s", ErrMissingParameter, parameter.name)
		}
		resolved[parameter.name] = value
	}
	arguments := make(map[string]string, len(d.arguments))
	for name, template := range d.arguments {
		arguments[name] = interpolate(template, resolved)
	}
	equivalent := make([]string, len(d.equivalent))
	for i, part := range d.equivalent {
		equivalent[i] = interpolate(part, resolved)
	}
	return Invocation{id: d.id, title: d.title, action: d.action, risk: d.risk, arguments: arguments, equivalent: equivalent}, nil
}
func interpolate(template string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		return values[parts[1]]
	})
}
func (d Definition) ID() string              { return d.id }
func (d Definition) Title() string           { return d.title }
func (d Definition) Description() string     { return d.description }
func (d Definition) Group() string           { return d.group }
func (d Definition) Action() string          { return d.action }
func (d Definition) Risk() Risk              { return d.risk }
func (d Definition) Parameters() []Parameter { return append([]Parameter(nil), d.parameters...) }
func (d Definition) Custom() bool            { return d.custom }
func (d Definition) Input() DefinitionInput {
	params := make([]ParameterInput, len(d.parameters))
	for i, p := range d.parameters {
		params[i] = ParameterInput{Name: p.name, Help: p.help, Default: p.defaultValue, Required: p.required}
	}
	args := make(map[string]string, len(d.arguments))
	for k, v := range d.arguments {
		args[k] = v
	}
	return DefinitionInput{ID: d.id, Title: d.title, Description: d.description, Group: d.group, Action: d.action, Risk: d.risk, Parameters: params, Arguments: args, Equivalent: append([]string(nil), d.equivalent...), Custom: d.custom}
}

type Invocation struct {
	id, title, action string
	risk              Risk
	arguments         map[string]string
	equivalent        []string
}

func (i Invocation) ID() string                  { return i.id }
func (i Invocation) Title() string               { return i.title }
func (i Invocation) Action() string              { return i.action }
func (i Invocation) Risk() Risk                  { return i.risk }
func (i Invocation) Argument(name string) string { return i.arguments[name] }
func (i Invocation) Arguments() map[string]string {
	result := make(map[string]string, len(i.arguments))
	for k, v := range i.arguments {
		result[k] = v
	}
	return result
}
func (i Invocation) Equivalent() []string { return append([]string(nil), i.equivalent...) }

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type HistoryEntry struct {
	id, commandID, title, errorText string
	startedAt, completedAt          time.Time
	status                          Status
}

func NewHistoryEntry(id, commandID, title string, startedAt time.Time) (*HistoryEntry, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(commandID) == "" || strings.TrimSpace(title) == "" || startedAt.IsZero() {
		return nil, errors.New("command history identity, command, title, and start time are required")
	}
	return &HistoryEntry{id: id, commandID: commandID, title: title, startedAt: startedAt, status: StatusRunning}, nil
}
func RehydrateHistoryEntry(id, commandID, title, errorText string, startedAt, completedAt time.Time, status Status) (*HistoryEntry, error) {
	entry, err := NewHistoryEntry(id, commandID, title, startedAt)
	if err != nil {
		return nil, err
	}
	if status == StatusRunning {
		return entry, nil
	}
	if status != StatusSucceeded && status != StatusFailed {
		return nil, errors.New("invalid command history status")
	}
	if completedAt.Before(startedAt) {
		return nil, errors.New("command completion precedes start")
	}
	entry.status, entry.completedAt, entry.errorText = status, completedAt, errorText
	return entry, nil
}
func (h *HistoryEntry) Complete(at time.Time, errorText string) error {
	if h.status != StatusRunning {
		return ErrHistoryCompleted
	}
	if at.Before(h.startedAt) {
		return errors.New("command completion precedes start")
	}
	h.completedAt = at
	h.errorText = strings.TrimSpace(errorText)
	h.status = StatusSucceeded
	if h.errorText != "" {
		h.status = StatusFailed
	}
	return nil
}
func (h HistoryEntry) ID() string             { return h.id }
func (h HistoryEntry) CommandID() string      { return h.commandID }
func (h HistoryEntry) Title() string          { return h.title }
func (h HistoryEntry) Error() string          { return h.errorText }
func (h HistoryEntry) StartedAt() time.Time   { return h.startedAt }
func (h HistoryEntry) CompletedAt() time.Time { return h.completedAt }
func (h HistoryEntry) Status() Status         { return h.status }
func (h HistoryEntry) Duration() time.Duration {
	if h.completedAt.IsZero() {
		return 0
	}
	return h.completedAt.Sub(h.startedAt)
}

func SortedArgumentKeys(arguments map[string]string) []string {
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
