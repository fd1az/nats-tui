// Package subjects builds the read model used to explore configured NATS subjects.
package subjects

import (
	"slices"
	"strings"

	"github.com/clcsolutions/nats-tui/internal/app/consumers"
	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

// Node is one visible row in the flattened subject tree.
type Node struct {
	Subject      string
	WatchSubject string
	Depth        int
	Configured   bool
	Observed     bool
	Streams      []string
	Consumers    []string
	Permissions  []PermissionRule
	MessageRate  float64
	ByteRate     float64
	Heat         float64
}

type PermissionRule struct {
	Principal string
	Account   string
	Operation string
	Effect    string
	Pattern   string
}

// PrincipalPermissions is the Subject Explorer's anti-corruption input. It
// avoids coupling this read model to any particular authority implementation.
type PrincipalPermissions struct {
	Principal, Account                                       string
	PublishAllow, PublishDeny, SubscribeAllow, SubscribeDeny []string
}

// Build derives a deterministic subject tree from the stream catalog.
func Build(catalog []streams.Summary) []Node {
	type entry struct {
		configured bool
		streams    map[string]struct{}
	}
	entries := make(map[string]*entry)
	for _, stream := range catalog {
		for _, configuredSubject := range stream.Subjects {
			tokens := strings.Split(configuredSubject, ".")
			for depth := range tokens {
				subject := strings.Join(tokens[:depth+1], ".")
				current := entries[subject]
				if current == nil {
					current = &entry{streams: make(map[string]struct{})}
					entries[subject] = current
				}
				current.streams[stream.Name] = struct{}{}
				current.configured = current.configured || subject == configuredSubject
			}
		}
	}

	paths := make([]string, 0, len(entries))
	for path := range entries {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	result := make([]Node, 0, len(paths))
	for _, path := range paths {
		current := entries[path]
		owners := make([]string, 0, len(current.streams))
		for stream := range current.streams {
			owners = append(owners, stream)
		}
		slices.Sort(owners)
		watchSubject := path
		if !current.configured {
			watchSubject += ".>"
		}
		result = append(result, Node{
			Subject: path, WatchSubject: watchSubject, Depth: strings.Count(path, "."),
			Configured: current.configured, Streams: owners,
		})
	}
	return result
}

// Enrich projects every source of subject knowledge into the explorer read
// model. It intentionally has no infrastructure dependencies.
func Enrich(catalog []streams.Summary, consumerCatalog []consumers.Summary, permissions []PrincipalPermissions, traffic TrafficSnapshot) []Node {
	nodes := Build(catalog)
	entries := make(map[string]Node, len(nodes)+len(traffic.Subjects))
	for _, node := range nodes {
		entries[node.Subject] = node
	}
	for subject := range traffic.Observed {
		tokens := strings.Split(subject, ".")
		for depth := range tokens {
			path := strings.Join(tokens[:depth+1], ".")
			node, exists := entries[path]
			if !exists {
				node = Node{Subject: path, WatchSubject: path + ".>", Depth: depth}
			}
			if path == subject {
				node.Observed = true
				node.WatchSubject = path
			}
			entries[path] = node
		}
	}
	paths := make([]string, 0, len(entries))
	maximumRate := float64(0)
	for path, node := range entries {
		metric := traffic.Subjects[path]
		node.MessageRate, node.ByteRate = metric.MessageRate, metric.ByteRate
		if node.MessageRate > maximumRate {
			maximumRate = node.MessageRate
		}
		node.Streams = mergeSorted(node.Streams, owningStreams(node.Subject, catalog))
		node.Consumers = attachedConsumers(node, consumerCatalog)
		node.Permissions = affectingPermissions(node, permissions)
		entries[path] = node
		paths = append(paths, path)
	}
	slices.Sort(paths)
	result := make([]Node, 0, len(paths))
	for _, path := range paths {
		node := entries[path]
		if maximumRate > 0 {
			node.Heat = node.MessageRate / maximumRate
		}
		result = append(result, node)
	}
	return result
}

func mergeSorted(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func owningStreams(subject string, catalog []streams.Summary) []string {
	owners := make([]string, 0)
	for _, stream := range catalog {
		for _, pattern := range stream.Subjects {
			if PatternsOverlap(pattern, subject) {
				owners = append(owners, stream.Name)
				break
			}
		}
	}
	slices.Sort(owners)
	return owners
}

func attachedConsumers(node Node, catalog []consumers.Summary) []string {
	result := make([]string, 0)
	for _, consumer := range catalog {
		if !slices.Contains(node.Streams, consumer.Stream) {
			continue
		}
		filters := consumer.FilterSubjects
		if len(filters) == 0 {
			filters = consumer.Config.FilterSubjects
		}
		if len(filters) == 0 {
			result = append(result, consumer.Stream+"/"+consumer.Name)
			continue
		}
		for _, filter := range filters {
			if PatternsOverlap(filter, node.WatchSubject) {
				result = append(result, consumer.Stream+"/"+consumer.Name)
				break
			}
		}
	}
	slices.Sort(result)
	return result
}

func affectingPermissions(node Node, principals []PrincipalPermissions) []PermissionRule {
	result := make([]PermissionRule, 0)
	for _, principal := range principals {
		sets := []struct {
			operation, effect string
			patterns          []string
		}{
			{"publish", "allow", principal.PublishAllow}, {"publish", "deny", principal.PublishDeny},
			{"subscribe", "allow", principal.SubscribeAllow}, {"subscribe", "deny", principal.SubscribeDeny},
		}
		for _, set := range sets {
			for _, pattern := range set.patterns {
				if PatternsOverlap(pattern, node.WatchSubject) {
					result = append(result, PermissionRule{Principal: principal.Principal, Account: principal.Account, Operation: set.operation, Effect: set.effect, Pattern: pattern})
				}
			}
		}
	}
	slices.SortFunc(result, func(left, right PermissionRule) int {
		return strings.Compare(left.Principal+left.Operation+left.Effect+left.Pattern, right.Principal+right.Operation+right.Effect+right.Pattern)
	})
	return result
}

// PatternsOverlap reports whether two concrete-or-wildcard NATS subjects can
// address at least one common concrete subject.
func PatternsOverlap(left, right string) bool {
	return patternsOverlap(strings.Split(left, "."), strings.Split(right, "."), 0, 0)
}

func patternsOverlap(left, right []string, li, ri int) bool {
	if li == len(left) || ri == len(right) {
		return li == len(left) && ri == len(right)
	}
	if left[li] == ">" || right[ri] == ">" {
		return true
	}
	if left[li] != "*" && right[ri] != "*" && left[li] != right[ri] {
		return false
	}
	return patternsOverlap(left, right, li+1, ri+1)
}

// Filter returns configured subjects matching every query term.
func Filter(nodes []Node, query string) []Node {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nodes
	}
	result := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.Configured {
			continue
		}
		haystack := strings.ToLower(node.Subject + " " + strings.Join(node.Streams, " "))
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			result = append(result, node)
		}
	}
	return result
}
