package datacore

// GridDim is one dimension of a raw grid: a field to group by. Field is a field
// key (or "facet:<key>" for a facet). When Table is set, the dimension lives on
// that table's rows and the grid fans one output row per table row, reading the
// value off the row identity; when Table is empty the value is read off the
// root and broadcast onto every fanned row. DateWidth > 0 buckets the value as
// a date prefix (4 = year, 10 = day, else month).
type GridDim struct {
	Field     string
	Table     string
	DateWidth int
}

// GridNum is one numeric measure column of a raw grid, coerced to a number per
// row. Table follows the same rule as GridDim: empty reads off the root, set
// reads off the fanned table row.
type GridNum struct {
	Field string
	Table string
}

// GridFilter scopes the grid to roots whose Field satisfies the comparison.
// Op is eq/ne (text) or lt/le/gt/ge (numeric).
type GridFilter struct {
	Field string
	Op    string
	Value string
}

// NumCell is one numeric measure value, with OK false when the row carried no
// coercible number (the LEFT-join NULL).
type NumCell struct {
	Value float64
	OK    bool
}

// GridRow is one raw row of a grid: the form it came from, its dimension
// values in order, and its numeric measures in order.
type GridRow struct {
	Form string
	Dims []string
	Nums []NumCell
}

// Grid produces the raw flattened rows that feed statistical aggregation: for
// each root passing every filter, one or more rows carrying the form, its
// dimension values, and its numeric measures. Dims are complete-case (a row
// missing or blank in any dim is dropped, matching an INNER join); nums are
// best-effort (a missing or non-numeric value leaves NumCell.OK false, matching
// a LEFT join). The caller groups by Dims and reduces Nums, counting distinct
// Forms.
//
// When no dim or measure names a table, each root yields one row from its own
// cells (scalar fields, facets, date-bucketed dims). When a dim or measure
// names a table, the grid fans that table: it follows the root to the table's
// rows and emits one row per table row, reading table-scoped values off the row
// identity and root-scoped values off the root (broadcast). Because both
// columns of a row are read from the SAME row identity, two columns of one
// table stay aligned per row, where the index's same-table self-join produces
// their cartesian. This row alignment is the divergence from the index, by
// design.
//
// All table-scoped dims and measures must name one table: aligning rows across
// two tables would itself be a cartesian. A grid that names more than one table
// is invalid and yields no rows (the Service surfaces the reason).
func (p *Perspective) Grid(dims []GridDim, nums []GridNum, filters []GridFilter) []GridRow {
	tables := fanTablesOf(dims, nums)
	if len(tables) > 1 {
		return nil
	}
	out := make([]GridRow, 0)
	for _, root := range p.identities() {
		if !p.passes(root, filters) {
			continue
		}
		form := p.t.iax.label(root)
		if len(tables) == 0 {
			if row, ok := p.gridRow(form, root, root, dims, nums); ok {
				out = append(out, row)
			}
			continue
		}
		ft, ok := p.t.fax.lookup(tables[0])
		if !ok {
			continue
		}
		for _, rowID := range p.t.refsFrom(root, ft) {
			if row, ok := p.gridRow(form, root, rowID, dims, nums); ok {
				out = append(out, row)
			}
		}
	}
	return out
}

// gridRow assembles one output row, reading each table-scoped dim/measure off
// rowID and each root-scoped one off root. It drops the row (ok=false) when any
// dim is missing, blank, or an unparseable date (complete-case INNER).
func (p *Perspective) gridRow(form string, root, rowID sym, dims []GridDim, nums []GridNum) (GridRow, bool) {
	row := GridRow{Form: form, Dims: make([]string, len(dims)), Nums: make([]NumCell, len(nums))}
	for di, d := range dims {
		v, ok := p.cellStr(gridSource(root, rowID, d.Table), d.Field)
		if !ok || v == "" {
			return GridRow{}, false
		}
		if d.DateWidth > 0 {
			bucketed, ok := dateByWidth(v, d.DateWidth)
			if !ok {
				return GridRow{}, false
			}
			v = bucketed
		}
		row.Dims[di] = v
	}
	for ni, n := range nums {
		if v, ok := p.cellStr(gridSource(root, rowID, n.Table), n.Field); ok && v != "" {
			if f, err := parseNum(v); err == nil {
				row.Nums[ni] = NumCell{Value: f, OK: true}
			}
		}
	}
	return row, true
}

// gridSource picks where a dim/measure reads from: the fanned table row when it
// is table-scoped, the root otherwise.
func gridSource(root, rowID sym, table string) sym {
	if table != "" {
		return rowID
	}
	return root
}

// fanTablesOf returns the distinct tables named by any dim or measure, in
// first-seen order. Empty means a plain root-level grid; one means fan that
// table; more than one is invalid.
func fanTablesOf(dims []GridDim, nums []GridNum) []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	add := func(table string) {
		if table == "" || seen[table] {
			return
		}
		seen[table] = true
		out = append(out, table)
	}
	for _, d := range dims {
		add(d.Table)
	}
	for _, n := range nums {
		add(n.Table)
	}
	return out
}

func (p *Perspective) cellStr(i sym, field string) (string, bool) {
	f, ok := p.t.fax.lookup(field)
	if !ok {
		return "", false
	}
	v, _, present := p.t.at(i, f, p.scope)
	return v, present
}

func (p *Perspective) passes(i sym, filters []GridFilter) bool {
	for _, flt := range filters {
		v, _ := p.cellStr(i, flt.Field)
		if !passOp(v, flt.Op, flt.Value) {
			return false
		}
	}
	return true
}

func passOp(cell, op, val string) bool {
	switch op {
	case "eq":
		return cell == val
	case "ne":
		return cell != val
	case "lt", "le", "gt", "ge":
		a, e1 := parseNum(cell)
		b, e2 := parseNum(val)
		if e1 != nil || e2 != nil {
			return false
		}
		switch op {
		case "lt":
			return a < b
		case "le":
			return a <= b
		case "gt":
			return a > b
		default:
			return a >= b
		}
	}
	return false
}

func dateByWidth(v string, width int) (string, bool) {
	d, ok := parseDate(v)
	if !ok {
		return "", false
	}
	switch width {
	case 4:
		return bucketDate(d, "year"), true
	case 10:
		return bucketDate(d, "day"), true
	default:
		return bucketDate(d, "month"), true
	}
}
