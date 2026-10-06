// Package diagram draws models and mappings as port graphs: nodes carry ordered
// ports (attributes), edges connect port to port. Projections turn record data
// into a Graph; Layout places it; SVG emits it. All three are pure and
// deterministic so the same data always yields the same picture.
package diagram

// Graph is the projection-neutral IR. Layers are the left-to-right columns;
// every node names the layer it sits in.
type Graph struct {
	Layers []Layer `json:"layers"`
	Nodes  []Node  `json:"nodes"`
	Edges  []Edge  `json:"edges"`
}

type Layer struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// Node kinds.
const (
	KindEntity = "entity"
	KindRule   = "rule"
)

// Node layer ids used by the lineage projection.
const (
	LayerSource = "source"
	LayerRule   = "rule"
	LayerTarget = "target"
)

// Port classes.
const (
	PortUnmapped  = "unmapped"
	PortUnsourced = "unsourced"
)

type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Layer string `json:"layer"`
	Ports []Port `json:"ports"`
}

type Port struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Class string `json:"class,omitempty"`
}

// PortRef addresses a port; an empty Port anchors on the node itself.
type PortRef struct {
	Node string `json:"node"`
	Port string `json:"port,omitempty"`
}

type Edge struct {
	From  PortRef `json:"from"`
	To    PortRef `json:"to"`
	Label string  `json:"label,omitempty"`
}
