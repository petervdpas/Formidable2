// Package relation lets templates declare relations to other templates, persisted per template in
// a relations folder. Scope is deliberately minimal: set and get relations, nothing more.
package relation

// Cardinality is the shape of a relation. The three the user specified.
type Cardinality string

const (
	OneToOne   Cardinality = "one-to-one"
	OneToMany  Cardinality = "one-to-many"
	ManyToMany Cardinality = "many-to-many"
)

func (c Cardinality) valid() bool {
	switch c {
	case OneToOne, OneToMany, ManyToMany:
		return true
	}
	return false
}

// Edge is one record-to-record link: a source record id to a target record id (the actual
// relating, by id, never stored in form data).
type Edge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

// Relation is one declared relation from a template to another template, plus its edges. It is
// identified entirely by its target: the source is the owning file, and there is at most one
// relation per from -> to pair, so no name is needed.
type Relation struct {
	To          string      `yaml:"to" json:"to"`
	Cardinality Cardinality `yaml:"cardinality" json:"cardinality"`
	Edges       []Edge      `yaml:"edges,omitempty" json:"edges,omitempty"`
}

// file is the on-disk shape of relations/<name>.yaml.
type file struct {
	Template  string     `yaml:"template" json:"template"`
	Relations []Relation `yaml:"relations" json:"relations"`
}
