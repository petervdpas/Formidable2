package builder

// Operator is one State-picker operator; LabelKey is an i18n key, not a translated string.
type Operator struct {
	Op       string `json:"op"`
	LabelKey string `json:"labelKey"`
}

// DateOpDescriptor is one Date-picker helper; HasArg gates the days-window input.
type DateOpDescriptor struct {
	Op       DateOp `json:"op"`
	LabelKey string `json:"labelKey"`
	HasArg   bool   `json:"hasArg"`
}

// OperatorsForKind returns the State-picker operators; empty for boolean (the value is the predicate) and date.
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

// DateOps returns the Date-picker helper vocabulary in render order.
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
