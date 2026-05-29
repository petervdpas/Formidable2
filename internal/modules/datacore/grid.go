package datacore

// GridDim is one dimension of a raw grid: a field to group by. Field is a root
// field key, or "facet:<key>" for a facet. DateWidth > 0 buckets the value as
// a date prefix (4 = year, 10 = day, else month).
type GridDim struct {
	Field     string
	DateWidth int
}

// GridNum is one numeric measure column of a raw grid: a root field coerced to
// a number per row.
type GridNum struct {
	Field string
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
// each root passing every filter, one row carrying the form, its dimension
// values, and its numeric measures. Dims are complete-case (a root missing or
// blank in any dim is dropped, matching an INNER join); nums are best-effort (a
// missing or non-numeric value leaves NumCell.OK false, matching a LEFT join).
// The caller groups by Dims and reduces Nums, counting distinct Forms.
//
// This core covers root fields, facets, and date-bucketed dims. Table-column
// dims (which fan per cell) are a separate pass: datacore reaches them through
// Follow and keeps rows aligned, so it intentionally diverges from the index's
// same-table cartesian rather than reproducing it.
func (p *Perspective) Grid(dims []GridDim, nums []GridNum, filters []GridFilter) []GridRow {
	out := make([]GridRow, 0)
	for _, root := range p.identities() {
		if !p.passes(root, filters) {
			continue
		}
		row := GridRow{Form: p.t.iax.label(root), Dims: make([]string, len(dims)), Nums: make([]NumCell, len(nums))}
		complete := true
		for di, d := range dims {
			v, ok := p.cellStr(root, d.Field)
			if !ok || v == "" {
				complete = false
				break
			}
			if d.DateWidth > 0 {
				bucketed, ok := dateByWidth(v, d.DateWidth)
				if !ok {
					complete = false
					break
				}
				v = bucketed
			}
			row.Dims[di] = v
		}
		if !complete {
			continue
		}
		for ni, n := range nums {
			if v, ok := p.cellStr(root, n.Field); ok && v != "" {
				if f, err := parseNum(v); err == nil {
					row.Nums[ni] = NumCell{Value: f, OK: true}
				}
			}
		}
		out = append(out, row)
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
