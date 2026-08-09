// Package subjects builds the read model used to explore configured NATS subjects.
package subjects

import (
	"slices"
	"strings"

	"github.com/clcsolutions/nats-tui/internal/app/streams"
)

// Node is one visible row in the flattened subject tree.
type Node struct {
	Subject      string
	WatchSubject string
	Depth        int
	Configured   bool
	Streams      []string
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
