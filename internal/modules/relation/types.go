// Package relation lets templates declare relations to other templates, persisted per template in
// a relations folder. Scope is deliberately minimal: set and get relations, nothing more.
package relation

// Cardinality is the shape of a relation. The three the user specified.
type Cardinality string

const (
	OneToOne   Cardinality = "one-to-one"
	OneToMany  Cardinality = "one-to-many"
	ManyToOne  Cardinality = "many-to-one"
	ManyToMany Cardinality = "many-to-many"
)

func (c Cardinality) valid() bool {
	switch c {
	case OneToOne, OneToMany, ManyToOne, ManyToMany:
		return true
	}
	return false
}

// inverse is the cardinality seen from the other side of the relation. one-to-one
// and many-to-many are symmetric; one-to-many and many-to-one are each other's flip.
func (c Cardinality) inverse() Cardinality {
	switch c {
	case OneToMany:
		return ManyToOne
	case ManyToOne:
		return OneToMany
	}
	return c
}

// Cardinalities returns the valid cardinalities in declaration order. The
// frontend's cardinality picker reads this so the option set has one source.
// many-to-one exists so the inverse of a one-to-many declaration is representable.
func Cardinalities() []Cardinality {
	return []Cardinality{OneToOne, OneToMany, ManyToOne, ManyToMany}
}

// CardinalityOption pairs a cardinality value with its i18n label key, so the
// frontend never maintains its own value->key mapping (backend steers the labels
// too, like field-type descriptors carry label_key).
type CardinalityOption struct {
	Value    Cardinality `json:"value"`
	LabelKey string      `json:"label_key"`
}

var cardinalityLabelKeys = map[Cardinality]string{
	OneToOne:   "workspace.templates.relations.cardinality.one_to_one",
	OneToMany:  "workspace.templates.relations.cardinality.one_to_many",
	ManyToOne:  "workspace.templates.relations.cardinality.many_to_one",
	ManyToMany: "workspace.templates.relations.cardinality.many_to_many",
}

// CardinalityOptions returns the picker options (value + label key) in order.
func CardinalityOptions() []CardinalityOption {
	cs := Cardinalities()
	out := make([]CardinalityOption, 0, len(cs))
	for _, c := range cs {
		out = append(out, CardinalityOption{Value: c, LabelKey: cardinalityLabelKeys[c]})
	}
	return out
}

// Edge is one record-to-record link: a source record id to a target record id (the actual
// relating, by id, never stored in form data).
type Edge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

// Relation is one declared relation from a template to another template, plus its edges. It is
// identified entirely by its target: the source is the owning file, and there is at most one
// relation per from -> to pair, so no name is needed. Inverse marks which side of the pair is the
// derived mirror; the two halves always carry opposite Inverse values (see the mirroring in
// SetRelations and Reconcile).
type Relation struct {
	To          string      `yaml:"to" json:"to"`
	Cardinality Cardinality `yaml:"cardinality" json:"cardinality"`
	Inverse     bool        `yaml:"inverse,omitempty" json:"inverse,omitempty"`
	Edges       []Edge      `yaml:"edges,omitempty" json:"edges,omitempty"`
}

// file is the on-disk shape of relations/<name>.yaml.
type file struct {
	Template  string     `yaml:"template" json:"template"`
	Relations []Relation `yaml:"relations" json:"relations"`
}
