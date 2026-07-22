package model

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type FactIndex struct {
	Title string
	Topic TopicPath

	topicRaw string
}

func NewFactIndex(title, topic string) (*FactIndex, error) {
	index := &FactIndex{Title: title, topicRaw: topic}
	parsed, err := ParseTopicPath(topic)
	if err == nil {
		index.Topic = parsed
	}
	if err := index.Validate(); err != nil {
		return nil, err
	}
	return index, nil
}

func (i *FactIndex) Validate() error {
	if i == nil {
		return nil
	}
	if i.Title == "" || strings.TrimSpace(i.Title) != i.Title {
		return fmt.Errorf("index.title must be a trimmed, non-empty string")
	}
	for _, r := range i.Title {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return fmt.Errorf("index.title must not contain control or line-separator characters")
		}
	}
	topic := i.Topic.String()
	if topic == "" {
		topic = i.topicRaw
	}
	if topic == "" {
		return fmt.Errorf("index.topic must be a valid, non-empty topic path")
	}
	if _, err := ParseTopicPath(topic); err != nil {
		return fmt.Errorf("index.topic: %w", err)
	}
	return nil
}

func (i *FactIndex) ValidateForEntry(kind Kind, topics []TopicPath) error {
	if i == nil {
		return nil
	}
	if err := i.Validate(); err != nil {
		return err
	}
	if kind != KindFact {
		return fmt.Errorf("index is only valid on kind: fact")
	}
	if slices.ContainsFunc(topics, i.Topic.Equal) {
		return nil
	}
	return fmt.Errorf("index.topic %q must also appear in topics", i.Topic.String())
}

func (i *FactIndex) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("index must be a mapping with title and topic")
	}
	values := map[string]string{}
	for offset := 0; offset < len(node.Content); offset += 2 {
		key, value := node.Content[offset], node.Content[offset+1]
		if key.Kind != yaml.ScalarNode || (key.Value != "title" && key.Value != "topic") {
			return fmt.Errorf("index contains unknown field %q", key.Value)
		}
		if _, exists := values[key.Value]; exists {
			return fmt.Errorf("index contains duplicate field %q", key.Value)
		}
		if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
			return fmt.Errorf("index.%s must be a string", key.Value)
		}
		values[key.Value] = value.Value
	}
	i.Title = values["title"]
	i.topicRaw = values["topic"]
	if topic, err := ParseTopicPath(i.topicRaw); err == nil {
		i.Topic = topic
	}
	return nil
}

func (i FactIndex) MarshalYAML() (any, error) {
	topic := i.Topic.String()
	if topic == "" {
		topic = i.topicRaw
	}
	return struct {
		Title string `yaml:"title"`
		Topic string `yaml:"topic"`
	}{Title: i.Title, Topic: topic}, nil
}

type FactIndexRow struct {
	ID    string
	Title string
	Topic TopicPath
}

// IsIndexed reports whether the entry carries a valid fact-index enrollment.
// Enrollment is the single membership rule for the indexed population: the
// index block must be present and pass ValidateForEntry for the entry's kind
// and topics. A malformed index loads with a warning (see validateFactIndex)
// but is never a member here, so every surface that consumes the population
// agrees on who is in it.
func (e *Entry) IsIndexed() bool {
	return e.Index != nil && e.Index.ValidateForEntry(e.Kind, e.Topics) == nil
}

// FilterIndexed selects entries whose enrollment is valid, ignoring lifecycle.
func FilterIndexed(entries []*Entry) []*Entry {
	result := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsIndexed() {
			result = append(result, entry)
		}
	}
	return result
}

func (g *Graph) IndexedFacts() []FactIndexRow {
	var rows []FactIndexRow
	for _, entry := range g.Entries {
		if !entry.IsIndexed() {
			continue
		}
		status := g.DerivedStatus(entry).Kind
		if status != StatusOpen && status != StatusActive {
			continue
		}
		rows = append(rows, FactIndexRow{
			ID: entry.ID, Title: entry.Index.Title,
			Topic: TopicPath{Components: append([]string(nil), entry.Index.Topic.Components...)},
		})
	}
	sort.Slice(rows, func(a, b int) bool {
		if left, right := rows[a].Topic.FoldKey(), rows[b].Topic.FoldKey(); left != right {
			return left < right
		}
		if left, right := caseFoldKey(rows[a].Title), caseFoldKey(rows[b].Title); left != right {
			return left < right
		}
		return rows[a].ID < rows[b].ID
	})
	return rows
}
