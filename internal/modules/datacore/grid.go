package datacore

// GridDim is one group-by dimension. Field is a field key, or "facet:<key>".
// When Table is set the dimension lives on that table's rows (the grid fans one
// row per table row); empty Table reads the root value, broadcast onto every
// fanned row. DateWidth > 0 buckets a date (4 year, 10 day, else month).
type GridDim struct {
	Field     string
	Table     string
	DateWidth int
}

// GridNum is one numeric measure, coerced per row. Table follows GridDim: empty
// reads the root, set reads the fanned table row.
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

// NumCell is one numeric measure value; OK is false on a non-coercible value
// (the LEFT-join NULL).
type NumCell struct {
	Value float64
	OK    bool
}

// GridRow is one raw row: its form, dimension values, and numeric measures.
type GridRow struct {
	Form string
	Dims []string
	Nums []NumCell
}

// Grid produces the raw flattened rows that feed aggregation: per root passing
// every filter, one or more rows of (form, dim values, numeric measures). Dims
// are complete-case (INNER: a row blank in any dim drops); nums are best-effort
// (LEFT: a missing or non-numeric value leaves OK false).
//
// A dim or measure naming a table fans that table (one row per table row, table
// values off the row, root values broadcast). Two columns of one table thus
// stay aligned per row, where the index's same-table self-join would cartesian
// them: that alignment is the intended divergence from the index. Naming more
// than one table is invalid and yields no rows.
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

func gridSource(root, rowID sym, table string) sym {
	if table != "" {
		return rowID
	}
	return root
}

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
