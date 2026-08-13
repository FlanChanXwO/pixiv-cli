package understandgraph

import "encoding/json"

type scanFile struct {
	Path         string `json:"path"`
	Language     string `json:"language"`
	SizeLines    int    `json:"sizeLines"`
	FileCategory string `json:"fileCategory"`
}

type graphNode struct {
	rawFields     map[string]json.RawMessage
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	FilePath      string          `json:"filePath,omitempty"`
	LineRange     []int           `json:"lineRange,omitempty"`
	Summary       string          `json:"summary"`
	Tags          []string        `json:"tags"`
	Complexity    string          `json:"complexity"`
	LanguageNotes string          `json:"languageNotes,omitempty"`
	DomainMeta    json.RawMessage `json:"domainMeta,omitempty"`
	KnowledgeMeta json.RawMessage `json:"knowledgeMeta,omitempty"`
}

func (node *graphNode) UnmarshalJSON(data []byte) error {
	type plainGraphNode graphNode
	var plain plainGraphNode
	if err := json.Unmarshal(data, &plain); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*node = graphNode(plain)
	node.rawFields = fields
	return nil
}

func (node graphNode) MarshalJSON() ([]byte, error) {
	type plainGraphNode graphNode
	if node.rawFields == nil {
		return json.Marshal(plainGraphNode(node))
	}
	fields := cloneRawFields(node.rawFields)
	if err := setRawField(fields, "id", node.ID); err != nil {
		return nil, err
	}
	if err := setRawField(fields, "name", node.Name); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

type graphEdge struct {
	rawFields   map[string]json.RawMessage
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	Type        string  `json:"type"`
	Direction   string  `json:"direction"`
	Description string  `json:"description,omitempty"`
	Weight      float64 `json:"weight"`
}

func (edge *graphEdge) UnmarshalJSON(data []byte) error {
	type plainGraphEdge graphEdge
	var plain plainGraphEdge
	if err := json.Unmarshal(data, &plain); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*edge = graphEdge(plain)
	edge.rawFields = fields
	return nil
}

func (edge graphEdge) MarshalJSON() ([]byte, error) {
	type plainGraphEdge graphEdge
	if edge.rawFields == nil {
		return json.Marshal(plainGraphEdge(edge))
	}
	fields := cloneRawFields(edge.rawFields)
	if err := setRawField(fields, "source", edge.Source); err != nil {
		return nil, err
	}
	if err := setRawField(fields, "target", edge.Target); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func cloneRawFields(fields map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func setRawField(fields map[string]json.RawMessage, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fields[key] = raw
	return nil
}

type goPackage struct {
	ID         string
	Name       string
	ImportPath string
	Files      []string
}

type goSource struct {
	Path         string
	PackageName  string
	PackageID    string
	IsTest       bool
	ExternalTest bool
	Imports      []string
	Functions    []goFunction
	ContentHash  string
	TotalLines   int
}

type goFunction struct {
	Name          string
	Receiver      string
	QualifiedName string
	StartLine     int
	EndLine       int
}
