package model

// State is the cluster state of the node: indices, cluster settings,
// templates, data streams, pipelines and stored scripts. It holds data only;
// the server guards it with its locks (indices and most of the state under
// one lock, ingest pipelines and stored scripts under their own).
type State struct {
	Indices             map[string]*Index
	Counter             int64 // source of generated document IDs
	StateVersion        int64
	ClusterPersistent   map[string]any
	ClusterTransient    map[string]any // not durable
	LegacyTemplates     map[string]map[string]any
	ComposableTemplates map[string]map[string]any
	ComponentTemplates  map[string]map[string]any
	DataStreams         map[string]*DataStream
	IngestPipelines     map[string]map[string]any
	SearchPipelines     map[string]map[string]any
	Scripts             map[string]map[string]any
}

// NewState returns an empty cluster state.
func NewState() State {
	return State{
		Indices:             map[string]*Index{},
		StateVersion:        1,
		ClusterPersistent:   map[string]any{},
		ClusterTransient:    map[string]any{},
		LegacyTemplates:     map[string]map[string]any{},
		ComposableTemplates: map[string]map[string]any{},
		ComponentTemplates:  map[string]map[string]any{},
		DataStreams:         map[string]*DataStream{},
		IngestPipelines:     map[string]map[string]any{},
		SearchPipelines:     map[string]map[string]any{},
		Scripts:             map[string]map[string]any{},
	}
}

// DataStream is a data stream: its backing indices, newest last.
type DataStream struct {
	Name           string
	BackingIndices []string
	Generation     int64
	TimestampField string
	Template       string
}
