package builder

// Operator describes one selectable operator in the State-tab picker.
// LabelKey is an i18n key, not a translated string — keeps the
// frontend in charge of localisation while the backend remains the
// authoritative source for which operators exist per kind.
type Operator struct {
	Op       string `json:"op"`
	LabelKey string `json:"labelKey"`
}

// DateOpDescriptor is one helper in the Date-tab picker. HasArg tells
// the frontend whether to render the days-window number input next to
// the helper name (true for *N and ageGt/ageLt; false for the rest).
type DateOpDescriptor struct {
	Op       DateOp `json:"op"`
	LabelKey string `json:"labelKey"`
	HasArg   bool   `json:"hasArg"`
}

// OperatorsForKind returns the operator vocabulary for the State-tab
// picker. Boolean has no picker (the value IS the predicate, expressed
// as two rules) and Date is rendered from DateOps() instead — both
// return empty here so the frontend can branch on length.
func OperatorsForKind(kind RuleKind) []Operator {
	switch kind {
	case KindEnum:
		return []Operator{
			{Op: string(EnumOpEquals), LabelKey: "expression_builder.op.equals"},
			{Op: string(EnumOpNotEquals), LabelKey: "expression_builder.op.not_equals"},
		}
	case KindNumber:
		return []Operator{
			{Op: string(NumberOpEq), LabelKey: "expression_builder.op.eq"},
			{Op: string(NumberOpNe), LabelKey: "expression_builder.op.ne"},
			{Op: string(NumberOpGt), LabelKey: "expression_builder.op.gt"},
			{Op: string(NumberOpGe), LabelKey: "expression_builder.op.ge"},
			{Op: string(NumberOpLt), LabelKey: "expression_builder.op.lt"},
			{Op: string(NumberOpLe), LabelKey: "expression_builder.op.le"},
		}
	}
	return []Operator{}
}

// DateOps returns the date-helper vocabulary for the Date-tab picker.
// Order is the rendering order — ranged helpers (*N variants) follow
// their boolean counterparts so the picker stays predictable.
func DateOps() []DateOpDescriptor {
	return []DateOpDescriptor{
		{Op: DateOpIsOverdue, LabelKey: "expression_builder.date.is_overdue", HasArg: false},
		{Op: DateOpIsToday, LabelKey: "expression_builder.date.is_today", HasArg: false},
		{Op: DateOpIsFuture, LabelKey: "expression_builder.date.is_future", HasArg: false},
		{Op: DateOpIsDueSoon, LabelKey: "expression_builder.date.is_due_soon", HasArg: true},
		{Op: DateOpIsOverdueInDays, LabelKey: "expression_builder.date.is_overdue_in_days", HasArg: true},
		{Op: DateOpIsExpiredAfter, LabelKey: "expression_builder.date.is_expired_after", HasArg: true},
		{Op: DateOpIsUpcomingBefore, LabelKey: "expression_builder.date.is_upcoming_before", HasArg: true},
		{Op: DateOpDateGt, LabelKey: "expression_builder.date.date_gt", HasArg: true},
		{Op: DateOpDateLt, LabelKey: "expression_builder.date.date_lt", HasArg: true},
	}
}
